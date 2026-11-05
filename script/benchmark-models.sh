#!/usr/bin/env bash
# benchmark-models.sh — Run tinycode code review against tinycode-demo with multiple models.
#
# Uses `tinycode run` in headless mode to send a review prompt to each model,
# captures the output, and scores it against the 20 known planted bugs.
#
# Usage:
#   ./script/benchmark-models.sh                    # all MAAS chat models
#   ./script/benchmark-models.sh --status            # show progress of running/completed benchmark
#   ./script/benchmark-models.sh granite-4-1-8b     # single model
#   ./script/benchmark-models.sh --skip model1 model2  # skip specific models
#   DEMO_DIR=/path/to/demo ./script/benchmark-models.sh  # custom demo path
#
# Environment:
#   TIMEOUT          seconds per model (default: 300)
#   MAX_ITERATIONS   processor iterations per model (default: 50)
#   DEMO_DIR         path to tinycode-demo (default: ../tinycode-demo)
#   RESULTS_DIR      output directory (default: ./benchmark-results)
#   BENCHMARK_AGENT  agent to use (default: code-reviewer; also: security-reviewer, build)
#   TINYCODE_VLLM_HOST  vLLM endpoint (default: https://models.corp.redhat.com/oai)
#
# Prerequisites:
#   - tinycode binary built (make build)
#   - tinycode-demo project at $DEMO_DIR
#   - VPN connected (for models.corp.redhat.com)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
TINYCODE="${TINYCODE:-${PROJECT_DIR}/dist/tinycode}"
DEMO_DIR="${DEMO_DIR:-$(cd "$PROJECT_DIR/.." && pwd)/tinycode-demo}"
RESULTS_DIR="${RESULTS_DIR:-${PROJECT_DIR}/benchmark-results}"
TIMEOUT="${TIMEOUT:-300}"
MAX_ITERATIONS="${MAX_ITERATIONS:-50}"
LOG_FILE="${HOME}/.local/share/tinycode/tinycode.log"
PROGRESS_FILE="${RESULTS_DIR}/progress.txt"

# Convert to absolute paths
TINYCODE="$(cd "$(dirname "$TINYCODE")" 2>/dev/null && pwd)/$(basename "$TINYCODE")"
mkdir -p "$RESULTS_DIR"
RESULTS_DIR="$(cd "$RESULTS_DIR" && pwd)"

# Ensure vLLM provider is discoverable
export TINYCODE_VLLM_HOST="${TINYCODE_VLLM_HOST:-https://models.corp.redhat.com/oai}"

# The 20 planted bugs — ERE patterns to detect in model output.
declare -a BUG_PATTERNS=(
    "1:nil.*pointer|nil.*dereference|DueDate.*nil|task\.DueDate|nil check"
    "2:SQL.*inject|sql.*inject|fmt\.Sprintf.*SQL|string.*interpolat.*sql|parameterized"
    "3:DELETE.*GET|GET.*delete|delete.*method|wrong.*HTTP.*method|routes\.go.*delete"
    "4:missing.*return|no.*return.*after.*error|http\.Error.*return|execution.*continues"
    "5:mutex|concurrent.*access|race.*condition|sync\.Mutex|data.*race"
    "6:priority.*int.*string|priority.*type|string.*comparison.*int"
    "7:hardcoded.*key|hardcoded.*secret|api.*key.*source|sk-demo-secret"
    "8:off.by.one|rows\[2\]|skip.*first.*row|import.*skip|data.loss"
    "9:POST.*PUT|update.*POST|should.*PUT|always.*POST|create.*duplicate"
    "10:bare.*except|except:|catch.*everything|KeyboardInterrupt|SystemExit"
    "11:task-listt|getElementById.*typo|extra.*t|typo.*element"
    "12:missing.*await|fetch.*await|await.*fetch|promise.*resolve|Promise"
    "13:XSS|innerHTML|unsanitized|cross.site.*script|xss"
    "14:form.*action.*/task[^s]|wrong.*form.*action|action.*URL"
    "15:CSRF|csrf|cross.site.*request.*forgery"
    "16:unquoted.*variable|rm.*-rf.*unquoted|word.*splitting|rm.*-rf.*DEPLOY"
    "17:shebang|missing.*shebang|no.*shebang"
    "18:set.*-e|pipefail|set.*-u|error.*handling.*bash|set -euo"
    "19:due_date.*dueDate|dueDate.*due_date|field.*mismatch|camelCase.*snake"
    "20:hardcoded.*port|port.*8080|port.*hardcoded|configurable.*port"
)

# MAAS chat models (from models.corp.redhat.com catalog, 2026-11-04).
# Excludes embedding, reranking, safety, and auto-routing models.
MAAS_MODELS=(
    "claude-haiku-4-5"
    "claude-haiku-5-5"
    "claude-sonnet-4-6"
    "claude-sonnet-5-5"
    "deepseek-4-1-flash"
    "diffusiongemma-26b-a4b-it-fp8"
    "gemini-2-5-flash"
    "gemini-2-5-pro"
    "gemini-3-1-flash-lite"
    "gemini-3-1-pro-preview"
    "gemini-3-5-flash"
    "gemini-3-7-flash"
    "gemma-4-31b-it"
    "glm-5-3"
    "gpt-oss-120b"
    "gpt-oss-20b"
    "granite-3-1-8b-instruct"
    "granite-3-2-8b-instruct"
    "granite-3-3-8b-instruct"
    "granite-4-0-h-small"
    "granite-4-0-h-tiny"
    "granite-4-0-micro"
    "granite-4-1-30b"
    "granite-4-1-8b"
    "granite-switch-4-1-8b"
    "llama-3-3-70b-instruct"
    "mistral-7b-instruct-v0-3"
    "mistral-small-3-2-24b-instruct-2506"
    "nemotron-nano-30b-a3b-fp8"
    "nemotron-super-120b-bf16-ga"
    "nemotron-super-fp8-ana-002"
    "nvidia-nemotron-3-nano-30b-a3b-fp8-ga"
    "nvidia-nemotron-3-ultra-550b-a55b-nvfp4"
    "qwen3-8-27b-fp8"
)

REVIEW_PROMPT='Review this project for bugs and security issues. Read every source file (Go, Python, JavaScript, HTML, Bash). IMPORTANT: Do NOT read README.md — it contains test answers and will invalidate the benchmark. For each bug found, report: the file and line number, what the bug is, and its severity (high/medium/low). Number your findings. Do not fix anything — report only.'

# --- Functions ---

show_status() {
    echo "=== Benchmark Status ==="
    echo "Results: $RESULTS_DIR"
    echo ""

    local completed=0 total=0 running="" skipped=0
    for model in "${MAAS_MODELS[@]}"; do
        total=$((total + 1))
        local json_file="${RESULTS_DIR}/${model}.json"
        if [ -f "$json_file" ]; then
            completed=$((completed + 1))
        fi
    done

    if [ -f "$PROGRESS_FILE" ]; then
        running=$(tail -1 "$PROGRESS_FILE" 2>/dev/null || true)
    fi

    echo "Progress: $completed/${#MAAS_MODELS[@]} models completed"
    if [ -n "$running" ]; then
        echo "Current:  $running"
    fi
    echo ""

    if [ "$completed" -gt 0 ] && command -v jq &>/dev/null; then
        printf "  %-40s %6s  %5s  %4s  %4s  %s\n" "MODEL" "SCORE" "TIME" "ITER" "TOOL" "STATUS"
        echo "  $(printf '%.0s-' {1..90})"
        for f in "$RESULTS_DIR"/*.json; do
            [ -f "$f" ] || continue
            jq -r '"\(.model)\t\(.score)\t\(.total)\t\(.percent)\t\(.duration_seconds)\t\(.iterations)\t\(.tool_calls)\t\(.status)"' "$f" 2>/dev/null
        done | sort -t$'\t' -k2 -rn | while IFS=$'\t' read -r model score total pct dur iters tools status; do
            printf "  %-40s %2d/%-2d (%3d%%)  %4ds  %4s  %4s  %s\n" \
                "$model" "$score" "$total" "$pct" "$dur" "$iters" "$tools" "$status"
        done
    fi
    exit 0
}

score_output() {
    local output_file="$1"
    local found=0

    for entry in "${BUG_PATTERNS[@]}"; do
        local pattern="${entry#*:}"
        if grep -qiE "$pattern" "$output_file" 2>/dev/null; then
            found=$((found + 1))
        fi
    done

    echo "$found"
}

detect_skip() {
    local model="$1"
    local output_file="$2"
    local log_file="$3"
    local status="$4"
    local iterations="$5"
    local tool_calls="$6"
    local output_size
    output_size=$(wc -c < "$output_file" 2>/dev/null | tr -d ' ' || echo "0")

    # Model produced no output or errored immediately
    if [ "$status" != "completed" ] && [ "$output_size" -lt 50 ]; then
        echo "no-output"
        return
    fi

    # Model completed but made zero tool calls — not an agent model
    if [ "$tool_calls" -eq 0 ] && [ "$iterations" -le 2 ]; then
        # Check if output is just chat (no file references)
        if ! grep -qiE "handler/|store/|static/|scripts/|main\.go|routes\.go|task\.go" "$output_file" 2>/dev/null; then
            echo "no-tools"
            return
        fi
    fi

    echo ""
}

inject_provider_config() {
    local demo_config="${DEMO_DIR}/tinycode.jsonc"
    local vllm_host="${TINYCODE_VLLM_HOST}"
    python3 -I -c "
import json, sys, re
cfg_path = sys.argv[1]
vllm_url = sys.argv[2]
with open(cfg_path) as f:
    text = f.read()
text = re.sub(r'//.*\$', '', text, flags=re.MULTILINE)
cfg = json.loads(text)
cfg.setdefault('provider', {}).setdefault('vllm', {}).setdefault('options', {})['baseURL'] = vllm_url
with open(cfg_path, 'w') as f:
    json.dump(cfg, f, indent=2)
" "$demo_config" "$vllm_host" 2>/dev/null || true
}

run_model() {
    local model="$1"
    local provider="${2:-vllm}"
    local model_index="${3:-?}"
    local model_total="${4:-?}"
    local output_file="${RESULTS_DIR}/${model}.txt"
    local json_file="${RESULTS_DIR}/${model}.json"
    local log_out="${RESULTS_DIR}/${model}.log"
    local safe_model="${provider}/${model}"

    # Skip if already completed (supports resume)
    if [ -f "$json_file" ]; then
        local prev_status
        prev_status=$(jq -r '.status' "$json_file" 2>/dev/null || echo "")
        if [ "$prev_status" = "completed" ] || [ "$prev_status" = "skipped" ]; then
            local prev_score
            prev_score=$(jq -r '.score' "$json_file" 2>/dev/null || echo "0")
            local prev_dur
            prev_dur=$(jq -r '.duration_seconds' "$json_file" 2>/dev/null || echo "0")
            printf "  [%d/%d] %-40s %2d/20 (%3d%%)  %4ds  (cached)\n" \
                "$model_index" "$model_total" "$model" "$prev_score" "$((prev_score * 100 / 20))" "$prev_dur"
            return
        fi
    fi

    # Update progress file
    echo "[$(date '+%H:%M:%S')] $model_index/$model_total $model" >> "$PROGRESS_FILE"

    printf "  [%d/%d] %-40s " "$model_index" "$model_total" "$model"

    # Reset demo to clean state
    make -C "$DEMO_DIR" reset -s 2>/dev/null || true
    inject_provider_config

    # Record log position
    local log_start_line=0
    if [ -f "$LOG_FILE" ]; then
        log_start_line=$(wc -l < "$LOG_FILE")
    fi

    local start_time
    start_time=$(date +%s)

    # Run tinycode in headless mode
    local status
    if timeout "$TIMEOUT" "$TINYCODE" run \
        -m "$safe_model" \
        -agent "${BENCHMARK_AGENT:-code-reviewer}" \
        -dangerously-skip-permissions \
        -max-iterations "$MAX_ITERATIONS" \
        -no-fail-on-tool-error \
        -title "benchmark-${model}" \
        "$REVIEW_PROMPT" \
        2>"${RESULTS_DIR}/${model}.stderr" \
        >"$output_file"; then
        status="completed"
    else
        local exit_code=$?
        if [ "$exit_code" -eq 124 ]; then
            status="timeout"
        else
            status="error"
        fi
    fi

    # Extract log slice
    if [ -f "$LOG_FILE" ]; then
        tail -n +"$((log_start_line + 1))" "$LOG_FILE" > "$log_out" 2>/dev/null || true
    fi

    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))

    # Score
    local score
    score=$(score_output "$output_file")

    # Log stats
    local tool_calls log_errors log_warns compactions iterations
    tool_calls=$(grep -c "tool completed" "$log_out" 2>/dev/null | tr -d '\n' || echo "0")
    log_errors=$(grep -c "level=ERROR" "$log_out" 2>/dev/null | tr -d '\n' || echo "0")
    log_warns=$(grep -c "level=WARN" "$log_out" 2>/dev/null | tr -d '\n' || echo "0")
    compactions=$(grep -c "compaction triggered" "$log_out" 2>/dev/null | tr -d '\n' || echo "0")
    iterations=$(grep -c "processor loop iteration" "$log_out" 2>/dev/null | tr -d '\n' || echo "0")

    # Detect non-agent models
    local skip_reason
    skip_reason=$(detect_skip "$model" "$output_file" "$log_out" "$status" "$iterations" "$tool_calls")
    if [ -n "$skip_reason" ]; then
        status="skipped (${skip_reason})"
    fi

    # Bugs found/missed
    local found_list="" missed_list=""
    for entry in "${BUG_PATTERNS[@]}"; do
        local num="${entry%%:*}"
        local pattern="${entry#*:}"
        if grep -qiE "$pattern" "$output_file" 2>/dev/null; then
            found_list="${found_list}${num},"
        else
            missed_list="${missed_list}${num},"
        fi
    done

    # Output size
    local output_bytes
    output_bytes=$(wc -c < "$output_file" 2>/dev/null | tr -d ' ' || echo "0")

    # Count total findings reported by the model (lines starting with numbers, bullets, or bracketed severity)
    local total_findings
    total_findings=$(grep -ciE "^\s*[0-9]+[\.\)]\s|^\s*[-\*]\s.*\b(bug|issue|vuln|error|missing|hardcoded|inject|xss|csrf)\b|\[(critical|high|medium|low)\]" "$output_file" 2>/dev/null | tr -d '\n' || echo "0")
    local bonus_findings=$((total_findings - score))
    if [ "$bonus_findings" -lt 0 ]; then
        bonus_findings=0
    fi

    # Write JSON result
    cat > "$json_file" <<JSONEOF
{
    "model": "${model}",
    "provider": "${provider}",
    "agent": "${BENCHMARK_AGENT:-code-reviewer}",
    "score": ${score},
    "total": 20,
    "percent": $((score * 100 / 20)),
    "total_findings": ${total_findings},
    "bonus_findings": ${bonus_findings},
    "duration_seconds": ${duration},
    "status": "${status}",
    "tool_calls": ${tool_calls},
    "iterations": ${iterations},
    "compactions": ${compactions},
    "log_errors": ${log_errors},
    "log_warnings": ${log_warns},
    "output_bytes": ${output_bytes},
    "found": [${found_list%,}],
    "missed": [${missed_list%,}],
    "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
JSONEOF

    # Print result
    printf "%2d/20 (%3d%%) +%d  %4ds  %s\n" \
        "$score" "$((score * 100 / 20))" "$bonus_findings" "$duration" "$status"

    # Update progress
    echo "[$(date '+%H:%M:%S')] $model_index/$model_total $model → ${score}/20 (${duration}s) $status" >> "$PROGRESS_FILE"

    # Reset demo
    make -C "$DEMO_DIR" reset -s 2>/dev/null || true
}

print_summary() {
    echo ""
    echo "=== Final Results (sorted by score) ==="
    echo ""

    if ! command -v jq &>/dev/null; then
        echo "Install jq for formatted summary. Raw results in: $RESULTS_DIR/"
        return
    fi

    local completed=0 skipped=0 errored=0 timed_out=0
    printf "  %-40s %6s  %5s  %5s  %4s  %4s  %s\n" "MODEL" "SCORE" "BONUS" "TIME" "ITER" "TOOL" "STATUS"
    echo "  $(printf '%.0s-' {1..100})"

    for f in "$RESULTS_DIR"/*.json; do
        [ -f "$f" ] || continue
        jq -r '"\(.model)\t\(.score)\t\(.total)\t\(.percent)\t\(.total_findings // 0)\t\(.bonus_findings // 0)\t\(.duration_seconds)\t\(.iterations)\t\(.tool_calls)\t\(.status)"' "$f" 2>/dev/null
    done | sort -t$'\t' -k2 -rn -k5 -n | while IFS=$'\t' read -r model score total pct tfind bonus dur iters tools status; do
        printf "  %-40s %2d/%-2d (%3d%%)  +%-3s  %4ds  %4s  %4s  %s\n" \
            "$model" "$score" "$total" "$pct" "$bonus" "$dur" "$iters" "$tools" "$status"
    done

    # Stats
    echo ""
    local total_models total_completed total_skipped
    total_models=$(ls "$RESULTS_DIR"/*.json 2>/dev/null | wc -l | tr -d ' ')
    total_completed=$(jq -r '.status' "$RESULTS_DIR"/*.json 2>/dev/null | grep -c "completed" || echo "0")
    total_skipped=$(jq -r '.status' "$RESULTS_DIR"/*.json 2>/dev/null | grep -c "skipped" || echo "0")
    echo "  Models tested: $total_models  Completed: $total_completed  Skipped: $total_skipped"
    echo ""
    echo "  Results: $RESULTS_DIR/"
    echo "  Per model: .json (scores) .txt (output) .log (tinycode log) .stderr (errors)"
    echo ""
    echo "  Check progress anytime:  $0 --status"
    echo "  Resume interrupted run:  $0  (completed models are cached)"
}

# --- Main ---

# Handle --status flag
if [ "${1:-}" = "--status" ]; then
    show_status
fi

if [ ! -d "$DEMO_DIR" ]; then
    echo "Error: tinycode-demo not found at $DEMO_DIR"
    echo "Set DEMO_DIR=/path/to/tinycode-demo"
    exit 1
fi

if [ ! -x "$TINYCODE" ]; then
    echo "Error: tinycode binary not found at $TINYCODE"
    echo "Run: make build"
    exit 1
fi

# Determine which models to test
models=()
provider="vllm"
skip_models=()

while [ $# -gt 0 ]; do
    case "$1" in
        --provider)
            provider="$2"
            shift 2
            mapfile -t models < <("$TINYCODE" models 2>/dev/null | grep "^$provider/" | sed "s|$provider/||" || true)
            if [ ${#models[@]} -eq 0 ]; then
                echo "No models found for provider: $provider"
                exit 1
            fi
            ;;
        --skip)
            shift
            while [ $# -gt 0 ] && [[ ! "$1" =~ ^-- ]]; do
                skip_models+=("$1")
                shift
            done
            ;;
        *)
            models+=("$1")
            shift
            ;;
    esac
done

if [ ${#models[@]} -eq 0 ]; then
    models=("${MAAS_MODELS[@]}")
fi

# Filter out skipped models
if [ ${#skip_models[@]} -gt 0 ]; then
    filtered=()
    for m in "${models[@]}"; do
        local_skip=false
        for s in "${skip_models[@]}"; do
            if [ "$m" = "$s" ]; then
                local_skip=true
                break
            fi
        done
        if ! $local_skip; then
            filtered+=("$m")
        fi
    done
    models=("${filtered[@]}")
fi

# Must run from the demo directory so tinycode sees the project
cd "$DEMO_DIR"

# Initialize progress
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Benchmark started — ${#models[@]} models" > "$PROGRESS_FILE"

echo "=== tinycode Model Benchmark ==="
echo "Demo:     $DEMO_DIR"
echo "Models:   ${#models[@]}"
echo "Timeout:  ${TIMEOUT}s per model"
echo "Results:  $RESULTS_DIR"
echo ""
echo "  Check progress:  $0 --status"
echo "  Resume if interrupted: just re-run (completed models are cached)"
echo ""
printf "  %-50s %6s  %5s  %s\n" "MODEL" "SCORE" "TIME" "STATUS"
echo "  $(printf '%.0s-' {1..90})"

model_index=0
for model in "${models[@]}"; do
    model_index=$((model_index + 1))
    run_model "$model" "$provider" "$model_index" "${#models[@]}"
done

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Benchmark complete" >> "$PROGRESS_FILE"

print_summary

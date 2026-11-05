#!/usr/bin/env bash
# benchmark-models.sh — Run tinycode code review against tinycode-demo with multiple models.
#
# Uses `tinycode run` in headless mode to send a review prompt to each model,
# captures the output, and scores it against the 20 known planted bugs.
#
# Usage:
#   ./script/benchmark-models.sh                    # all MAAS chat models
#   ./script/benchmark-models.sh granite-4-1-8b     # single model
#   ./script/benchmark-models.sh --provider vllm    # all models from one provider
#   DEMO_DIR=/path/to/demo ./script/benchmark-models.sh  # custom demo path
#
# Prerequisites:
#   - tinycode binary built (make build)
#   - tinycode-demo project at $DEMO_DIR (default: ../tinycode-demo)
#   - Provider configured (MAAS_API_KEY or provider in tinycode.jsonc)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
TINYCODE="${TINYCODE:-${PROJECT_DIR}/dist/tinycode}"
DEMO_DIR="${DEMO_DIR:-$(cd "$PROJECT_DIR/.." && pwd)/tinycode-demo}"
RESULTS_DIR="${RESULTS_DIR:-${PROJECT_DIR}/benchmark-results}"
TIMEOUT="${TIMEOUT:-300}"
MAX_ITERATIONS="${MAX_ITERATIONS:-50}"
LOG_FILE="${HOME}/.local/share/tinycode/tinycode.log"

# Convert to absolute paths
TINYCODE="$(cd "$(dirname "$TINYCODE")" && pwd)/$(basename "$TINYCODE")"
RESULTS_DIR="$(mkdir -p "$RESULTS_DIR" && cd "$RESULTS_DIR" && pwd)"

# The 20 planted bugs — grep patterns to detect in model output.
# Each pattern is "BUG_NUMBER:grep_pattern" where the pattern matches
# how a model would describe finding that bug.
declare -a BUG_PATTERNS=(
    "1:nil.*pointer\|nil.*dereference\|DueDate.*nil\|task\.DueDate\|nil check"
    "2:SQL.*inject\|sql.*inject\|fmt\.Sprintf.*SQL\|string.*interpolat.*sql\|parameterized"
    "3:DELETE.*GET\|GET.*delete\|delete.*method\|wrong.*HTTP.*method\|routes\.go.*delete"
    "4:missing.*return\|no.*return.*after.*error\|http\.Error.*return\|execution.*continues"
    "5:mutex\|concurrent.*access\|race.*condition\|sync\.Mutex\|data.*race"
    "6:priority.*int.*string\|priority.*type\|string.*comparison.*int"
    "7:hardcoded.*key\|hardcoded.*secret\|api.*key.*source\|sk-demo-secret"
    "8:off.by.one\|rows\[2\]\|skip.*first.*row\|import.*skip"
    "9:POST.*PUT\|update.*POST\|should.*PUT\|always.*POST"
    "10:bare.*except\|except:\|catch.*everything\|KeyboardInterrupt"
    "11:task-listt\|getElementById.*typo\|extra.*t\|typo.*element"
    "12:missing.*await\|fetch.*await\|await.*fetch\|promise.*resolve"
    "13:XSS\|innerHTML\|unsanitized\|cross.site.*script\|xss"
    "14:form.*action.*/task[^s]\|wrong.*form.*action\|action.*URL"
    "15:CSRF\|csrf\|cross.site.*request.*forgery"
    "16:unquoted.*variable\|rm.*-rf.*unquoted\|word.*splitting\|\$[A-Z].*rm"
    "17:shebang\|#!/\|missing.*shebang"
    "18:set.*-e\|pipefail\|set.*-u\|error.*handling.*bash"
    "19:due_date.*dueDate\|dueDate.*due_date\|field.*mismatch\|camelCase.*snake"
    "20:hardcoded.*port\|port.*8080\|port.*hardcoded\|configurable.*port"
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

# Ensure vLLM provider is discoverable
export TINYCODE_VLLM_HOST="${TINYCODE_VLLM_HOST:-https://models.corp.redhat.com/oai}"

REVIEW_PROMPT='Review this project for bugs and security issues. Read every source file (Go, Python, JavaScript, HTML, Bash). IMPORTANT: Do NOT read README.md — it contains test answers and will invalidate the benchmark. For each bug found, report: the file and line number, what the bug is, and its severity (high/medium/low). Number your findings. Do not fix anything — report only.'

# --- Functions ---

score_output() {
    local output_file="$1"
    local found=0
    local found_bugs=""
    local missed_bugs=""

    for entry in "${BUG_PATTERNS[@]}"; do
        local num="${entry%%:*}"
        local pattern="${entry#*:}"
        if grep -qiE "$pattern" "$output_file" 2>/dev/null; then
            found=$((found + 1))
            found_bugs="${found_bugs} #${num}"
        else
            missed_bugs="${missed_bugs} #${num}"
        fi
    done

    echo "$found"
}

run_model() {
    local model="$1"
    local provider="${2:-vllm}"
    local output_file="${RESULTS_DIR}/${model}.txt"
    local json_file="${RESULTS_DIR}/${model}.json"
    local safe_model="${provider}/${model}"

    echo "--- Testing: ${model} ---"

    # Reset demo to clean state
    make -C "$DEMO_DIR" reset -s 2>/dev/null || true

    # Record log position before run so we can extract this model's log slice
    local log_start_line=0
    if [ -f "$LOG_FILE" ]; then
        log_start_line=$(wc -l < "$LOG_FILE")
    fi

    local start_time
    start_time=$(date +%s)

    # Ensure the demo project has the vLLM provider configured so the run
    # command can resolve the model without waiting for async discovery.
    local demo_config="${DEMO_DIR}/tinycode.jsonc"
    local vllm_host="${TINYCODE_VLLM_HOST}"
    python3 -I -c "
import json, sys
cfg_path = sys.argv[1]
vllm_url = sys.argv[2]
with open(cfg_path) as f:
    text = f.read()
# Strip JS-style comments for json.loads
import re
text = re.sub(r'//.*$', '', text, flags=re.MULTILINE)
cfg = json.loads(text)
cfg.setdefault('provider', {}).setdefault('vllm', {}).setdefault('options', {})['baseURL'] = vllm_url
with open(cfg_path, 'w') as f:
    json.dump(cfg, f, indent=2)
" "$demo_config" "$vllm_host" 2>/dev/null || true

    # Run tinycode in headless mode
    if timeout "$TIMEOUT" "$TINYCODE" run \
        -m "$safe_model" \
        -dangerously-skip-permissions \
        -max-iterations "$MAX_ITERATIONS" \
        -no-fail-on-tool-error \
        -title "benchmark-${model}" \
        "$REVIEW_PROMPT" \
        2>"${RESULTS_DIR}/${model}.stderr" \
        >"$output_file"; then
        local status="completed"
    else
        local exit_code=$?
        if [ "$exit_code" -eq 124 ]; then
            local status="timeout"
        else
            local status="error (exit $exit_code)"
        fi
    fi

    # Extract this model's log slice
    if [ -f "$LOG_FILE" ]; then
        tail -n +"$((log_start_line + 1))" "$LOG_FILE" > "${RESULTS_DIR}/${model}.log" 2>/dev/null || true
    fi

    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))

    # Score the output
    local score
    score=$(score_output "$output_file")

    # Count tool calls and errors from the log slice
    local log_file="${RESULTS_DIR}/${model}.log"
    local tool_calls log_errors log_warns compactions iterations
    tool_calls=$(grep -c "tool completed" "$log_file" 2>/dev/null | tr -d '\n' || echo "0")
    log_errors=$(grep -c "level=ERROR" "$log_file" 2>/dev/null | tr -d '\n' || echo "0")
    log_warns=$(grep -c "level=WARN" "$log_file" 2>/dev/null | tr -d '\n' || echo "0")
    compactions=$(grep -c "compaction triggered" "$log_file" 2>/dev/null | tr -d '\n' || echo "0")
    iterations=$(grep -c "processor loop iteration" "$log_file" 2>/dev/null | tr -d '\n' || echo "0")

    # Detect which bugs were found/missed
    local found_list=""
    local missed_list=""
    for entry in "${BUG_PATTERNS[@]}"; do
        local num="${entry%%:*}"
        local pattern="${entry#*:}"
        if grep -qiE "$pattern" "$output_file" 2>/dev/null; then
            found_list="${found_list}${num},"
        else
            missed_list="${missed_list}${num},"
        fi
    done

    # Write JSON result
    cat > "$json_file" <<JSONEOF
{
    "model": "${model}",
    "provider": "${provider}",
    "score": ${score},
    "total": 20,
    "percent": $((score * 100 / 20)),
    "duration_seconds": ${duration},
    "status": "${status}",
    "tool_calls": ${tool_calls},
    "iterations": ${iterations},
    "compactions": ${compactions},
    "log_errors": ${log_errors},
    "log_warnings": ${log_warns},
    "found": [${found_list%,}],
    "missed": [${missed_list%,}],
    "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
JSONEOF

    # Print summary line
    printf "  %-45s %2d/20 (%3d%%)  %4ds  %s\n" \
        "$model" "$score" "$((score * 100 / 20))" "$duration" "$status"

    # Reset demo
    make -C "$DEMO_DIR" reset -s 2>/dev/null || true
}

# --- Main ---

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

mkdir -p "$RESULTS_DIR"

# Determine which models to test
models=()
provider="vllm"

if [ $# -eq 0 ]; then
    models=("${MAAS_MODELS[@]}")
elif [ "$1" = "--provider" ] && [ $# -ge 2 ]; then
    provider="$2"
    # Use tinycode models command to list available models
    mapfile -t models < <("$TINYCODE" models 2>/dev/null | grep "^$provider/" | sed "s|$provider/||" || true)
    if [ ${#models[@]} -eq 0 ]; then
        echo "No models found for provider: $provider"
        exit 1
    fi
else
    models=("$@")
fi

# Must run from the demo directory so tinycode sees the project
cd "$DEMO_DIR"

echo "=== tinycode Model Benchmark ==="
echo "Demo:     $DEMO_DIR"
echo "Models:   ${#models[@]}"
echo "Timeout:  ${TIMEOUT}s"
echo "Results:  $RESULTS_DIR"
echo ""
printf "  %-45s %5s  %5s  %s\n" "MODEL" "SCORE" "TIME" "STATUS"
echo "  $(printf '%.0s-' {1..80})"

for model in "${models[@]}"; do
    run_model "$model" "$provider"
done

echo ""
echo "=== Summary ==="

# Sort results by score descending
if command -v jq &>/dev/null; then
    echo ""
    printf "  %-45s %5s  %5s  %s\n" "MODEL" "SCORE" "TIME" "STATUS"
    echo "  $(printf '%.0s-' {1..80})"
    for f in "$RESULTS_DIR"/*.json; do
        [ -f "$f" ] || continue
        jq -r '"\(.model)\t\(.score)\t\(.total)\t\(.percent)\t\(.duration_seconds)\t\(.status)"' "$f"
    done | sort -t$'\t' -k2 -rn | while IFS=$'\t' read -r model score total pct dur status; do
        printf "  %-45s %2d/%-2d (%3d%%)  %4ds  %s\n" "$model" "$score" "$total" "$pct" "$dur" "$status"
    done
    echo ""
    echo "JSON results in: $RESULTS_DIR/*.json"
    echo "Full output in:  $RESULTS_DIR/*.txt"
else
    echo "Install jq for sorted summary. Raw results in: $RESULTS_DIR/"
fi

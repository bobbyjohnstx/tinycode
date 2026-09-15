#!/usr/bin/env bash
#
# Functional tests for tinycode plugins against live APIs.
#
# Builds each plugin binary, sends JSON-RPC initialize + tool/call
# over stdin, and validates the response content.
#
# Prerequisites:
#   - Go toolchain (builds plugins on demand)
#   - Internet access (for public API plugins)
#   - jq installed
#
# Usage:
#   ./script/test-plugins.sh                    # all public API tests
#   ./script/test-plugins.sh web-search         # single plugin
#   ./script/test-plugins.sh --list             # list test groups
#
# Environment:
#   PLUGIN_TEST_TIMEOUT  Override per-test timeout in seconds (default: 30)

set -euo pipefail

TIMEOUT="${PLUGIN_TEST_TIMEOUT:-30}"
BUILD_DIR=$(mktemp -d)
PASS=0
FAIL=0
SKIP=0
TOTAL=0
RESULTS=()

trap 'rm -rf "$BUILD_DIR"' EXIT

# Colors
if [ -t 1 ]; then
    GREEN='\033[0;32m'
    RED='\033[0;31m'
    YELLOW='\033[0;33m'
    BOLD='\033[1m'
    NC='\033[0m'
else
    GREEN='' RED='' YELLOW='' BOLD='' NC=''
fi

# --- Helpers ---

build_plugin() {
    local name="$1"
    local bin="$BUILD_DIR/$name"
    if [ ! -x "$bin" ]; then
        go build -o "$bin" "./cmd/plugin-$name" 2>/dev/null || {
            echo "    Failed to build plugin-$name" >&2
            return 1
        }
    fi
    echo "$bin"
}

# run_tool_call builds the plugin, sends initialize + tool/call, returns content.
# Uses temp files to handle large JSON responses (100KB+) that break echo/command substitution.
run_tool_call() {
    local plugin_name="$1" tool_name="$2" args="$3"
    local bin
    bin=$(build_plugin "$plugin_name") || return 1

    local init_params='{"version":"1.0","directory":"/tmp"}'
    local init_req
    init_req=$(printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":%s}' "$init_params")

    local tool_params
    tool_params=$(printf '{"name":"%s","args":%s,"context":{"sessionID":"test","directory":"/tmp"}}' "$tool_name" "$args")
    local tool_req
    tool_req=$(printf '{"jsonrpc":"2.0","id":2,"method":"tool/call","params":%s}' "$tool_params")

    local outfile="$BUILD_DIR/resp_$$_$RANDOM"
    printf '%s\n%s\n' "$init_req" "$tool_req" | timeout "$TIMEOUT" "$bin" 2>/dev/null > "$outfile" || return 1

    local tool_resp
    tool_resp=$(sed -n '2p' "$outfile")
    rm -f "$outfile"
    [ -z "$tool_resp" ] && return 1

    local err_msg
    err_msg=$(printf '%s' "$tool_resp" | jq -r '.error.message // empty' 2>/dev/null)
    if [ -n "$err_msg" ]; then
        echo "RPC error: $err_msg"
        return 1
    fi

    printf '%s' "$tool_resp" | jq -r '.result.content // .result.Content // .result // empty' 2>/dev/null
}

assert_tool_contains() {
    local plugin_name="$1" tool_name="$2" args="$3" label="$4" expect="$5"
    TOTAL=$((TOTAL + 1))

    local content
    content=$(run_tool_call "$plugin_name" "$tool_name" "$args" 2>/dev/null) || {
        echo -e "  ${RED}FAIL${NC}: $label (plugin error or timeout)"
        FAIL=$((FAIL + 1))
        RESULTS+=("FAIL  $label")
        return
    }

    if printf '%s\n' "$content" | grep -iF "$expect" >/dev/null 2>&1; then
        echo -e "  ${GREEN}PASS${NC}: $label"
        PASS=$((PASS + 1))
        RESULTS+=("PASS  $label")
    else
        echo -e "  ${RED}FAIL${NC}: $label"
        echo "    Expected to contain: $expect"
        echo "    Got (first 200 chars): $(printf '%s\n' "$content" | head -c 200)"
        FAIL=$((FAIL + 1))
        RESULTS+=("FAIL  $label")
    fi
}

assert_tool_not_contains() {
    local plugin_name="$1" tool_name="$2" args="$3" label="$4" reject="$5"
    TOTAL=$((TOTAL + 1))

    local content
    content=$(run_tool_call "$plugin_name" "$tool_name" "$args" 2>/dev/null) || {
        echo -e "  ${RED}FAIL${NC}: $label (plugin error or timeout)"
        FAIL=$((FAIL + 1))
        RESULTS+=("FAIL  $label")
        return
    }

    if printf '%s\n' "$content" | grep -iF "$reject" >/dev/null 2>&1; then
        echo -e "  ${RED}FAIL${NC}: $label"
        echo "    Should NOT contain: $reject"
        FAIL=$((FAIL + 1))
        RESULTS+=("FAIL  $label")
    else
        echo -e "  ${GREEN}PASS${NC}: $label"
        PASS=$((PASS + 1))
        RESULTS+=("PASS  $label")
    fi
}

skip_test() {
    local label="$1" reason="$2"
    TOTAL=$((TOTAL + 1))
    SKIP=$((SKIP + 1))
    echo -e "  ${YELLOW}SKIP${NC}: $label ($reason)"
    RESULTS+=("SKIP  $label")
}

# --- Helpers for option-bearing plugins ---

# run_tool_call_with_opts builds the plugin, sends initialize with options + tool/call, returns content.
run_tool_call_with_opts() {
    local plugin_name="$1" tool_name="$2" args="$3" opts="$4"
    local bin
    bin=$(build_plugin "$plugin_name") || return 1

    local init_params
    init_params=$(printf '{"version":"1.0","directory":"/tmp","options":%s}' "$opts")
    local init_req
    init_req=$(printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":%s}' "$init_params")

    local tool_params
    tool_params=$(printf '{"name":"%s","args":%s,"context":{"sessionID":"test","directory":"/tmp"}}' "$tool_name" "$args")
    local tool_req
    tool_req=$(printf '{"jsonrpc":"2.0","id":2,"method":"tool/call","params":%s}' "$tool_params")

    local outfile="$BUILD_DIR/resp_$$_$RANDOM"
    printf '%s\n%s\n' "$init_req" "$tool_req" | timeout "$TIMEOUT" "$bin" 2>/dev/null > "$outfile" || return 1

    local tool_resp
    tool_resp=$(sed -n '2p' "$outfile")
    rm -f "$outfile"
    [ -z "$tool_resp" ] && return 1

    local err_msg
    err_msg=$(printf '%s' "$tool_resp" | jq -r '.error.message // empty' 2>/dev/null)
    if [ -n "$err_msg" ]; then
        echo "RPC error: $err_msg"
        return 1
    fi

    printf '%s' "$tool_resp" | jq -r '.result.content // .result.Content // .result // empty' 2>/dev/null
}

assert_tool_contains_opts() {
    local plugin_name="$1" tool_name="$2" args="$3" opts="$4" label="$5" expect="$6"
    TOTAL=$((TOTAL + 1))

    local content
    content=$(run_tool_call_with_opts "$plugin_name" "$tool_name" "$args" "$opts" 2>/dev/null) || {
        echo -e "  ${RED}FAIL${NC}: $label (plugin error or timeout)"
        FAIL=$((FAIL + 1))
        RESULTS+=("FAIL  $label")
        return
    }

    if printf '%s\n' "$content" | grep -iF "$expect" >/dev/null 2>&1; then
        echo -e "  ${GREEN}PASS${NC}: $label"
        PASS=$((PASS + 1))
        RESULTS+=("PASS  $label")
    else
        echo -e "  ${RED}FAIL${NC}: $label"
        echo "    Expected to contain: $expect"
        echo "    Got (first 200 chars): $(printf '%s\n' "$content" | head -c 200)"
        FAIL=$((FAIL + 1))
        RESULTS+=("FAIL  $label")
    fi
}

assert_tool_not_contains_opts() {
    local plugin_name="$1" tool_name="$2" args="$3" opts="$4" label="$5" reject="$6"
    TOTAL=$((TOTAL + 1))

    local content
    content=$(run_tool_call_with_opts "$plugin_name" "$tool_name" "$args" "$opts" 2>/dev/null) || {
        echo -e "  ${RED}FAIL${NC}: $label (plugin error or timeout)"
        FAIL=$((FAIL + 1))
        RESULTS+=("FAIL  $label")
        return
    }

    if printf '%s\n' "$content" | grep -iF "$reject" >/dev/null 2>&1; then
        echo -e "  ${RED}FAIL${NC}: $label"
        echo "    Should NOT contain: $reject"
        FAIL=$((FAIL + 1))
        RESULTS+=("FAIL  $label")
    else
        echo -e "  ${GREEN}PASS${NC}: $label"
        PASS=$((PASS + 1))
        RESULTS+=("PASS  $label")
    fi
}

# --- Test Groups (public API — no credentials) ---

test_web_search() {
    echo -e "${BOLD}web-search${NC} (DuckDuckGo HTML, public)"

    assert_tool_contains web-search "web_search" \
        '{"query":"OpenShift container platform","maxResults":3}' \
        "web_search: returns results for OpenShift" \
        "http"

    assert_tool_contains web-search "rh_kb_search" \
        '{"query":"kernel panic troubleshooting","maxResults":3}' \
        "rh_kb_search: scoped to access.redhat.com" \
        "http"

    assert_tool_contains web-search "web_search" \
        '{"query":"Kubernetes pod networking"}' \
        "web_search: default maxResults returns results" \
        "http"

    echo ""
}

test_rh_dev_content() {
    echo -e "${BOLD}rh-dev-content${NC} (developers.redhat.com, public)"

    assert_tool_contains rh-dev-content "rh_dev_search" \
        '{"topic":"kubernetes","page":1}' \
        "rh_dev_search: browse kubernetes articles" \
        "kubernetes"

    assert_tool_contains rh-dev-content "rh_dev_search" \
        '{"topic":"python","page":1}' \
        "rh_dev_search: browse python articles" \
        "python"

    assert_tool_contains rh-dev-content "rh_dev_search" \
        '{"topic":"invalid-topic-xyz"}' \
        "rh_dev_search: rejects invalid topic" \
        "Unknown topic"

    # RSS feed is blocked by Akamai WAF (403) — test for graceful degradation
    assert_tool_contains rh-dev-content "rh_dev_recent" \
        '{"limit":3}' \
        "rh_dev_recent: handles RSS 403 gracefully" \
        "Failed to fetch"

    echo ""
}

test_rh_ecosystem_catalog() {
    echo -e "${BOLD}rh-ecosystem-catalog${NC} (Pyxis API, public)"

    assert_tool_contains rh-ecosystem-catalog "ecosystem_search" \
        '{"repository":"ubi9","page_size":3}' \
        "ecosystem_search: find ubi9 image" \
        "ubi"

    assert_tool_contains rh-ecosystem-catalog "ecosystem_search" \
        '{"repository":"nodejs-18","page_size":3}' \
        "ecosystem_search: find nodejs-18 image" \
        "nodejs"

    assert_tool_contains rh-ecosystem-catalog "ecosystem_search" \
        '{"repository":"nonexistent-image-xyz-12345"}' \
        "ecosystem_search: handles no results" \
        "No container image"

    assert_tool_contains rh-ecosystem-catalog "ecosystem_operator" \
        '{"package":"amq-streams","page_size":3}' \
        "ecosystem_operator: find AMQ Streams" \
        "amq-streams"

    assert_tool_contains rh-ecosystem-catalog "ecosystem_operator" \
        '{"package":"elasticsearch-operator","page_size":3}' \
        "ecosystem_operator: find Elasticsearch operator" \
        "elasticsearch"

    assert_tool_contains rh-ecosystem-catalog "ecosystem_browse" \
        '{"type":"containers","page_size":3}' \
        "ecosystem_browse: list certified containers" \
        "certified container"

    assert_tool_contains rh-ecosystem-catalog "ecosystem_browse" \
        '{"type":"operators","page_size":3}' \
        "ecosystem_browse: list operator bundles" \
        "operator"

    echo ""
}

# --- Test Groups (OPP cluster — require credentials) ---

test_rhacs() {
    if [ -z "${RHACS_URL:-}" ] || [ -z "${RHACS_API_TOKEN:-}" ]; then
        skip_test "rhacs: all tests" "RHACS_URL and RHACS_API_TOKEN not set"
        return
    fi
    echo -e "${BOLD}rhacs${NC} (StackRox Central API)"
    local opts
    opts=$(printf '{"centralUrl":"%s","apiToken":"%s"}' "$RHACS_URL" "$RHACS_API_TOKEN")

    assert_tool_contains_opts rhacs "rhacs_image_scan" \
        '{"image":"registry.access.redhat.com/ubi9/ubi:latest"}' \
        "$opts" \
        "rhacs_image_scan: scan UBI9 image" \
        "CVE"

    assert_tool_contains_opts rhacs "rhacs_violations" \
        '{}' \
        "$opts" \
        "rhacs_violations: list violations" \
        "violation"

    assert_tool_contains_opts rhacs "rhacs_compliance_status" \
        '{}' \
        "$opts" \
        "rhacs_compliance_status: get compliance summary" \
        "compliance"

    assert_tool_not_contains_opts rhacs "rhacs_image_scan" \
        '{"image":"registry.access.redhat.com/ubi9/ubi:latest"}' \
        "$opts" \
        "rhacs_image_scan: not stub response" \
        "not configured"

    echo ""
}

test_quay() {
    if [ -z "${QUAY_URL:-}" ]; then
        skip_test "quay: all tests" "QUAY_URL not set"
        return
    fi
    echo -e "${BOLD}quay${NC} (Quay container registry)"
    local opts
    opts=$(printf '{"registryUrl":"%s","apiToken":"%s"}' "$QUAY_URL" "${QUAY_TOKEN:-}")

    assert_tool_contains_opts quay "quay_search" \
        '{"query":"ubi"}' \
        "$opts" \
        "quay_search: search for repos" \
        "repositor"

    assert_tool_contains_opts quay "quay_tags" \
        '{"repository":"projectquay/quay","namespace":"projectquay"}' \
        "$opts" \
        "quay_tags: list image tags" \
        "tag"

    assert_tool_contains_opts quay "quay_vulnerabilities" \
        '{"repository":"projectquay/quay","namespace":"projectquay","tag":"latest"}' \
        "$opts" \
        "quay_vulnerabilities: scan for vulnerabilities" \
        ""

    echo ""
}

test_ocp_obs_metrics() {
    if [ -z "${PROMETHEUS_HOST:-}" ] || [ -z "${OC_TOKEN:-}" ]; then
        skip_test "ocp-obs-metrics: all tests" "PROMETHEUS_HOST and OC_TOKEN not set"
        return
    fi
    echo -e "${BOLD}ocp-obs-metrics${NC} (Prometheus/Thanos)"
    local prom_url="https://$PROMETHEUS_HOST"
    local opts
    opts=$(printf '{"prometheusUrl":"%s","token":"%s"}' "$prom_url" "$OC_TOKEN")

    assert_tool_contains_opts ocp-obs-metrics "obs_promql" \
        '{"query":"up"}' \
        "$opts" \
        "obs_promql: instant query 'up'" \
        "vectors"

    assert_tool_contains_opts ocp-obs-metrics "obs_promql" \
        '{"query":"node_memory_MemTotal_bytes"}' \
        "$opts" \
        "obs_promql: query node memory" \
        "node_memory"

    assert_tool_contains_opts ocp-obs-metrics "obs_alerts" \
        '{}' \
        "$opts" \
        "obs_alerts: list firing alerts" \
        "alert"

    assert_tool_contains_opts ocp-obs-metrics "obs_health" \
        '{}' \
        "$opts" \
        "obs_health: services reachable" \
        "OK"

    echo ""
}

test_ocp_context_injection() {
    if ! command -v oc &>/dev/null; then
        skip_test "ocp-context-injection: all tests" "oc CLI not available"
        return
    fi
    if ! oc whoami &>/dev/null 2>&1; then
        skip_test "ocp-context-injection: all tests" "not logged in to cluster (oc whoami failed)"
        return
    fi
    echo -e "${BOLD}ocp-context-injection${NC} (cluster context via oc)"

    # This plugin populates state in the session.start hook, so we need a 3-step protocol:
    # initialize → hook/invoke(session.start) → tool/call(cluster_context)
    TOTAL=$((TOTAL + 1))
    local label="cluster_context: returns cluster context after session.start"
    local bin
    bin=$(build_plugin "ocp-context-injection") || {
        echo -e "  ${RED}FAIL${NC}: $label (build failed)"
        FAIL=$((FAIL + 1))
        RESULTS+=("FAIL  $label")
        return
    }

    local init_req='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"version":"1.0","directory":"/tmp"}}'
    local hook_req='{"jsonrpc":"2.0","id":2,"method":"hook/invoke","params":{"name":"session.start","input":{"sessionId":"test","directory":"/tmp"}}}'
    local tool_req='{"jsonrpc":"2.0","id":3,"method":"tool/call","params":{"name":"cluster_context","args":{},"context":{"sessionID":"test","directory":"/tmp"}}}'

    local output
    output=$(printf '%s\n%s\n%s\n' "$init_req" "$hook_req" "$tool_req" | timeout 60 "$bin" 2>/dev/null) || {
        echo -e "  ${RED}FAIL${NC}: $label (plugin error or timeout)"
        FAIL=$((FAIL + 1))
        RESULTS+=("FAIL  $label")
        return
    }

    local tool_resp
    tool_resp=$(echo "$output" | sed -n '3p')
    local content
    content=$(echo "$tool_resp" | jq -r '.result.content // .result.Content // .result // empty' 2>/dev/null)

    if echo "$content" | grep -qF "cluster-context" && ! echo "$content" | grep -qF "not connected"; then
        echo -e "  ${GREEN}PASS${NC}: $label"
        PASS=$((PASS + 1))
        RESULTS+=("PASS  $label")

        # Additional assertions on the populated context
        TOTAL=$((TOTAL + 1))
        if echo "$content" | grep -qi "version:"; then
            echo -e "  ${GREEN}PASS${NC}: cluster_context: includes version"
            PASS=$((PASS + 1))
            RESULTS+=("PASS  cluster_context: includes version")
        else
            echo -e "  ${RED}FAIL${NC}: cluster_context: includes version"
            FAIL=$((FAIL + 1))
            RESULTS+=("FAIL  cluster_context: includes version")
        fi

        TOTAL=$((TOTAL + 1))
        if echo "$content" | grep -qi "operators:"; then
            echo -e "  ${GREEN}PASS${NC}: cluster_context: includes operators"
            PASS=$((PASS + 1))
            RESULTS+=("PASS  cluster_context: includes operators")
        else
            echo -e "  ${RED}FAIL${NC}: cluster_context: includes operators"
            FAIL=$((FAIL + 1))
            RESULTS+=("FAIL  cluster_context: includes operators")
        fi
    else
        echo -e "  ${RED}FAIL${NC}: $label"
        echo "    Got: $(echo "$content" | head -c 200)"
        FAIL=$((FAIL + 1))
        RESULTS+=("FAIL  $label")
    fi

    echo ""
}

test_tekton() {
    if ! command -v oc &>/dev/null; then
        skip_test "tekton: all tests" "oc CLI not available"
        return
    fi
    if ! oc whoami &>/dev/null 2>&1; then
        skip_test "tekton: all tests" "not logged in to cluster (oc whoami failed)"
        return
    fi
    echo -e "${BOLD}tekton${NC} (OpenShift Pipelines)"

    assert_tool_contains tekton "tekton_list_pipelines" \
        '{"namespace":"openshift-pipelines"}' \
        "tekton_list_pipelines: list pipelines" \
        "pipeline"

    assert_tool_contains tekton "tekton_list_tasks" \
        '{"namespace":"openshift-pipelines"}' \
        "tekton_list_tasks: list tasks" \
        "task"

    assert_tool_contains tekton "tekton_list_runs" \
        '{"namespace":"openshift-pipelines"}' \
        "tekton_list_runs: list pipeline runs" \
        "run"

    echo ""
}

test_ocp_odf() {
    if ! command -v oc &>/dev/null; then
        skip_test "ocp-odf: all tests" "oc CLI not available"
        return
    fi
    if ! oc whoami &>/dev/null 2>&1; then
        skip_test "ocp-odf: all tests" "not logged in to cluster (oc whoami failed)"
        return
    fi
    if ! oc get crd storageclusters.ocs.openshift.io &>/dev/null 2>&1; then
        skip_test "ocp-odf: all tests" "ODF not installed (no storagecluster CRD)"
        return
    fi
    echo -e "${BOLD}ocp-odf${NC} (OpenShift Data Foundation)"

    assert_tool_contains ocp-odf "odf_status" \
        '{}' \
        "odf_status: StorageCluster status" \
        ""

    assert_tool_contains ocp-odf "odf_ceph_status" \
        '{}' \
        "odf_ceph_status: Ceph cluster health" \
        ""

    assert_tool_contains ocp-odf "odf_pools" \
        '{}' \
        "odf_pools: list Ceph pools" \
        ""

    assert_tool_contains ocp-odf "odf_storage_classes" \
        '{}' \
        "odf_storage_classes: list ODF storage classes" \
        ""

    assert_tool_contains ocp-odf "odf_pvcs" \
        '{"namespace":"all"}' \
        "odf_pvcs: list ODF-backed PVCs" \
        ""

    echo ""
}

test_ocp_virt() {
    if ! command -v oc &>/dev/null; then
        skip_test "ocp-virt: all tests" "oc CLI not available"
        return
    fi
    if ! oc whoami &>/dev/null 2>&1; then
        skip_test "ocp-virt: all tests" "not logged in to cluster (oc whoami failed)"
        return
    fi
    if ! oc get crd virtualmachines.kubevirt.io &>/dev/null 2>&1; then
        skip_test "ocp-virt: all tests" "OpenShift Virtualization not installed (no kubevirt CRD)"
        return
    fi
    echo -e "${BOLD}ocp-virt${NC} (OpenShift Virtualization)"

    assert_tool_contains ocp-virt "virt_vms" \
        '{"namespace":"all"}' \
        "virt_vms: list all VMs" \
        ""

    assert_tool_contains ocp-virt "virt_templates" \
        '{}' \
        "virt_templates: list VM templates" \
        ""

    assert_tool_contains ocp-virt "virt_datavolumes" \
        '{"namespace":"all"}' \
        "virt_datavolumes: list DataVolumes" \
        ""

    assert_tool_contains ocp-virt "virt_network" \
        '{"namespace":"all"}' \
        "virt_network: list NetworkAttachmentDefinitions" \
        ""

    echo ""
}

test_rhacm() {
    if ! command -v oc &>/dev/null; then
        skip_test "rhacm: all tests" "oc CLI not available"
        return
    fi
    if ! oc whoami &>/dev/null 2>&1; then
        skip_test "rhacm: all tests" "not logged in to cluster (oc whoami failed)"
        return
    fi
    echo -e "${BOLD}rhacm${NC} (Advanced Cluster Management)"
    local opts='{}'

    assert_tool_contains_opts rhacm "acm_clusters" \
        '{}' \
        "$opts" \
        "acm_clusters: list managed clusters" \
        "cluster"

    assert_tool_contains_opts rhacm "acm_policies" \
        '{}' \
        "$opts" \
        "acm_policies: list governance policies" \
        "polic"

    assert_tool_contains_opts rhacm "acm_cluster_detail" \
        '{"cluster":"local-cluster"}' \
        "$opts" \
        "acm_cluster_detail: get local-cluster details" \
        "cluster"

    assert_tool_contains_opts rhacm "acm_violations" \
        '{}' \
        "$opts" \
        "acm_violations: list policy violations" \
        "violation"

    echo ""
}

test_ocp_oauth() {
    if ! command -v oc &>/dev/null; then
        skip_test "ocp-oauth: all tests" "oc CLI not available"
        return
    fi
    if [ -z "${OCP_API_URL:-}" ] || [ -z "${OC_TOKEN:-}" ]; then
        skip_test "ocp-oauth: all tests" "OCP_API_URL and OC_TOKEN not set"
        return
    fi
    echo -e "${BOLD}ocp-oauth${NC} (OpenShift OAuth login)"
    local opts='{"insecureSkipTlsVerify":true}'

    assert_tool_contains_opts ocp-oauth "oc-login" \
        "$(jq -cn --arg server "$OCP_API_URL" --arg token "$OC_TOKEN" '{"server":$server,"token":$token}')" \
        "$opts" \
        "oc-login: authenticate to cluster" \
        "Logged into"

    echo ""
}

test_ocp_obs_logging() {
    if [ -z "${LOKI_URL:-}" ] || [ -z "${OC_TOKEN:-}" ]; then
        skip_test "ocp-obs-logging: all tests" "LOKI_URL and OC_TOKEN not set"
        return
    fi
    echo -e "${BOLD}ocp-obs-logging${NC} (Loki log queries)"
    local opts
    opts=$(jq -cn --arg url "$LOKI_URL" --arg token "$OC_TOKEN" \
        '{"lokiUrl":$url,"token":$token}')

    assert_tool_contains_opts ocp-obs-logging "obs_logs" \
        '{"query":"{log_type=~\".+\"}","limit":3}' \
        "$opts" \
        "obs_logs: query infrastructure logs" \
        "Log entries"

    assert_tool_contains_opts ocp-obs-logging "obs_traces" \
        '{"service":"test"}' \
        "$opts" \
        "obs_traces: returns unconfigured without tempoUrl" \
        "not configured"

    assert_tool_contains_opts ocp-obs-logging "obs_flow_collectors" \
        '{}' \
        "$opts" \
        "obs_flow_collectors: list network flow collectors" \
        ""

    assert_tool_contains_opts ocp-obs-logging "obs_dashboards" \
        '{}' \
        "$opts" \
        "obs_dashboards: list observability dashboards" \
        ""

    echo ""
}

test_aap_bridge() {
    if [ -z "${AAP_URL:-}" ] || [ -z "${AAP_TOKEN:-}" ]; then
        skip_test "aap-bridge: all tests" "AAP_URL and AAP_TOKEN not set"
        return
    fi
    echo -e "${BOLD}aap-bridge${NC} (Ansible Automation Platform)"
    local api_prefix="${AAP_API_PREFIX:-/api/v2}"
    local opts
    opts=$(jq -cn --arg url "$AAP_URL" --arg token "$AAP_TOKEN" --arg prefix "$api_prefix" \
        '{"controllerUrl":$url,"oauthToken":$token,"apiPrefix":$prefix}')

    assert_tool_contains_opts aap-bridge "aap_list_templates" \
        '{}' \
        "$opts" \
        "aap_list_templates: list job templates" \
        "template"

    assert_tool_contains_opts aap-bridge "aap_list_inventories" \
        '{}' \
        "$opts" \
        "aap_list_inventories: list inventories" \
        "inventor"

    assert_tool_contains_opts aap-bridge "aap_job_status" \
        '{"jobId":999}' \
        "$opts" \
        "aap_job_status: error on missing job" \
        "404"

    echo ""
}

test_rhoai_model_serving() {
    if ! command -v oc &>/dev/null; then
        skip_test "rhoai-model-serving: all tests" "oc CLI not available"
        return
    fi
    if ! oc whoami &>/dev/null 2>&1; then
        skip_test "rhoai-model-serving: all tests" "not logged in to cluster (oc whoami failed)"
        return
    fi
    echo -e "${BOLD}rhoai-model-serving${NC} (OpenShift AI model serving)"

    assert_tool_contains rhoai-model-serving "rhoai_list_models" \
        '{"namespace":"my-first-model"}' \
        "rhoai_list_models: list inference services" \
        "Inference Services"

    assert_tool_contains rhoai-model-serving "rhoai_model_status" \
        '{"name":"llama-32-3b-instruct","namespace":"my-first-model"}' \
        "rhoai_model_status: get model status with pods" \
        "Ready"

    assert_tool_contains rhoai-model-serving "rhoai_list_runtimes" \
        '{"namespace":"my-first-model"}' \
        "rhoai_list_runtimes: list serving runtimes" \
        "Serving Runtimes"

    assert_tool_contains rhoai-model-serving "rhoai_sandbox_status" \
        '{}' \
        "rhoai_sandbox_status: unconfigured returns message" \
        "not configured"

    assert_tool_contains rhoai-model-serving "rhoai_health" \
        '{}' \
        "rhoai_health: cluster API reachable" \
        "OK"

    echo ""
}

test_rhoai_eval_trustyai() {
    if ! command -v oc &>/dev/null; then
        skip_test "rhoai-eval-trustyai: all tests" "oc CLI not available"
        return
    fi
    if ! oc whoami &>/dev/null 2>&1; then
        skip_test "rhoai-eval-trustyai: all tests" "not logged in to cluster (oc whoami failed)"
        return
    fi
    echo -e "${BOLD}rhoai-eval-trustyai${NC} (OpenShift AI evaluation + TrustyAI)"

    assert_tool_contains rhoai-eval-trustyai "rhoai_eval_run" \
        '{"model":"test","provider":"vllm"}' \
        "rhoai_eval_run: unconfigured returns message" \
        "not configured"

    assert_tool_contains rhoai-eval-trustyai "rhoai_trusty_metrics" \
        '{"model":"test"}' \
        "rhoai_trusty_metrics: unconfigured returns message" \
        "not configured"

    assert_tool_contains rhoai-eval-trustyai "rhoai_workbench_list" \
        '{}' \
        "rhoai_workbench_list: list workbenches (oc)" \
        "workbench"

    assert_tool_contains rhoai-eval-trustyai "rhoai_eval_health" \
        '{}' \
        "rhoai_eval_health: health check reports status" \
        "Service Health"

    echo ""
}

test_rhoai_pipelines() {
    if [ -z "${PIPELINES_URL:-}" ]; then
        skip_test "rhoai-pipelines: all tests" "PIPELINES_URL not set"
        return
    fi
    echo -e "${BOLD}rhoai-pipelines${NC} (Kubeflow pipelines on OpenShift AI)"
    local opts
    opts=$(printf '{"pipelinesUrl":"%s","token":"%s"}' "$PIPELINES_URL" "${OC_TOKEN:-}")

    assert_tool_contains_opts rhoai-pipelines "rhoai_pipeline_list" \
        '{}' \
        "$opts" \
        "rhoai_pipeline_list: list pipelines" \
        "ipeline"

    assert_tool_contains_opts rhoai-pipelines "rhoai_pipeline_run" \
        '{}' \
        "$opts" \
        "rhoai_pipeline_run: list pipeline runs" \
        ""

    echo ""
}

test_rhoai_mlflow_tools() {
    if [ -z "${MLFLOW_URL:-}" ]; then
        skip_test "rhoai-mlflow-tools: all tests" "MLFLOW_URL not set"
        return
    fi
    echo -e "${BOLD}rhoai-mlflow-tools${NC} (MLflow on OpenShift AI)"
    local opts
    opts=$(printf '{"mlflowUrl":"%s"}' "$MLFLOW_URL")

    assert_tool_contains_opts rhoai-mlflow-tools "mlflow_experiments" \
        '{}' \
        "$opts" \
        "mlflow_experiments: list experiments" \
        "xperiment"

    assert_tool_contains_opts rhoai-mlflow-tools "mlflow_runs" \
        '{"experimentId":"0"}' \
        "$opts" \
        "mlflow_runs: list runs in default experiment" \
        ""

    assert_tool_contains_opts rhoai-mlflow-tools "mlflow_model_registry" \
        '{}' \
        "$opts" \
        "mlflow_model_registry: list registered models" \
        ""

    echo ""
}

test_satellite() {
    if [ -z "${SATELLITE_URL:-}" ]; then
        skip_test "satellite: all tests" "SATELLITE_URL not set"
        return
    fi
    if [ -z "${SATELLITE_USER:-}" ] || [ -z "${SATELLITE_PASSWORD:-}" ]; then
        skip_test "satellite: all tests" "SATELLITE_USER and SATELLITE_PASSWORD not set"
        return
    fi
    echo -e "${BOLD}satellite${NC} (Red Hat Satellite)"
    local opts
    opts=$(jq -cn --arg url "$SATELLITE_URL" --arg user "$SATELLITE_USER" --arg pass "$SATELLITE_PASSWORD" \
        '{"satelliteUrl":$url,"username":$user,"password":$pass}')

    assert_tool_contains_opts satellite "satellite_health_check" \
        '{}' \
        "$opts" \
        "satellite_health_check: multi-port connectivity" \
        "Satellite Health Check"

    assert_tool_contains_opts satellite "satellite_hosts" \
        '{}' \
        "$opts" \
        "satellite_hosts: list managed hosts" \
        "Hosts:"

    assert_tool_contains_opts satellite "satellite_host_facts" \
        '{"hostname":"satellite.wgvcz.sandbox5406.opentlc.com","search":"memory"}' \
        "$opts" \
        "satellite_host_facts: get host memory facts" \
        "memorysize"

    assert_tool_contains_opts satellite "satellite_errata" \
        '{"type":"security"}' \
        "$opts" \
        "satellite_errata: list security errata" \
        "Errata:"

    assert_tool_contains_opts satellite "satellite_content_views" \
        '{}' \
        "$opts" \
        "satellite_content_views: list content views" \
        "Content views:"

    assert_tool_contains_opts satellite "satellite_services" \
        '{}' \
        "$opts" \
        "satellite_services: check service health" \
        "Katello overall:"

    assert_tool_contains_opts satellite "satellite_proxies" \
        '{}' \
        "$opts" \
        "satellite_proxies: list smart proxies" \
        "Smart proxies:"

    assert_tool_contains_opts satellite "satellite_tasks" \
        '{"perPage":3}' \
        "$opts" \
        "satellite_tasks: list recent tasks" \
        "Tasks:"

    assert_tool_contains_opts satellite "satellite_repositories" \
        '{}' \
        "$opts" \
        "satellite_repositories: list repos" \
        "Repositories"

    assert_tool_contains_opts satellite "satellite_rex_run" \
        '{"host":"name = satellite.wgvcz.internal","command":"hostname -f"}' \
        "$opts" \
        "satellite_rex_run: run remote command" \
        "Job submitted"

    assert_tool_contains_opts satellite "satellite_rex_result" \
        '{"jobId":3}' \
        "$opts" \
        "satellite_rex_result: get job output" \
        "REX Job"

    echo ""
}

test_cluster_ops() {
    if ! command -v oc &>/dev/null; then
        skip_test "cluster-ops: all tests" "oc CLI not available"
        return
    fi
    if ! oc whoami &>/dev/null 2>&1; then
        skip_test "cluster-ops: all tests" "not logged in to cluster (oc whoami failed)"
        return
    fi
    echo -e "${BOLD}cluster-ops${NC} (basic cluster operations)"
    local opts='{"insecureSkipTLSVerify":true}'

    assert_tool_contains_opts cluster-ops "oc-status" \
        '{}' \
        "$opts" \
        "oc-status: show cluster status" \
        "project"

    assert_tool_contains_opts cluster-ops "cluster-info" \
        '{}' \
        "$opts" \
        "cluster-info: show cluster info" \
        "running"

    assert_tool_not_contains_opts cluster-ops "oc-status" \
        '{}' \
        "$opts" \
        "oc-status: not showing error" \
        "error"

    echo ""
}

# --- Test Groups (console.redhat.com — require OCM token) ---

test_rh_api_catalog() {
    local ocm_config="$HOME/Library/Application Support/ocm/ocm.json"
    if [ ! -f "$ocm_config" ]; then
        ocm_config="$HOME/.config/ocm/ocm.json"
    fi
    if [ ! -f "$ocm_config" ]; then
        skip_test "rh-api-catalog: all tests" "ocm not logged in (no ocm.json found)"
        return
    fi
    local refresh_token
    refresh_token=$(jq -r '.refresh_token // empty' "$ocm_config" 2>/dev/null)
    if [ -z "$refresh_token" ]; then
        skip_test "rh-api-catalog: all tests" "no refresh_token in ocm.json"
        return
    fi

    echo -e "${BOLD}rh-api-catalog${NC} (console.redhat.com APIs)"
    local opts
    opts=$(jq -cn --arg token "$refresh_token" '{"consoleOfflineToken":$token,"clientId":"ocm-cli"}')

    # Static catalog (no auth needed)
    assert_tool_contains_opts rh-api-catalog "rh_api_list" \
        '{"search":"cost"}' \
        "$opts" \
        "rh_api_list: search catalog for 'cost'" \
        "cost-management"

    assert_tool_contains_opts rh-api-catalog "rh_api_list" \
        '{}' \
        "$opts" \
        "rh_api_list: list all APIs" \
        "rbac"

    # Validate token before auth-dependent tests
    local token_valid=true
    if ! curl -sf -o /dev/null -d "grant_type=refresh_token&client_id=ocm-cli&refresh_token=$refresh_token" \
        "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token" 2>/dev/null; then
        token_valid=false
    fi

    if [ "$token_valid" = false ]; then
        skip_test "rh_api_spec: fetch RBAC spec (live)" "OCM token expired (run 'ocm login' to refresh)"
        skip_test "rh_api_endpoints: list RBAC endpoints" "OCM token expired"
        skip_test "rh_api_spec: not showing 'not available' message" "OCM token expired"
        echo ""
        return
    fi

    # Live API spec fetch (requires auth)
    assert_tool_contains_opts rh-api-catalog "rh_api_spec" \
        '{"api":"rbac"}' \
        "$opts" \
        "rh_api_spec: fetch RBAC spec (live)" \
        "components"

    assert_tool_contains_opts rh-api-catalog "rh_api_endpoints" \
        '{"api":"rbac"}' \
        "$opts" \
        "rh_api_endpoints: list RBAC endpoints" \
        "GET"

    assert_tool_not_contains_opts rh-api-catalog "rh_api_spec" \
        '{"api":"rbac"}' \
        "$opts" \
        "rh_api_spec: not showing 'not available' message" \
        "not available"

    echo ""
}

test_rhdp_provisioner() {
    if [ -z "${RHDP_SESSION_COOKIE:-}" ]; then
        skip_test "rhdp-provisioner: all tests" "RHDP_SESSION_COOKIE not set (extract from browser at demo.redhat.com)"
        return
    fi
    echo -e "${BOLD}rhdp-provisioner${NC} (demo.redhat.com)"
    local opts
    opts=$(jq -cn --arg cookie "$RHDP_SESSION_COOKIE" '{"sessionCookie":$cookie}')

    assert_tool_contains_opts rhdp-provisioner "rhdp_search" \
        '{"query":"openshift"}' \
        "$opts" \
        "rhdp_search: search catalog for openshift" \
        "openshift"

    assert_tool_contains_opts rhdp-provisioner "rhdp_list_active" \
        '{}' \
        "$opts" \
        "rhdp_list_active: list active environments" \
        ""

    echo ""
}

# --- Test Groups (HTTP-based — require credentials) ---

test_rhdh() {
    if [ -z "${RHDH_URL:-}" ]; then
        skip_test "rhdh: all tests" "RHDH_URL not set"
        return
    fi
    echo -e "${BOLD}rhdh${NC} (Red Hat Developer Hub catalog)"
    local opts
    opts=$(jq -cn --arg url "$RHDH_URL" --arg token "${RHDH_TOKEN:-}" \
        '{"baseUrl":$url,"apiToken":$token}')

    assert_tool_contains_opts rhdh "rhdh_catalog_search" \
        '{"kind":"Component"}' \
        "$opts" \
        "rhdh_catalog_search: search components" \
        ""

    assert_tool_contains_opts rhdh "rhdh_catalog_entity" \
        '{"kind":"Component","name":"test"}' \
        "$opts" \
        "rhdh_catalog_entity: get entity details" \
        ""

    assert_tool_contains_opts rhdh "rhdh_api_spec" \
        '{"name":"test"}' \
        "$opts" \
        "rhdh_api_spec: fetch API spec" \
        ""

    assert_tool_contains_opts rhdh "rhdh_techdocs" \
        '{"kind":"Component","name":"test"}' \
        "$opts" \
        "rhdh_techdocs: fetch TechDocs content" \
        ""

    assert_tool_contains_opts rhdh "rhdh_dependencies" \
        '{"kind":"Component","name":"test"}' \
        "$opts" \
        "rhdh_dependencies: get entity dependencies" \
        ""

    assert_tool_not_contains_opts rhdh "rhdh_catalog_search" \
        '{"kind":"Component"}' \
        "$opts" \
        "rhdh_catalog_search: not stub response" \
        "not configured"

    echo ""
}

test_rhoai_experiment_tracker() {
    if [ -z "${RHOAI_EXPERIMENT_TRACKER_URL:-}" ]; then
        skip_test "rhoai-experiment-tracker: all tests" "RHOAI_EXPERIMENT_TRACKER_URL not set"
        return
    fi
    echo -e "${BOLD}rhoai-experiment-tracker${NC} (MLflow experiment tracking)"
    local opts
    opts=$(jq -cn --arg url "$RHOAI_EXPERIMENT_TRACKER_URL" \
        '{"mlflowUrl":$url}')

    assert_tool_contains_opts rhoai-experiment-tracker "experiment_last_session" \
        '{}' \
        "$opts" \
        "experiment_last_session: get last session info" \
        ""

    assert_tool_not_contains_opts rhoai-experiment-tracker "experiment_last_session" \
        '{}' \
        "$opts" \
        "experiment_last_session: not stub response" \
        "not configured"

    echo ""
}

test_rhoai_mcp_bridge() {
    if [ -z "${RHOAI_MCP_BRIDGE_URL:-}" ]; then
        skip_test "rhoai-mcp-bridge: all tests" "RHOAI_MCP_BRIDGE_URL not set"
        return
    fi
    echo -e "${BOLD}rhoai-mcp-bridge${NC} (MCP server bridge)"
    local opts
    opts=$(jq -cn --arg url "$RHOAI_MCP_BRIDGE_URL" --arg token "${RHOAI_MCP_BRIDGE_TOKEN:-}" \
        '{"mcpServerUrl":$url,"oauthToken":$token}')

    assert_tool_contains_opts rhoai-mcp-bridge "rhoai_mcp_list" \
        '{}' \
        "$opts" \
        "rhoai_mcp_list: list available MCP tools" \
        ""

    assert_tool_contains_opts rhoai-mcp-bridge "rhoai_mcp_call" \
        '{"tool":"test"}' \
        "$opts" \
        "rhoai_mcp_call: call an MCP tool" \
        ""

    assert_tool_not_contains_opts rhoai-mcp-bridge "rhoai_mcp_list" \
        '{}' \
        "$opts" \
        "rhoai_mcp_list: not stub response" \
        "not configured"

    echo ""
}

test_lightwell() {
    if [ -z "${LIGHTWELL_TOKEN:-}" ]; then
        skip_test "lightwell: all tests" "LIGHTWELL_TOKEN not set"
        return
    fi
    echo -e "${BOLD}lightwell${NC} (Red Hat Lightwell package security)"
    local opts
    opts=$(jq -cn --arg token "$LIGHTWELL_TOKEN" \
        '{"serviceAccountToken":$token}')

    assert_tool_contains_opts lightwell "lightwell_check_package" \
        '{"ecosystem":"python","name":"requests","version":"2.28.0"}' \
        "$opts" \
        "lightwell_check_package: check python package" \
        ""

    assert_tool_contains_opts lightwell "lightwell_osv" \
        '{"ecosystem":"python","name":"requests","version":"2.28.0"}' \
        "$opts" \
        "lightwell_osv: query OSV vulnerabilities" \
        ""

    assert_tool_contains_opts lightwell "lightwell_config_check" \
        '{"content":"[global]\nindex-url = https://pypi.org/simple/","fileType":"pip.conf"}' \
        "$opts" \
        "lightwell_config_check: analyze pip config" \
        ""

    assert_tool_contains_opts lightwell "lightwell_check_deps" \
        '{"content":"requests==2.28.0","fileType":"requirements.txt"}' \
        "$opts" \
        "lightwell_check_deps: check requirements.txt deps" \
        ""

    assert_tool_not_contains_opts lightwell "lightwell_check_package" \
        '{"ecosystem":"python","name":"requests","version":"2.28.0"}' \
        "$opts" \
        "lightwell_check_package: not stub response" \
        "not configured"

    echo ""
}

# --- Prerequisites ---

check_prereqs() {
    if ! command -v jq &>/dev/null; then
        echo "Error: jq is required (brew install jq)" >&2
        exit 1
    fi
    if ! command -v go &>/dev/null; then
        echo "Error: go is required" >&2
        exit 1
    fi
}

# --- Main ---

main() {
    echo -e "${BOLD}tinycode plugin functional tests${NC}"
    echo "===================================="
    echo "  Timeout: ${TIMEOUT}s per test"
    echo ""

    if [ "${1:-}" = "--list" ]; then
        echo "Public API (no credentials):"
        echo "  web-search           DuckDuckGo web search + RH KB search"
        echo "  rh-dev-content       Red Hat developer articles"
        echo "  rh-ecosystem-catalog Red Hat ecosystem catalog / Pyxis"
        echo ""
        echo "console.redhat.com (require OCM login):"
        echo "  rh-api-catalog       Red Hat API catalog (ocm token)"
        echo "  rhdp-provisioner     Red Hat Demo Platform (RHDP-specific auth)"
        echo ""
        echo "OPP cluster (require credentials):"
        echo "  rhacs                StackRox Central (RHACS_URL, RHACS_API_TOKEN)"
        echo "  quay                 Quay registry (QUAY_URL, QUAY_TOKEN)"
        echo "  ocp-obs-metrics      Prometheus/Thanos (PROMETHEUS_HOST, OC_TOKEN)"
        echo "  ocp-oauth            OpenShift OAuth login (OCP_API_URL, OC_TOKEN)"
        echo "  ocp-obs-logging      Loki log queries (LOKI_URL, OC_TOKEN)"
        echo "  aap-bridge           Ansible Automation Platform (AAP_URL, AAP_TOKEN)"
        echo "  ocp-context-injection  Cluster context via oc CLI"
        echo "  tekton               OpenShift Pipelines via oc CLI"
        echo "  ocp-odf              OpenShift Data Foundation via oc CLI"
        echo "  ocp-virt             OpenShift Virtualization via oc CLI"
        echo "  rhacm                Advanced Cluster Management via oc CLI"
        echo "  cluster-ops          Basic cluster operations via oc/kubectl"
        echo ""
        echo "RHOAI cluster (require oc login or credentials):"
        echo "  rhoai-model-serving  RHOAI model serving via oc CLI"
        echo "  rhoai-eval-trustyai  RHOAI evaluation + TrustyAI via oc CLI"
        echo "  rhoai-pipelines      Kubeflow pipelines (PIPELINES_URL)"
        echo "  rhoai-mlflow-tools   MLflow tools (MLFLOW_URL)"
        echo "  rhoai-experiment-tracker  MLflow experiment tracking (RHOAI_EXPERIMENT_TRACKER_URL)"
        echo "  rhoai-mcp-bridge     MCP server bridge (RHOAI_MCP_BRIDGE_URL)"
        echo "  satellite            Satellite hosts, errata, content views (SATELLITE_URL, SATELLITE_USER, SATELLITE_PASSWORD)"
        echo ""
        echo "HTTP-based (require credentials):"
        echo "  rhdh                 Red Hat Developer Hub catalog (RHDH_URL, RHDH_TOKEN)"
        echo "  lightwell            Red Hat Lightwell package security (LIGHTWELL_TOKEN)"
        exit 0
    fi

    check_prereqs

    local filter="${1:-}"

    # Public API tests
    if [ -z "$filter" ] || [ "$filter" = "web-search" ]; then
        test_web_search
    fi
    if [ -z "$filter" ] || [ "$filter" = "rh-dev-content" ]; then
        test_rh_dev_content
    fi
    if [ -z "$filter" ] || [ "$filter" = "rh-ecosystem-catalog" ]; then
        test_rh_ecosystem_catalog
    fi

    # console.redhat.com tests (OCM token)
    if [ -z "$filter" ] || [ "$filter" = "rh-api-catalog" ]; then
        test_rh_api_catalog
    fi
    if [ -z "$filter" ] || [ "$filter" = "rhdp-provisioner" ]; then
        test_rhdp_provisioner
    fi

    # OPP cluster tests
    if [ -z "$filter" ] || [ "$filter" = "rhacs" ]; then
        test_rhacs
    fi
    if [ -z "$filter" ] || [ "$filter" = "quay" ]; then
        test_quay
    fi
    if [ -z "$filter" ] || [ "$filter" = "ocp-obs-metrics" ]; then
        test_ocp_obs_metrics
    fi
    if [ -z "$filter" ] || [ "$filter" = "ocp-oauth" ]; then
        test_ocp_oauth
    fi
    if [ -z "$filter" ] || [ "$filter" = "ocp-obs-logging" ]; then
        test_ocp_obs_logging
    fi
    if [ -z "$filter" ] || [ "$filter" = "aap-bridge" ]; then
        test_aap_bridge
    fi
    if [ -z "$filter" ] || [ "$filter" = "ocp-context-injection" ]; then
        test_ocp_context_injection
    fi
    if [ -z "$filter" ] || [ "$filter" = "tekton" ]; then
        test_tekton
    fi
    if [ -z "$filter" ] || [ "$filter" = "ocp-odf" ]; then
        test_ocp_odf
    fi
    if [ -z "$filter" ] || [ "$filter" = "ocp-virt" ]; then
        test_ocp_virt
    fi
    if [ -z "$filter" ] || [ "$filter" = "rhacm" ]; then
        test_rhacm
    fi
    if [ -z "$filter" ] || [ "$filter" = "cluster-ops" ]; then
        test_cluster_ops
    fi
    if [ -z "$filter" ] || [ "$filter" = "satellite" ]; then
        test_satellite
    fi

    # RHOAI tests
    if [ -z "$filter" ] || [ "$filter" = "rhoai-model-serving" ]; then
        test_rhoai_model_serving
    fi
    if [ -z "$filter" ] || [ "$filter" = "rhoai-eval-trustyai" ]; then
        test_rhoai_eval_trustyai
    fi
    if [ -z "$filter" ] || [ "$filter" = "rhoai-pipelines" ]; then
        test_rhoai_pipelines
    fi
    if [ -z "$filter" ] || [ "$filter" = "rhoai-mlflow-tools" ]; then
        test_rhoai_mlflow_tools
    fi
    if [ -z "$filter" ] || [ "$filter" = "rhoai-experiment-tracker" ]; then
        test_rhoai_experiment_tracker
    fi
    if [ -z "$filter" ] || [ "$filter" = "rhoai-mcp-bridge" ]; then
        test_rhoai_mcp_bridge
    fi

    # HTTP-based tests
    if [ -z "$filter" ] || [ "$filter" = "rhdh" ]; then
        test_rhdh
    fi
    if [ -z "$filter" ] || [ "$filter" = "lightwell" ]; then
        test_lightwell
    fi

    echo "===================================="
    echo -e "  ${GREEN}$PASS passed${NC}  ${RED}$FAIL failed${NC}  ${YELLOW}$SKIP skipped${NC}  (of $TOTAL)"
    echo ""
    for r in "${RESULTS[@]}"; do
        echo "  $r"
    done
    echo ""

    [ "$FAIL" -eq 0 ]
}

main "$@"

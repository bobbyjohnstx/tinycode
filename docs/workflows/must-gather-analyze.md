# Workflow: Must-Gather Cluster Analysis

Systematic offline analysis of an OpenShift cluster using a must-gather archive.

## Required plugins

```json
{
  "plugins": ["ocp-must-gather"]
}

Optional for deeper analysis:
- `etcd-diag` — etcd performance deep-dive
- `ingress-inspect` — HAProxy/Ingress detailed inspection
```

## Workflow

### 1. Load the must-gather

```
Use mg_use to set the must-gather path to /path/to/must-gather
```

The tool validates the directory structure and reports what data is available (monitoring, networking, etcd logs, etc.).

### 2. Start with the health summary

```
Run mg_health for an overall cluster health summary
```

This aggregates checks from other tools and flags `[OK]`, `[WARN]`, or `[CRITICAL]` per area. Use it to prioritize where to dig deeper.

### 3. Check cluster version and upgrade status

```
Run mg_cluster_version to see version, update channel, and upgrade history
```

Partial upgrades or stuck updates are a common root cause for other symptoms.

### 4. Inspect nodes

```
Run mg_nodes to list nodes with status, roles, conditions, and capacity
```

Look for `NotReady` nodes, memory/disk pressure conditions, or unbalanced workload distribution.

### 5. Check operators

```
Run mg_operators to list ClusterOperators with available/degraded/progressing status
```

Degraded operators are the most common signal. Cross-reference with events and pod status.

### 6. Review certificates

```
Run mg_certs to check certificate expiry across the cluster
```

Expired or soon-to-expire certificates cause cascading failures.

### 7. Search events

```
Run mg_events filtered by type "Warning" to see recent cluster events
```

Events provide the timeline. Filter by namespace if you already suspect a specific area.

### 8. Check pod health

```
Run mg_pods to list pods with status, restarts, and resource usage
```

High restart counts, `CrashLoopBackOff`, or `OOMKilled` indicate specific workload issues.

### 9. Deep-dive areas (as needed)

| Area | Tool | When to use |
|------|------|-------------|
| Node journal logs | `mg_node_logs` | Kernel panics, kubelet issues, systemd failures |
| OVN networking | `mg_ovn` | EgressIP issues, NetworkPolicy problems, pod connectivity |
| Prometheus metrics | `mg_prometheus` | Historical metric data from the monitoring stack |
| MachineConfig | `mg_machine_config` | MCO degraded, config drift, pool update issues |
| etcd performance | `etcd_diag_*` tools | See [diagnose-etcd.md](diagnose-etcd.md) |
| Ingress/HAProxy | `ingress_*` tools | See [diagnose-ingress.md](diagnose-ingress.md) |

## Triage priority

1. **Cluster version** — is the upgrade stuck?
2. **Operators** — which are degraded?
3. **Nodes** — any NotReady or under pressure?
4. **Certificates** — any expired?
5. **Events + Pods** — what's crashing and why?
6. **Infrastructure** — etcd, networking, storage

## Related

- [diagnose-etcd.md](diagnose-etcd.md) — etcd-specific deep-dive
- [diagnose-ingress.md](diagnose-ingress.md) — ingress/HAProxy deep-dive

# Workflow: Diagnose etcd Issues

Investigate etcd performance problems — slow writes, leader elections, and cross-pod divergence — using must-gather data.

## Required plugins

```json
{
  "plugins": ["ocp-must-gather", "etcd-diag"]
}
```

## Workflow

### 1. Load the must-gather

Point `ocp-must-gather` at your must-gather directory:

```
Use mg_use to set the must-gather path to /path/to/must-gather
```

### 2. Check cluster health first

Get the overall picture before diving into etcd:

```
Run mg_health to get the cluster health summary
```

Look for `[WARN]` or `[CRITICAL]` indicators in the etcd section.

### 3. Pull etcd statistics

```
Run etcd_diag_stats to parse etcd pod logs and extract performance statistics
```

Key metrics to watch:
- **Slow write counts** > 0 indicates disk pressure or network latency
- **Slow fsync counts** > 0 indicates storage backend issues
- **Compaction duration** — high median suggests large keyspace or slow storage

### 4. Check for errors

```
Run etcd_diag_errors to extract and categorize error messages
```

Error categories: auth failures, storage errors, raft errors, network errors. High counts in any category narrow the investigation.

### 5. Build a timeline

```
Run etcd_diag_timeline to see significant events in chronological order
```

Correlate leader elections with slow writes or compactions. Frequent leader changes indicate network instability or resource contention.

### 6. Cross-pod comparison

```
Run etcd_diag_compare to see per-pod metrics side by side
```

If one pod has significantly higher slow apply or fsync counts than others, the issue is node-specific (disk, CPU, network on that node). If all pods show similar numbers, it's cluster-wide (load, keyspace size).

### 7. Get the overall etcd verdict

```
Run etcd_diag_health for a summary with OK/WARN/CRITICAL per dimension
```

## Decision tree

| Symptom | Likely cause | Next step |
|---------|-------------|-----------|
| One pod high slow fsyncs, others fine | Disk issue on that node | Check node storage, consider replacement |
| All pods high slow fsyncs | Cluster-wide storage pressure | Check PV performance, etcd defrag |
| Frequent leader elections | Network instability | Check node-to-node latency, CNI health |
| Large compaction durations | Keyspace bloat | Check etcd size, defragment, tune compaction |

## Related

- [must-gather-analyze.md](must-gather-analyze.md) — broader cluster analysis
- `ocp-obs-metrics` plugin — live Prometheus etcd metrics (when cluster is accessible)

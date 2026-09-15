# Workflow: Diagnose Ingress / HAProxy Issues

Troubleshoot OpenShift Ingress and HAProxy routing problems — stale backends, route misconfigurations, certificate issues — using must-gather data.

## Required plugins

```json
{
  "plugins": ["ocp-must-gather", "ingress-inspect"]
}
```

## Workflow

### 1. Load the must-gather

```
Use mg_use to set the must-gather path to /path/to/must-gather
```

### 2. Get the ingress health summary

```
Run ingress_health for an overall ingress health check
```

This checks IngressController availability, HAProxy config validity, router pod logs, and certificate status. Start here to identify which area needs attention.

### 3. List IngressControllers

```
Run ingress_controllers to list all IngressController CRs
```

Key fields: name, domain, replicas, endpoint publishing strategy, default certificate. Multiple IngressControllers (e.g., default + internal sharding) are common — make sure each is healthy.

### 4. Parse HAProxy backends

```
Run ingress_backends to list all HAProxy backends with server counts
```

Shows backend name, server count, mode (http/tcp), and balance algorithm. Look for:
- Backends with 0 servers (no pods behind the route)
- Unexpected balance algorithms
- Large server counts (potential scaling issues)

### 5. Cross-reference routes with HAProxy

```
Run ingress_route_check to detect misconfigurations
```

This is the most diagnostic tool. It cross-references Route resources against the HAProxy config and detects:
- **Stale backends** — HAProxy config references routes that no longer exist
- **Missing routes** — routes defined in the cluster but missing from HAProxy
- **Misconfigurations** — TLS termination mismatches, weight issues

### 6. Inspect HAProxy configuration

```
Run ingress_config to show global/defaults configuration
```

Check timeouts, maxconn, SSL settings. Common issues:
- `timeout server` too low for long-running requests
- `maxconn` too low under load
- SSL/TLS version mismatches

## Common issues and fixes

| Symptom | Tool to use | Likely cause |
|---------|-------------|-------------|
| 503 errors on specific routes | `ingress_route_check` | Stale backend, no pods behind route |
| Intermittent 503s under load | `ingress_config` | `maxconn` too low or timeout too short |
| Routes not accessible externally | `ingress_controllers` | IngressController not publishing endpoints |
| TLS handshake failures | `ingress_config`, `ingress_health` | Certificate expired or TLS version mismatch |
| Routes work on one IC but not another | `ingress_backends` | Route label selector doesn't match IC shard |

## Related

- [must-gather-analyze.md](must-gather-analyze.md) — broader cluster analysis
- `ocp-obs-logging` plugin — live HAProxy access logs via Loki (when cluster is accessible)

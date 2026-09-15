# Workflow: Security Audit

Comprehensive security posture assessment using RHACS, Lightwell, and container-linter together.

## Required plugins

```json
{
  "plugins": [
    "rhacs",
    "lightwell",
    "container-linter",
    "ocp-context-injection"
  ]
}
```

## Plugin configuration

RHACS requires connection details:
```json
{
  "plugins": [
    ["rhacs", { "centralUrl": "https://central-stackrox.apps.cluster.example.com", "apiToken": "..." }],
    "lightwell",
    "container-linter",
    "ocp-context-injection"
  ]
}
```

## Workflow

### Phase 1: Container image security

#### 1.1 Scan images for CVEs

```
Use rhacs_image_scan to scan your container images for known vulnerabilities
```

Start with your most critical images (production workloads, base images). The scan returns CVEs sorted by severity with fix availability.

#### 1.2 Check images against deploy-time policies

```
Use rhacs_image_check to validate images against your organization's security policies
```

This catches policy violations that aren't CVEs — unsigned images, disallowed base images, running as root.

#### 1.3 Scan Containerfiles for best practices

```
Use container_lint to lint your Containerfiles against Red Hat best practices
```

Catches: non-UBI base images, missing labels, running as root, hardcoded secrets, multi-stage build issues.

#### 1.4 Validate bootc images (if applicable)

```
Use bootc_validate to check bootc-compatible image builds
```

### Phase 2: Supply chain and dependencies

#### 2.1 Check package provenance

```
Use lightwell_provenance to verify SLSA Level 3 build provenance for packages
```

#### 2.2 Scan dependencies for vulnerabilities

```
Use lightwell_check_deps to scan pom.xml or requirements.txt for known issues
```

#### 2.3 Check individual packages

```
Use lightwell_check_package to check a specific package against Lightwell repos
```

#### 2.4 Query OSV vulnerability data

```
Use lightwell_osv to query the Open Source Vulnerability database for a package
```

### Phase 3: Runtime security posture

#### 3.1 Check deployment configurations

```
Use rhacs_deployment_check to validate deployment YAML against security policies
```

Feed your Kubernetes manifests through this before deploying.

#### 3.2 Review active violations

```
Use rhacs_violations to list active policy violations across the cluster
```

#### 3.3 Assess deployment risk

```
Use rhacs_risk to get risk scores and contributing factors for deployments
```

#### 3.4 Run compliance scan

```
Use rhacs_compliance_scan to trigger a compliance scan, then
rhacs_compliance_status to check results by standard (CIS, NIST, PCI)
```

### Phase 4: Build pipeline audit

#### 4.1 Audit build configuration

```
Use lightwell_config_check to audit build config for Lightwell repo setup
```

#### 4.2 Scan Containerfiles in the pipeline

```
Use lightwell_scan_containerfile to check Containerfiles for dependency and base image issues
```

#### 4.3 Suggest base images

```
Use container_base_suggest to get UBI base image recommendations for your use case
```

## Audit checklist

| Area | Tools | What to check |
|------|-------|---------------|
| Image CVEs | `rhacs_image_scan` | Critical/High CVEs with available fixes |
| Policy compliance | `rhacs_image_check`, `rhacs_deployment_check` | Unsigned images, root containers, disallowed packages |
| Supply chain | `lightwell_provenance`, `lightwell_check_deps` | SLSA provenance, dependency vulnerabilities |
| Runtime violations | `rhacs_violations`, `rhacs_risk` | Active policy violations, high-risk deployments |
| Compliance standards | `rhacs_compliance_scan`, `rhacs_compliance_status` | CIS, NIST, PCI benchmark results |
| Build hygiene | `container_lint`, `lightwell_config_check` | Containerfile best practices, build config |

## Related

- `log-sanitizer` builtin — redacts secrets from tool outputs (always active)
- `safety-net` plugin — blocks destructive commands during investigation
- `audit-logs` plugin — API audit log analysis for access pattern review

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/pkg/mustgather"
)

func toolIngressControllers(root *mustgather.Root) (string, error) {
	dir := root.ClusterScopedPath("operator.openshift.io", "ingresscontrollers")
	resources, err := mustgather.WalkResources(dir)
	if err != nil || len(resources) == 0 {
		dir = filepath.Join(root.NamespacedPath("openshift-ingress-operator"),
			"operator.openshift.io", "ingresscontrollers")
		resources, err = mustgather.WalkResources(dir)
		if err != nil || len(resources) == 0 {
			return "No IngressController resources found.", nil
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "IngressControllers: %d\n\n", len(resources))

	for _, r := range resources {
		obj := r.Object
		name := r.Name()
		domain := mustgather.GetNested(obj, "status", "domain")
		replicas := mustgather.GetNested(obj, "spec", "replicas")
		if replicas == "" {
			replicas = mustgather.GetNested(obj, "status", "availableReplicas")
		}

		strategy := mustgather.GetNested(obj, "spec", "endpointPublishingStrategy", "type")
		certName := mustgather.GetNested(obj, "spec", "defaultCertificate", "name")
		if certName == "" {
			certName = "(default)"
		}

		fmt.Fprintf(&b, "%-30s domain=%s\n", name, domain)
		fmt.Fprintf(&b, "  Replicas: %s\n", replicas)
		fmt.Fprintf(&b, "  Endpoint publishing: %s\n", strategy)
		fmt.Fprintf(&b, "  Default certificate: %s\n", certName)

		conditions := mustgather.GetNestedSlice(obj, "status", "conditions")
		if len(conditions) > 0 {
			fmt.Fprintf(&b, "  Conditions:\n")
			for _, c := range conditions {
				cm, ok := c.(map[string]any)
				if !ok {
					continue
				}
				ct := mustgather.GetNested(cm, "type")
				cs := mustgather.GetNested(cm, "status")
				fmt.Fprintf(&b, "    %s=%s\n", ct, cs)
			}
		}
		fmt.Fprintln(&b)
	}
	return b.String(), nil
}

func findHAProxyConfigs(root *mustgather.Root) ([]string, error) {
	ingressNS := root.NamespacedPath("openshift-ingress")
	return mustgather.FindResourceFiles(ingressNS, "haproxy.config")
}

func loadHAProxyConfigs(root *mustgather.Root) ([]*HAProxyConfig, []string, error) {
	allPaths, err := findHAProxyConfigs(root)
	if err != nil || len(allPaths) == 0 {
		return nil, nil, fmt.Errorf("no HAProxy configuration found")
	}

	var configs []*HAProxyConfig
	var paths []string
	for _, p := range allPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		configs = append(configs, parseHAProxyConfig(string(data)))
		paths = append(paths, p)
	}
	if len(configs) == 0 {
		return nil, nil, fmt.Errorf("no HAProxy configuration could be read")
	}
	return configs, paths, nil
}

func toolIngressBackends(root *mustgather.Root, filter string) (string, error) {
	configs, paths, err := loadHAProxyConfigs(root)
	if err != nil {
		return "No HAProxy configuration found in must-gather.", nil
	}

	var b strings.Builder
	for i, cfg := range configs {
		rel, _ := filepath.Rel(root.Path, paths[i])
		fmt.Fprintf(&b, "HAProxy config: %s\n", rel)
		fmt.Fprintf(&b, "Backends: %d\n\n", len(cfg.Backends))

		fmt.Fprintf(&b, "%-60s %-6s %-6s %s\n", "NAME", "SRVS", "MODE", "BALANCE")
		fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 90))

		for _, be := range cfg.Backends {
			if filter != "" && !strings.Contains(be.Name, filter) {
				continue
			}
			mode := be.Mode
			if mode == "" {
				mode = cfg.Defaults.Mode
			}
			balance := be.Balance
			if balance == "" {
				balance = "-"
			}
			fmt.Fprintf(&b, "%-60s %-6d %-6s %s\n",
				be.Name, len(be.Servers), mode, balance)
		}
		fmt.Fprintln(&b)
	}
	return b.String(), nil
}

func toolIngressRouteCheck(root *mustgather.Root, namespace string) (string, error) {
	configs, _, err := loadHAProxyConfigs(root)
	if err != nil {
		return "No HAProxy configuration found in must-gather.", nil
	}

	haproxyBackends := make(map[string]*HAProxyBackend)
	for _, cfg := range configs {
		for i := range cfg.Backends {
			be := &cfg.Backends[i]
			haproxyBackends[be.Name] = be
		}
	}

	type routeInfo struct {
		Name      string
		Namespace string
		Host      string
		TLS       string
	}

	var routes []routeInfo
	namespaces := root.Namespaces
	if namespace != "" {
		namespaces = []string{namespace}
	}

	for _, ns := range namespaces {
		routesFile := filepath.Join(root.NamespacedPath(ns), "route.openshift.io", "routes.yaml")
		resources, err := mustgather.ReadResourceList(routesFile)
		if err != nil {
			continue
		}
		for _, r := range resources {
			obj := r.Object
			rns := r.Namespace()
			if rns == "" {
				rns = ns
			}
			routes = append(routes, routeInfo{
				Name:      r.Name(),
				Namespace: rns,
				Host:      mustgather.GetNested(obj, "spec", "host"),
				TLS:       mustgather.GetNested(obj, "spec", "tls", "termination"),
			})
		}
	}

	var b strings.Builder
	fmt.Fprintln(&b, "Route-Backend Cross-Reference")
	fmt.Fprintf(&b, "Routes: %d, HAProxy backends: %d\n\n", len(routes), len(haproxyBackends))

	matchedBackends := make(map[string]bool)
	var missing []routeInfo
	var misconfigs []string

	for _, route := range routes {
		key := route.Namespace + ":" + route.Name
		found := false
		for beName, be := range haproxyBackends {
			if strings.Contains(beName, key) {
				matchedBackends[beName] = true
				found = true

				if route.TLS == "passthrough" && be.Balance == "source" {
					misconfigs = append(misconfigs, fmt.Sprintf(
						"%s/%s: balance=source on passthrough route (consider roundrobin or leastconn)",
						route.Namespace, route.Name))
				}
				if len(be.Servers) == 0 {
					misconfigs = append(misconfigs, fmt.Sprintf(
						"%s/%s: backend %q has 0 servers",
						route.Namespace, route.Name, beName))
				}
			}
		}
		if !found {
			missing = append(missing, route)
		}
	}

	routePrefixes := []string{"be_http:", "be_edge_http:", "be_tcp:", "be_secure:"}
	var stale []string
	for beName := range haproxyBackends {
		if matchedBackends[beName] {
			continue
		}
		for _, prefix := range routePrefixes {
			if strings.HasPrefix(beName, prefix) {
				stale = append(stale, beName)
				break
			}
		}
	}
	sort.Strings(stale)

	if len(stale) > 0 {
		fmt.Fprintf(&b, "Stale backends (in HAProxy, no matching Route): %d\n", len(stale))
		for _, s := range stale {
			fmt.Fprintf(&b, "  %s\n", s)
		}
		fmt.Fprintln(&b)
	}

	if len(missing) > 0 {
		fmt.Fprintf(&b, "Missing backends (Route exists, not in HAProxy): %d\n", len(missing))
		for _, m := range missing {
			fmt.Fprintf(&b, "  %s/%s (host=%s)\n", m.Namespace, m.Name, m.Host)
		}
		fmt.Fprintln(&b)
	}

	if len(misconfigs) > 0 {
		fmt.Fprintf(&b, "Misconfigurations: %d\n", len(misconfigs))
		for _, m := range misconfigs {
			fmt.Fprintf(&b, "  %s\n", m)
		}
		fmt.Fprintln(&b)
	}

	if len(stale) == 0 && len(missing) == 0 && len(misconfigs) == 0 {
		fmt.Fprintln(&b, "No issues found. All Routes have matching HAProxy backends.")
	}

	return b.String(), nil
}

func toolIngressConfig(root *mustgather.Root) (string, error) {
	configs, paths, err := loadHAProxyConfigs(root)
	if err != nil {
		return "No HAProxy configuration found in must-gather.", nil
	}

	var b strings.Builder
	for i, cfg := range configs {
		rel, _ := filepath.Rel(root.Path, paths[i])
		fmt.Fprintf(&b, "HAProxy config: %s\n\n", rel)

		fmt.Fprintln(&b, "Global:")
		if cfg.Global.MaxConn != "" {
			fmt.Fprintf(&b, "  maxconn: %s\n", cfg.Global.MaxConn)
		}
		for k, v := range cfg.Global.Timeouts {
			fmt.Fprintf(&b, "  timeout %s: %s\n", k, v)
		}
		for _, opt := range cfg.Global.Options {
			fmt.Fprintf(&b, "  %s\n", opt)
		}
		fmt.Fprintln(&b)

		fmt.Fprintln(&b, "Defaults:")
		if cfg.Defaults.Mode != "" {
			fmt.Fprintf(&b, "  mode: %s\n", cfg.Defaults.Mode)
		}
		if cfg.Defaults.MaxConn != "" {
			fmt.Fprintf(&b, "  maxconn: %s\n", cfg.Defaults.MaxConn)
		}
		if len(cfg.Defaults.Timeouts) > 0 {
			fmt.Fprintln(&b, "  Timeouts:")
			for k, v := range cfg.Defaults.Timeouts {
				warning := ""
				if isHighTimeout(v) {
					warning = " [WARNING: > 5 minutes]"
				}
				fmt.Fprintf(&b, "    %s: %s%s\n", k, v, warning)
			}
		}
		fmt.Fprintln(&b)

		if len(cfg.Frontends) > 0 {
			fmt.Fprintf(&b, "Frontends: %d\n", len(cfg.Frontends))
			for _, fe := range cfg.Frontends {
				fmt.Fprintf(&b, "  %s", fe.Name)
				if fe.Mode != "" {
					fmt.Fprintf(&b, " (mode=%s)", fe.Mode)
				}
				fmt.Fprintln(&b)
				for _, bind := range fe.Binds {
					ssl := ""
					if strings.Contains(bind, "ssl") {
						ssl = " [SSL]"
					}
					fmt.Fprintf(&b, "    bind %s%s\n", bind, ssl)
				}
			}
			fmt.Fprintln(&b)
		}

		fmt.Fprintf(&b, "Backends: %d total\n", len(cfg.Backends))
		modes := make(map[string]int)
		emptyBackends := 0
		for _, be := range cfg.Backends {
			m := be.Mode
			if m == "" {
				m = cfg.Defaults.Mode
			}
			if m == "" {
				m = "unknown"
			}
			modes[m]++
			if len(be.Servers) == 0 {
				emptyBackends++
			}
		}
		for m, c := range modes {
			fmt.Fprintf(&b, "  mode=%s: %d\n", m, c)
		}
		if emptyBackends > 0 {
			fmt.Fprintf(&b, "  backends with 0 servers: %d\n", emptyBackends)
		}
	}
	return b.String(), nil
}

func isHighTimeout(val string) bool {
	val = strings.TrimSpace(val)
	if val == "" {
		return false
	}
	if d, err := time.ParseDuration(val); err == nil {
		return d > 5*time.Minute
	}
	numStr := strings.TrimSuffix(val, "ms")
	var ms int
	if _, err := fmt.Sscanf(numStr, "%d", &ms); err == nil {
		return time.Duration(ms)*time.Millisecond > 5*time.Minute
	}
	return false
}

func toolIngressHealth(root *mustgather.Root) (string, error) {
	var b strings.Builder
	var issues []string

	fmt.Fprintln(&b, "Ingress Health Summary")
	fmt.Fprintln(&b, "======================")
	fmt.Fprintln(&b)

	// IngressController status
	icOut, err := toolIngressControllers(root)
	if err == nil && !strings.Contains(icOut, "No IngressController") {
		for _, line := range strings.Split(icOut, "\n") {
			if strings.Contains(line, "IngressControllers:") {
				fmt.Fprintf(&b, "%s\n", strings.TrimSpace(line))
			}
			if strings.Contains(line, "Available=False") {
				issues = append(issues, "IngressController not available")
			}
			if strings.Contains(line, "Degraded=True") {
				issues = append(issues, "IngressController degraded")
			}
		}
	} else {
		issues = append(issues, "No IngressController resources found")
	}

	// HAProxy config
	configs, _, loadErr := loadHAProxyConfigs(root)
	if loadErr != nil {
		issues = append(issues, "No HAProxy configuration found")
	} else {
		fmt.Fprintf(&b, "HAProxy configs: %d\n", len(configs))
		totalBackends := 0
		emptyBackends := 0
		for _, cfg := range configs {
			totalBackends += len(cfg.Backends)
			for _, be := range cfg.Backends {
				if len(be.Servers) == 0 {
					emptyBackends++
				}
			}
			for k, v := range cfg.Defaults.Timeouts {
				if isHighTimeout(v) {
					issues = append(issues, fmt.Sprintf("High default timeout: %s=%s", k, v))
				}
			}
		}
		fmt.Fprintf(&b, "Total backends: %d\n", totalBackends)
		if emptyBackends > 0 {
			issues = append(issues, fmt.Sprintf("%d backends with 0 servers", emptyBackends))
		}
	}

	// Router pod logs
	ingressNS := root.NamespacedPath("openshift-ingress")
	podsDir := filepath.Join(ingressNS, "pods")
	if entries, err := os.ReadDir(podsDir); err == nil {
		errorPat := regexp.MustCompile(`(?i)error|failed|panic|fatal`)
		for _, entry := range entries {
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "router-") {
				continue
			}
			logFiles, _ := mustgather.FindLogFiles(filepath.Join(podsDir, entry.Name()))
			for _, lf := range logFiles {
				counts, _ := mustgather.CountLogPatterns(lf, map[string]*regexp.Regexp{"errors": errorPat})
				if counts["errors"] > 10 {
					rel, _ := filepath.Rel(root.Path, lf)
					issues = append(issues, fmt.Sprintf("Router %s: %d errors in %s", entry.Name(), counts["errors"], rel))
				}
			}
		}
	}

	// Route check
	routeOut, err := toolIngressRouteCheck(root, "")
	if err == nil {
		for _, line := range strings.Split(routeOut, "\n") {
			if strings.Contains(line, "Stale backends") ||
				strings.Contains(line, "Missing backends") ||
				strings.Contains(line, "Misconfigurations") {
				issues = append(issues, strings.TrimSpace(line))
			}
		}
	}

	fmt.Fprintln(&b)
	if len(issues) == 0 {
		fmt.Fprintln(&b, "No critical issues detected.")
	} else {
		fmt.Fprintf(&b, "Issues found: %d\n", len(issues))
		for i, issue := range issues {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, issue)
		}
	}

	return b.String(), nil
}

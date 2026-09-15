package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

type state struct {
	mu      sync.RWMutex
	summary *vmSummary
}

func buildTools(oc *redhat.OcClient) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "virt_vms",
			Description: "List VirtualMachines. Shows name, status, readiness, CPU, and memory for each VM. Use namespace='all' for all namespaces.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace to list VMs in, or 'all' for all namespaces (default: current namespace)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
				}
				json.Unmarshal(args, &input)

				var raw string
				var err error
				if input.Namespace == "all" {
					raw, err = oc.Raw(ctx, "get", "virtualmachines.kubevirt.io", "-A", "-o", "json")
				} else {
					getArgs := []string{"get", "virtualmachines.kubevirt.io", "-o", "json"}
					if input.Namespace != "" {
						getArgs = append(getArgs, "--namespace", input.Namespace)
					}
					raw, err = oc.Raw(ctx, getArgs...)
				}
				if err != nil {
					return fmt.Sprintf("Error listing VMs: %v", err), nil
				}
				vms, err := parseVMList(json.RawMessage(raw))
				if err != nil {
					return fmt.Sprintf("Error parsing VMs: %v", err), nil
				}
				if len(vms) == 0 {
					ns := input.Namespace
					if ns == "" {
						ns = "current"
					}
					if ns == "all" {
						return "No VirtualMachines found across all namespaces.", nil
					}
					return fmt.Sprintf("No VirtualMachines found in namespace %s.", ns), nil
				}
				out, _ := json.MarshalIndent(vms, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "virt_describe",
			Description: "Get detailed information about a VirtualMachine including CPU, memory, disks, volumes, networks, guest OS info, and conditions.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":      map[string]any{"type": "string", "description": "VirtualMachine name"},
					"namespace": map[string]any{"type": "string", "description": "Namespace (default: current namespace)"},
				},
				"required": []string{"name"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				resource := "virtualmachines.kubevirt.io/" + input.Name
				var opts *redhat.OcGetOptions
				if input.Namespace != "" {
					opts = &redhat.OcGetOptions{Namespace: input.Namespace}
				}
				result, err := oc.Get(ctx, resource, opts)
				if err != nil {
					return fmt.Sprintf("Error getting VM %q: %v", input.Name, err), nil
				}
				detail, err := parseVMDetail(result)
				if err != nil {
					return fmt.Sprintf("Error parsing VM: %v", err), nil
				}
				out, _ := json.MarshalIndent(detail, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "virt_start",
			Description: "Start a stopped VirtualMachine by patching spec.running to true.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":      map[string]any{"type": "string", "description": "VirtualMachine name"},
					"namespace": map[string]any{"type": "string", "description": "Namespace (default: current namespace)"},
				},
				"required": []string{"name"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				patchArgs := []string{
					"patch", "virtualmachine.kubevirt.io/" + input.Name,
					"--type", "merge", "-p", `{"spec":{"running":true}}`,
				}
				if input.Namespace != "" {
					patchArgs = append(patchArgs, "--namespace", input.Namespace)
				}
				out, err := oc.Raw(ctx, patchArgs...)
				if err != nil {
					return fmt.Sprintf("Error starting VM %q: %v", input.Name, err), nil
				}
				return strings.TrimSpace(out), nil
			},
		},
		{
			Name:        "virt_stop",
			Description: "Stop a running VirtualMachine by patching spec.running to false.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":      map[string]any{"type": "string", "description": "VirtualMachine name"},
					"namespace": map[string]any{"type": "string", "description": "Namespace (default: current namespace)"},
				},
				"required": []string{"name"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				patchArgs := []string{
					"patch", "virtualmachine.kubevirt.io/" + input.Name,
					"--type", "merge", "-p", `{"spec":{"running":false}}`,
				}
				if input.Namespace != "" {
					patchArgs = append(patchArgs, "--namespace", input.Namespace)
				}
				out, err := oc.Raw(ctx, patchArgs...)
				if err != nil {
					return fmt.Sprintf("Error stopping VM %q: %v", input.Name, err), nil
				}
				return strings.TrimSpace(out), nil
			},
		},
		{
			Name:        "virt_restart",
			Description: "Restart a VirtualMachine by deleting its VirtualMachineInstance. The VM controller will recreate it.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":      map[string]any{"type": "string", "description": "VirtualMachine name"},
					"namespace": map[string]any{"type": "string", "description": "Namespace (default: current namespace)"},
				},
				"required": []string{"name"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				deleteArgs := []string{
					"delete", "virtualmachineinstance.kubevirt.io/" + input.Name,
				}
				if input.Namespace != "" {
					deleteArgs = append(deleteArgs, "--namespace", input.Namespace)
				}
				out, err := oc.Raw(ctx, deleteArgs...)
				if err != nil {
					return fmt.Sprintf("Error restarting VM %q: %v", input.Name, err), nil
				}
				return fmt.Sprintf("Restart initiated. %s", strings.TrimSpace(out)), nil
			},
		},
		{
			Name:        "virt_migrate",
			Description: "Live migrate a running VirtualMachine to another node by creating a VirtualMachineInstanceMigration.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":      map[string]any{"type": "string", "description": "VirtualMachine name to migrate"},
					"namespace": map[string]any{"type": "string", "description": "Namespace (default: current namespace)"},
				},
				"required": []string{"name"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				ns := input.Namespace
				if ns == "" {
					nsOut, err := oc.Raw(ctx, "config", "view", "--minify", "-o", "jsonpath={.contexts[0].context.namespace}")
					if err == nil && strings.TrimSpace(nsOut) != "" {
						ns = strings.TrimSpace(nsOut)
					} else {
						ns = "default"
					}
				}
				migrationName := fmt.Sprintf("%s-migration", input.Name)
				manifest := map[string]any{
					"apiVersion": "kubevirt.io/v1",
					"kind":       "VirtualMachineInstanceMigration",
					"metadata": map[string]string{
						"name":      migrationName,
						"namespace": ns,
					},
					"spec": map[string]string{
						"vmiName": input.Name,
					},
				}
				manifestJSON, _ := json.Marshal(manifest)
				out, err := oc.Apply(ctx, string(manifestJSON))
				if err != nil {
					return fmt.Sprintf("Error creating migration for VM %q: %v", input.Name, err), nil
				}
				return fmt.Sprintf("Migration %q created. %s", migrationName, strings.TrimSpace(out)), nil
			},
		},
		{
			Name:        "virt_console",
			Description: "Get recent serial console output from a VirtualMachineInstance by reading its virt-launcher pod logs.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":      map[string]any{"type": "string", "description": "VirtualMachine name"},
					"namespace": map[string]any{"type": "string", "description": "Namespace (default: current namespace)"},
					"tail":      map[string]any{"type": "integer", "description": "Number of log lines to retrieve (default: 100)"},
				},
				"required": []string{"name"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
					Tail      int    `json:"tail"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				tail := input.Tail
				if tail <= 0 {
					tail = 100
				}
				// Find the virt-launcher pod by label
				podArgs := []string{
					"get", "pods",
					"-l", fmt.Sprintf("kubevirt.io/domain=%s", input.Name),
					"-o", "jsonpath={.items[0].metadata.name}",
				}
				if input.Namespace != "" {
					podArgs = append(podArgs, "--namespace", input.Namespace)
				}
				podName, err := oc.Raw(ctx, podArgs...)
				if err != nil || strings.TrimSpace(podName) == "" {
					return fmt.Sprintf("No virt-launcher pod found for VM %q", input.Name), nil
				}
				logArgs := []string{
					"logs", strings.TrimSpace(podName),
					"--container", "guest-console-log",
					"--tail", fmt.Sprintf("%d", tail),
				}
				if input.Namespace != "" {
					logArgs = append(logArgs, "--namespace", input.Namespace)
				}
				out, err := oc.Raw(ctx, logArgs...)
				if err != nil {
					return fmt.Sprintf("Error getting console output for VM %q: %v", input.Name, err), nil
				}
				return out, nil
			},
		},
		{
			Name:        "virt_datavolumes",
			Description: "List DataVolumes showing import/clone/upload status, progress, and size.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace (default: current namespace). Use 'all' for all namespaces."},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
				}
				json.Unmarshal(args, &input)

				var raw string
				var err error
				if input.Namespace == "all" {
					raw, err = oc.Raw(ctx, "get", "datavolumes.cdi.kubevirt.io", "-A", "-o", "json")
				} else {
					getArgs := []string{"get", "datavolumes.cdi.kubevirt.io", "-o", "json"}
					if input.Namespace != "" {
						getArgs = append(getArgs, "--namespace", input.Namespace)
					}
					raw, err = oc.Raw(ctx, getArgs...)
				}
				if err != nil {
					return fmt.Sprintf("Error listing DataVolumes: %v", err), nil
				}
				dvs, err := parseDVList(raw)
				if err != nil {
					return fmt.Sprintf("Error parsing DataVolumes: %v", err), nil
				}
				if len(dvs) == 0 {
					return "No DataVolumes found.", nil
				}
				out, _ := json.MarshalIndent(dvs, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "virt_templates",
			Description: "List available VirtualMachine templates and instance types for creating new VMs.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace for namespace-scoped templates (default: current namespace)"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
				}
				json.Unmarshal(args, &input)

				type templateInfo struct {
					Name        string `json:"name"`
					Namespace   string `json:"namespace,omitempty"`
					Description string `json:"description,omitempty"`
					Scope       string `json:"scope"`
				}
				var results []templateInfo

				if raw, err := oc.Raw(ctx, "get", "templates", "-n", "openshift", "-l", "template.kubevirt.io/type=base", "-o", "json"); err == nil {
					var tplList struct {
						Items []struct {
							Metadata struct {
								Name        string            `json:"name"`
								Namespace   string            `json:"namespace"`
								Annotations map[string]string `json:"annotations,omitempty"`
							} `json:"metadata"`
						} `json:"items"`
					}
					if json.Unmarshal([]byte(raw), &tplList) == nil {
						for _, t := range tplList.Items {
							info := templateInfo{
								Name:      t.Metadata.Name,
								Namespace: t.Metadata.Namespace,
								Scope:     "common",
							}
							if desc, ok := t.Metadata.Annotations["description"]; ok {
								info.Description = desc
							}
							results = append(results, info)
						}
					}
				}

				if raw, err := oc.Raw(ctx, "get", "virtualmachineclusterinstancetypes", "-o", "json"); err == nil {
					var itList struct {
						Items []struct {
							Metadata struct {
								Name        string            `json:"name"`
								Annotations map[string]string `json:"annotations,omitempty"`
							} `json:"metadata"`
							Spec struct {
								CPU struct {
									Guest int `json:"guest"`
								} `json:"cpu"`
								Memory struct {
									Guest string `json:"guest"`
								} `json:"memory"`
							} `json:"spec"`
						} `json:"items"`
					}
					if json.Unmarshal([]byte(raw), &itList) == nil {
						for _, it := range itList.Items {
							desc := fmt.Sprintf("%d vCPU, %s memory", it.Spec.CPU.Guest, it.Spec.Memory.Guest)
							if d, ok := it.Metadata.Annotations["description"]; ok {
								desc = d
							}
							results = append(results, templateInfo{
								Name:        it.Metadata.Name,
								Description: desc,
								Scope:       "instancetype",
							})
						}
					}
				}

				if len(results) == 0 {
					return "No VM templates or instance types found.", nil
				}
				out, _ := json.MarshalIndent(results, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "virt_network",
			Description: "List NetworkAttachmentDefinitions available for VMs (Multus secondary networks).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace (default: current namespace). Use 'all' for all namespaces."},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
				}
				json.Unmarshal(args, &input)

				var raw string
				var err error
				if input.Namespace == "all" {
					raw, err = oc.Raw(ctx, "get", "net-attach-def", "-A", "-o", "json")
				} else {
					getArgs := []string{"get", "net-attach-def", "-o", "json"}
					if input.Namespace != "" {
						getArgs = append(getArgs, "--namespace", input.Namespace)
					}
					raw, err = oc.Raw(ctx, getArgs...)
				}
				if err != nil {
					return fmt.Sprintf("Error listing NetworkAttachmentDefinitions: %v", err), nil
				}

				var nadList struct {
					Items []struct {
						Metadata struct {
							Name      string `json:"name"`
							Namespace string `json:"namespace"`
						} `json:"metadata"`
						Spec struct {
							Config string `json:"config"`
						} `json:"spec"`
					} `json:"items"`
				}
				if err := json.Unmarshal([]byte(raw), &nadList); err != nil {
					return fmt.Sprintf("Error parsing NetworkAttachmentDefinitions: %v", err), nil
				}
				if len(nadList.Items) == 0 {
					return "No NetworkAttachmentDefinitions found.", nil
				}

				type nadInfo struct {
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
					Type      string `json:"type,omitempty"`
				}
				var nads []nadInfo
				for _, item := range nadList.Items {
					info := nadInfo{
						Name:      item.Metadata.Name,
						Namespace: item.Metadata.Namespace,
					}
					var config struct {
						Type string `json:"type"`
					}
					if json.Unmarshal([]byte(item.Spec.Config), &config) == nil {
						info.Type = config.Type
					}
					nads = append(nads, info)
				}
				out, _ := json.MarshalIndent(nads, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "virt_migrations",
			Description: "List VirtualMachineInstanceMigrations showing status of live migrations.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace (default: current namespace). Use 'all' for all namespaces."},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
				}
				json.Unmarshal(args, &input)

				var raw string
				var err error
				if input.Namespace == "all" {
					raw, err = oc.Raw(ctx, "get", "virtualmachineinstancemigrations.kubevirt.io", "-A", "-o", "json")
				} else {
					getArgs := []string{"get", "virtualmachineinstancemigrations.kubevirt.io", "-o", "json"}
					if input.Namespace != "" {
						getArgs = append(getArgs, "--namespace", input.Namespace)
					}
					raw, err = oc.Raw(ctx, getArgs...)
				}
				if err != nil {
					return fmt.Sprintf("Error listing migrations: %v", err), nil
				}

				var migList struct {
					Items []struct {
						Metadata struct {
							Name              string `json:"name"`
							Namespace         string `json:"namespace"`
							CreationTimestamp string `json:"creationTimestamp"`
						} `json:"metadata"`
						Spec struct {
							VMIName string `json:"vmiName"`
						} `json:"spec"`
						Status *struct {
							Phase      string `json:"phase"`
							SourceNode string `json:"sourceNode"`
							TargetNode string `json:"targetNode"`
						} `json:"status,omitempty"`
					} `json:"items"`
				}
				if err := json.Unmarshal([]byte(raw), &migList); err != nil {
					return fmt.Sprintf("Error parsing migrations: %v", err), nil
				}
				if len(migList.Items) == 0 {
					return "No VirtualMachineInstanceMigrations found.", nil
				}

				type migInfo struct {
					Name       string `json:"name"`
					Namespace  string `json:"namespace"`
					VMI        string `json:"vmi"`
					Phase      string `json:"phase"`
					SourceNode string `json:"sourceNode,omitempty"`
					TargetNode string `json:"targetNode,omitempty"`
					Created    string `json:"created"`
				}
				var migs []migInfo
				for _, item := range migList.Items {
					m := migInfo{
						Name:      item.Metadata.Name,
						Namespace: item.Metadata.Namespace,
						VMI:       item.Spec.VMIName,
						Created:   item.Metadata.CreationTimestamp,
					}
					if item.Status != nil {
						m.Phase = item.Status.Phase
						m.SourceNode = item.Status.SourceNode
						m.TargetNode = item.Status.TargetNode
					} else {
						m.Phase = "Pending"
					}
					migs = append(migs, m)
				}
				out, _ := json.MarshalIndent(migs, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "virt_node_capacity",
			Description: "Show node roles, CPU, and memory capacity vs allocatable for VM scheduling and sizing.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
				data, err := oc.Get(ctx, "nodes", nil)
				if err != nil {
					return fmt.Sprintf("Error listing nodes: %v", err), nil
				}
				nodes, err := parseNodeCapacity(data)
				if err != nil {
					return fmt.Sprintf("Error parsing nodes: %v", err), nil
				}
				if len(nodes) == 0 {
					return "No nodes found.", nil
				}
				out, _ := json.MarshalIndent(nodes, "", "  ")
				return string(out), nil
			},
		},
	}
}

func gatherVMSummary(ctx context.Context, oc *redhat.OcClient) *vmSummary {
	if !oc.IsAvailable(ctx) || !oc.IsLoggedIn(ctx) {
		return nil
	}
	raw, err := oc.Raw(ctx, "get", "virtualmachines.kubevirt.io", "-A", "-o", "json")
	if err != nil {
		return nil
	}
	vms, err := parseVMList(json.RawMessage(raw))
	if err != nil {
		return nil
	}
	summary := &vmSummary{Total: len(vms)}
	for _, vm := range vms {
		switch classifyVMStatus(vm.Status) {
		case "running":
			summary.Running++
		case "stopped":
			summary.Stopped++
		case "error":
			summary.Error++
		default:
			summary.Other++
		}
	}
	return summary
}

func buildHealthTool(oc *redhat.OcClient) plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "virt_health",
		Description: "Check OpenShift Virtualization service health: oc connectivity, KubeVirt CRD, HyperConverged operator status.",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Execute: func(ctx context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
			var lines []string
			lines = append(lines, "Service Health:", "")

			if !oc.IsAvailable(ctx) {
				lines = append(lines, "[DOWN] oc CLI — not found in PATH")
				return strings.Join(lines, "\n"), nil
			}
			if !oc.IsLoggedIn(ctx) {
				lines = append(lines, "[DOWN] OpenShift cluster — not logged in")
				return strings.Join(lines, "\n"), nil
			}
			lines = append(lines, "[OK] OpenShift cluster")

			_, err := oc.Raw(ctx, "get", "crd", "virtualmachines.kubevirt.io", "-o", "name")
			if err != nil {
				lines = append(lines, "[DOWN] KubeVirt — CRD not found (OpenShift Virtualization may not be installed)")
			} else {
				lines = append(lines, "[OK] KubeVirt CRD")
			}

			hcRaw, err := oc.Raw(ctx, "get", "hyperconvergeds.hco.kubevirt.io", "-A", "-o", "json")
			if err != nil {
				lines = append(lines, "[SKIP] HyperConverged — unable to query")
			} else {
				var hcList struct {
					Items []struct {
						Status *struct {
							Conditions []struct {
								Type   string `json:"type"`
								Status string `json:"status"`
							} `json:"conditions"`
						} `json:"status,omitempty"`
					} `json:"items"`
				}
				if json.Unmarshal([]byte(hcRaw), &hcList) == nil && len(hcList.Items) > 0 {
					available := false
					if hcList.Items[0].Status != nil {
						for _, c := range hcList.Items[0].Status.Conditions {
							if c.Type == "Available" && c.Status == "True" {
								available = true
							}
						}
					}
					if available {
						lines = append(lines, "[OK] HyperConverged operator")
					} else {
						lines = append(lines, "[DOWN] HyperConverged operator — not Available")
					}
				} else {
					lines = append(lines, "[DOWN] HyperConverged — not found")
				}
			}

			return strings.Join(lines, "\n"), nil
		},
	}
}

func newPlugin() plugin.Plugin {
	oc := redhat.NewOcClient()
	s := &state{}

	tools := buildTools(oc)
	tools = append(tools, buildHealthTool(oc))

	return plugin.Plugin{
		ID:    "ocp-virt",
		Tools: tools,
		Hooks: plugin.HookHandlers{
			SessionStart: func(ctx context.Context, event plugin.SessionStartEvent) error {
				slog.Info("ocp-virt: gathering VM summary", "sessionId", event.SessionID)
				summary := gatherVMSummary(ctx, oc)
				s.mu.Lock()
				s.summary = summary
				s.mu.Unlock()
				return nil
			},
		},
	}
}

func main() {
	plugin.Run(newPlugin())
}

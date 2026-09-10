package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

func getRunStatus(conditions []pipelineRunCondition) string {
	if len(conditions) == 0 {
		return "Unknown"
	}
	c := conditions[0]
	if c.Status == "True" {
		if c.Reason != "" {
			return c.Reason
		}
		return "Succeeded"
	}
	if c.Status == "False" {
		if c.Reason != "" {
			return c.Reason
		}
		return "Failed"
	}
	if c.Reason != "" {
		return c.Reason
	}
	return "Running"
}

func formatDuration(start, end string) string {
	startTime, err := time.Parse(time.RFC3339, start)
	if err != nil {
		return "unknown"
	}
	endTime, err := time.Parse(time.RFC3339, end)
	if err != nil {
		return "unknown"
	}
	totalSeconds := int(math.Round(endTime.Sub(startTime).Seconds()))
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", totalSeconds)
}

type pipelineRunCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type childReference struct {
	Name             string `json:"name"`
	PipelineTaskName string `json:"pipelineTaskName"`
	Kind             string `json:"kind"`
}

func buildTools(oc *redhat.OcClient) []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "tekton_list_pipelines",
			Description: "List Tekton pipelines in a namespace. Returns name, creation date, and tasks in each pipeline.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Kubernetes namespace"},
				},
				"required": []string{"namespace"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				result, err := oc.Get(ctx, "pipelines", &redhat.OcGetOptions{Namespace: input.Namespace})
				if err != nil {
					return fmt.Sprintf("Error listing pipelines: %v", err), nil
				}
				var pipelineList struct {
					Items []struct {
						Metadata struct {
							Name              string `json:"name"`
							Namespace         string `json:"namespace"`
							CreationTimestamp string `json:"creationTimestamp"`
						} `json:"metadata"`
						Spec struct {
							Tasks []struct {
								Name string `json:"name"`
							} `json:"tasks"`
						} `json:"spec"`
					} `json:"items"`
				}
				if err := json.Unmarshal(result, &pipelineList); err != nil {
					return fmt.Sprintf("Error parsing pipelines: %v", err), nil
				}
				if len(pipelineList.Items) == 0 {
					return "No pipelines found in namespace " + input.Namespace, nil
				}
				type pipelineInfo struct {
					Name    string   `json:"name"`
					Created string   `json:"created"`
					Tasks   []string `json:"tasks"`
				}
				var pipelines []pipelineInfo
				for _, p := range pipelineList.Items {
					var tasks []string
					for _, t := range p.Spec.Tasks {
						tasks = append(tasks, t.Name)
					}
					if tasks == nil {
						tasks = []string{}
					}
					pipelines = append(pipelines, pipelineInfo{
						Name:    p.Metadata.Name,
						Created: p.Metadata.CreationTimestamp,
						Tasks:   tasks,
					})
				}
				out, _ := json.MarshalIndent(pipelines, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "tekton_list_runs",
			Description: "List PipelineRuns in a namespace with status, start time, and duration. Optionally filter by pipeline name.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Kubernetes namespace"},
					"pipeline":  map[string]any{"type": "string", "description": "Filter by pipeline name"},
				},
				"required": []string{"namespace"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
					Pipeline  string `json:"pipeline"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				getOpts := &redhat.OcGetOptions{Namespace: input.Namespace}
				if input.Pipeline != "" {
					getOpts.Selector = "tekton.dev/pipeline=" + input.Pipeline
				}
				result, err := oc.Get(ctx, "pipelineruns", getOpts)
				if err != nil {
					return fmt.Sprintf("Error listing PipelineRuns: %v", err), nil
				}
				var prList struct {
					Items []struct {
						Metadata struct {
							Name   string            `json:"name"`
							Labels map[string]string `json:"labels,omitempty"`
						} `json:"metadata"`
						Spec struct {
							PipelineRef *struct {
								Name string `json:"name"`
							} `json:"pipelineRef,omitempty"`
						} `json:"spec"`
						Status *struct {
							Conditions     []pipelineRunCondition `json:"conditions,omitempty"`
							StartTime      string                 `json:"startTime,omitempty"`
							CompletionTime string                 `json:"completionTime,omitempty"`
						} `json:"status,omitempty"`
					} `json:"items"`
				}
				if err := json.Unmarshal(result, &prList); err != nil {
					return fmt.Sprintf("Error parsing PipelineRuns: %v", err), nil
				}
				if len(prList.Items) == 0 {
					return "No PipelineRuns found", nil
				}
				type runInfo struct {
					Name      string `json:"name"`
					Pipeline  string `json:"pipeline"`
					Status    string `json:"status"`
					StartTime string `json:"startTime"`
					Duration  string `json:"duration"`
				}
				var runs []runInfo
				for _, r := range prList.Items {
					pipeline := "unknown"
					if r.Spec.PipelineRef != nil && r.Spec.PipelineRef.Name != "" {
						pipeline = r.Spec.PipelineRef.Name
					}
					var conditions []pipelineRunCondition
					startTime := "not started"
					duration := "in progress"
					if r.Status != nil {
						conditions = r.Status.Conditions
						if r.Status.StartTime != "" {
							startTime = r.Status.StartTime
						}
						if r.Status.StartTime != "" && r.Status.CompletionTime != "" {
							duration = formatDuration(r.Status.StartTime, r.Status.CompletionTime)
						}
					}
					runs = append(runs, runInfo{
						Name:      r.Metadata.Name,
						Pipeline:  pipeline,
						Status:    getRunStatus(conditions),
						StartTime: startTime,
						Duration:  duration,
					})
				}
				out, _ := json.MarshalIndent(runs, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "tekton_run_status",
			Description: "Get detailed status of a specific PipelineRun including each task's status, conditions, and timing.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Kubernetes namespace"},
					"name":      map[string]any{"type": "string", "description": "PipelineRun name"},
				},
				"required": []string{"namespace", "name"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
					Name      string `json:"name"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				result, err := oc.Get(ctx, "pipelineruns/"+input.Name, &redhat.OcGetOptions{Namespace: input.Namespace})
				if err != nil {
					return fmt.Sprintf("Error getting PipelineRun status: %v", err), nil
				}
				var pr struct {
					Metadata struct {
						Name string `json:"name"`
					} `json:"metadata"`
					Spec struct {
						PipelineRef *struct {
							Name string `json:"name"`
						} `json:"pipelineRef,omitempty"`
					} `json:"spec"`
					Status *struct {
						Conditions      []pipelineRunCondition `json:"conditions,omitempty"`
						StartTime       string                 `json:"startTime,omitempty"`
						CompletionTime  string                 `json:"completionTime,omitempty"`
						ChildReferences []childReference       `json:"childReferences,omitempty"`
					} `json:"status,omitempty"`
				}
				if err := json.Unmarshal(result, &pr); err != nil {
					return fmt.Sprintf("Error parsing PipelineRun: %v", err), nil
				}

				pipeline := "unknown"
				if pr.Spec.PipelineRef != nil && pr.Spec.PipelineRef.Name != "" {
					pipeline = pr.Spec.PipelineRef.Name
				}

				var conditions []pipelineRunCondition
				var conditionOut any
				var startTime, completionTime, duration any
				var tasks []map[string]string

				if pr.Status != nil {
					conditions = pr.Status.Conditions
					if len(pr.Status.Conditions) > 0 {
						c := pr.Status.Conditions[0]
						conditionOut = map[string]string{
							"type":    c.Type,
							"status":  c.Status,
							"reason":  c.Reason,
							"message": c.Message,
						}
					}
					if pr.Status.StartTime != "" {
						startTime = pr.Status.StartTime
					}
					if pr.Status.CompletionTime != "" {
						completionTime = pr.Status.CompletionTime
					}
					if pr.Status.StartTime != "" && pr.Status.CompletionTime != "" {
						duration = formatDuration(pr.Status.StartTime, pr.Status.CompletionTime)
					}
					for _, ref := range pr.Status.ChildReferences {
						tasks = append(tasks, map[string]string{
							"name":        ref.PipelineTaskName,
							"taskRunName": ref.Name,
							"kind":        ref.Kind,
						})
					}
				}

				status := map[string]any{
					"name":           pr.Metadata.Name,
					"pipeline":       pipeline,
					"status":         getRunStatus(conditions),
					"condition":      conditionOut,
					"startTime":      startTime,
					"completionTime": completionTime,
					"duration":       duration,
					"tasks":          tasks,
				}
				out, _ := json.MarshalIndent(status, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "tekton_run_logs",
			Description: "Get logs for a task in a PipelineRun. Retrieves the pod logs for the specified task's TaskRun.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Kubernetes namespace"},
					"run":       map[string]any{"type": "string", "description": "PipelineRun name"},
					"task":      map[string]any{"type": "string", "description": "Task name from the pipeline"},
					"container": map[string]any{"type": "string", "description": "Specific step container name"},
					"tail":      map[string]any{"type": "integer", "description": "Number of log lines from the end"},
				},
				"required": []string{"namespace", "run", "task"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
					Run       string `json:"run"`
					Task      string `json:"task"`
					Container string `json:"container"`
					Tail      int    `json:"tail"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				// Get the PipelineRun to find the task's TaskRun
				result, err := oc.Get(ctx, "pipelineruns/"+input.Run, &redhat.OcGetOptions{Namespace: input.Namespace})
				if err != nil {
					return fmt.Sprintf("Error getting PipelineRun: %v", err), nil
				}
				var pr struct {
					Status *struct {
						ChildReferences []childReference `json:"childReferences,omitempty"`
					} `json:"status,omitempty"`
				}
				if err := json.Unmarshal(result, &pr); err != nil {
					return fmt.Sprintf("Error parsing PipelineRun: %v", err), nil
				}
				var taskRunName string
				if pr.Status != nil {
					for _, ref := range pr.Status.ChildReferences {
						if ref.PipelineTaskName == input.Task {
							taskRunName = ref.Name
							break
						}
					}
				}
				if taskRunName == "" {
					return fmt.Sprintf("Task %q not found in PipelineRun %q", input.Task, input.Run), nil
				}
				podName := taskRunName + "-pod"
				logArgs := []string{"logs", "pod/" + podName, "--namespace", input.Namespace}
				if input.Container != "" {
					logArgs = append(logArgs, "--container", input.Container)
				} else {
					logArgs = append(logArgs, "--all-containers=true")
				}
				if input.Tail > 0 {
					logArgs = append(logArgs, "--tail", fmt.Sprintf("%d", input.Tail))
				}
				out, err := oc.Raw(ctx, logArgs...)
				if err != nil {
					return fmt.Sprintf("Error getting logs: %v", err), nil
				}
				return out, nil
			},
		},
		{
			Name:        "tekton_list_tasks",
			Description: "List available Tasks (namespace-scoped) and ClusterTasks.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Kubernetes namespace for namespace-scoped Tasks"},
				},
				"required": []string{"namespace"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string `json:"namespace"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				type taskInfo struct {
					Name  string   `json:"name"`
					Scope string   `json:"scope"`
					Steps []string `json:"steps"`
				}
				var results []taskInfo

				// Namespace-scoped tasks
				if taskResult, err := oc.Get(ctx, "tasks", &redhat.OcGetOptions{Namespace: input.Namespace}); err == nil {
					var taskList struct {
						Items []struct {
							Metadata struct {
								Name string `json:"name"`
							} `json:"metadata"`
							Spec struct {
								Steps []struct {
									Name string `json:"name"`
								} `json:"steps"`
							} `json:"spec"`
						} `json:"items"`
					}
					if err := json.Unmarshal(taskResult, &taskList); err == nil {
						for _, t := range taskList.Items {
							var steps []string
							for _, s := range t.Spec.Steps {
								steps = append(steps, s.Name)
							}
							if steps == nil {
								steps = []string{}
							}
							results = append(results, taskInfo{Name: t.Metadata.Name, Scope: "namespace", Steps: steps})
						}
					}
				}

				// ClusterTasks
				if ctResult, err := oc.Get(ctx, "clustertasks", nil); err == nil {
					var ctList struct {
						Items []struct {
							Metadata struct {
								Name string `json:"name"`
							} `json:"metadata"`
							Spec struct {
								Steps []struct {
									Name string `json:"name"`
								} `json:"steps"`
							} `json:"spec"`
						} `json:"items"`
					}
					if err := json.Unmarshal(ctResult, &ctList); err == nil {
						for _, t := range ctList.Items {
							var steps []string
							for _, s := range t.Spec.Steps {
								steps = append(steps, s.Name)
							}
							if steps == nil {
								steps = []string{}
							}
							results = append(results, taskInfo{Name: t.Metadata.Name, Scope: "cluster", Steps: steps})
						}
					}
				}

				if len(results) == 0 {
					return "No Tasks or ClusterTasks found", nil
				}
				out, _ := json.MarshalIndent(results, "", "  ")
				return string(out), nil
			},
		},
		{
			Name:        "tekton_start_run",
			Description: "Start a pipeline run by creating a PipelineRun resource.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Kubernetes namespace"},
					"pipeline":  map[string]any{"type": "string", "description": "Pipeline name to run"},
					"params":    map[string]any{"type": "object", "description": "Pipeline parameters as key-value pairs", "additionalProperties": map[string]any{"type": "string"}},
				},
				"required": []string{"namespace", "pipeline"},
			},
			Execute: func(ctx context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Namespace string            `json:"namespace"`
					Pipeline  string            `json:"pipeline"`
					Params    map[string]string `json:"params"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}

				runName := fmt.Sprintf("%s-run-%d", input.Pipeline, time.Now().UnixMilli())

				spec := map[string]any{
					"pipelineRef": map[string]string{"name": input.Pipeline},
				}
				if len(input.Params) > 0 {
					var params []map[string]string
					for k, v := range input.Params {
						params = append(params, map[string]string{"name": k, "value": v})
					}
					spec["params"] = params
				}

				manifest := map[string]any{
					"apiVersion": "tekton.dev/v1",
					"kind":       "PipelineRun",
					"metadata": map[string]string{
						"name":      runName,
						"namespace": input.Namespace,
					},
					"spec": spec,
				}
				manifestJSON, _ := json.Marshal(manifest)

				applyResult, err := oc.Apply(ctx, string(manifestJSON))
				if err != nil {
					return fmt.Sprintf("Error starting pipeline run: %v", err), nil
				}

				resultObj := map[string]string{
					"created":   runName,
					"namespace": input.Namespace,
					"pipeline":  input.Pipeline,
					"result":    strings.TrimSpace(applyResult),
				}
				out, _ := json.MarshalIndent(resultObj, "", "  ")
				return string(out), nil
			},
		},
	}
}

func newPlugin() plugin.Plugin {
	oc := redhat.NewOcClient()
	return plugin.Plugin{
		ID:    "tekton",
		Tools: buildTools(oc),
	}
}

func main() {
	plugin.Run(newPlugin())
}

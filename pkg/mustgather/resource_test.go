package mustgather

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadResource_YAML(t *testing.T) {
	dir := t.TempDir()
	yamlData := `apiVersion: v1
kind: Node
metadata:
  name: master-0
  labels:
    node-role.kubernetes.io/master: ""
status:
  conditions:
    - type: Ready
      status: "True"
  capacity:
    cpu: "16"
`
	path := filepath.Join(dir, "node.yaml")
	os.WriteFile(path, []byte(yamlData), 0o644)

	r, err := ReadResource(path)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name() != "master-0" {
		t.Errorf("name = %q", r.Name())
	}
	if r.Kind() != "Node" {
		t.Errorf("kind = %q", r.Kind())
	}
}

func TestReadResource_JSON(t *testing.T) {
	dir := t.TempDir()
	jsonData := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"test","namespace":"default"}}`
	path := filepath.Join(dir, "pod.json")
	os.WriteFile(path, []byte(jsonData), 0o644)

	r, err := ReadResource(path)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name() != "test" {
		t.Errorf("name = %q", r.Name())
	}
	if r.Namespace() != "default" {
		t.Errorf("namespace = %q", r.Namespace())
	}
}

func TestReadResourceList(t *testing.T) {
	dir := t.TempDir()
	yamlData := `apiVersion: v1
kind: PodList
items:
  - metadata:
      name: pod1
      namespace: default
  - metadata:
      name: pod2
      namespace: kube-system
`
	path := filepath.Join(dir, "pods.yaml")
	os.WriteFile(path, []byte(yamlData), 0o644)

	resources, err := ReadResourceList(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 2 {
		t.Fatalf("got %d, want 2", len(resources))
	}
	if resources[0].Name() != "pod1" {
		t.Errorf("resources[0].Name = %q", resources[0].Name())
	}
	if resources[1].Namespace() != "kube-system" {
		t.Errorf("resources[1].Namespace = %q", resources[1].Namespace())
	}
}

func TestReadResourceList_SingleResource(t *testing.T) {
	dir := t.TempDir()
	yamlData := `apiVersion: v1
kind: Node
metadata:
  name: single-node
`
	path := filepath.Join(dir, "node.yaml")
	os.WriteFile(path, []byte(yamlData), 0o644)

	resources, err := ReadResourceList(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 {
		t.Fatalf("got %d, want 1", len(resources))
	}
}

func TestWalkResources(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(`{"metadata":{"name":"a"}}`), 0o644)
	os.WriteFile(filepath.Join(dir, "sub", "b.json"), []byte(`{"metadata":{"name":"b"}}`), 0o644)
	os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("not yaml"), 0o644)

	resources, err := WalkResources(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 2 {
		t.Fatalf("got %d resources, want 2", len(resources))
	}
}

func TestGetNested(t *testing.T) {
	obj := map[string]any{
		"metadata": map[string]any{
			"name":      "test",
			"namespace": "default",
		},
		"status": map[string]any{
			"phase": "Running",
			"count": float64(42),
		},
	}

	if got := GetNested(obj, "metadata", "name"); got != "test" {
		t.Errorf("name = %q", got)
	}
	if got := GetNested(obj, "status", "phase"); got != "Running" {
		t.Errorf("phase = %q", got)
	}
	if got := GetNested(obj, "status", "count"); got != "42" {
		t.Errorf("count = %q", got)
	}
	if got := GetNested(obj, "nonexistent"); got != "" {
		t.Errorf("nonexistent = %q", got)
	}
	if got := GetNested(obj, "metadata", "missing"); got != "" {
		t.Errorf("missing = %q", got)
	}
}

func TestGetNestedInt(t *testing.T) {
	obj := map[string]any{
		"spec": map[string]any{
			"replicas": float64(3),
		},
	}
	if got := GetNestedInt(obj, "spec", "replicas"); got != 3 {
		t.Errorf("replicas = %d", got)
	}
	if got := GetNestedInt(obj, "spec", "missing"); got != 0 {
		t.Errorf("missing = %d", got)
	}
}

func TestGetNestedSlice(t *testing.T) {
	obj := map[string]any{
		"items": []any{"a", "b", "c"},
	}
	s := GetNestedSlice(obj, "items")
	if len(s) != 3 {
		t.Errorf("len = %d, want 3", len(s))
	}
}

func TestGetNestedMap(t *testing.T) {
	obj := map[string]any{
		"data": map[string]any{
			"key": "value",
		},
	}
	m := GetNestedMap(obj, "data")
	if m == nil {
		t.Fatal("nil map")
	}
	if m["key"] != "value" {
		t.Errorf("key = %v", m["key"])
	}
}

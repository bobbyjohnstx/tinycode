package main

import (
	"encoding/json"
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin()
	if p.ID != "ocp-virt" {
		t.Errorf("got %q, want %q", p.ID, "ocp-virt")
	}
}

func TestToolDefinitions(t *testing.T) {
	p := newPlugin()
	wantNames := []string{
		"virt_vms", "virt_describe", "virt_start", "virt_stop",
		"virt_restart", "virt_migrate", "virt_console", "virt_datavolumes",
		"virt_templates", "virt_network", "virt_migrations", "virt_node_capacity", "virt_health",
	}
	if len(p.Tools) != len(wantNames) {
		t.Fatalf("got %d tools, want %d", len(p.Tools), len(wantNames))
	}
	for i, want := range wantNames {
		if p.Tools[i].Name != want {
			t.Errorf("tool[%d] name = %q, want %q", i, p.Tools[i].Name, want)
		}
		if p.Tools[i].Execute == nil {
			t.Errorf("tool[%d] %q Execute is nil", i, want)
		}
	}
}

func TestToolSchemas(t *testing.T) {
	p := newPlugin()
	for _, tool := range p.Tools {
		params := tool.Parameters
		if params["type"] != "object" {
			t.Errorf("%s: params type = %v, want %q", tool.Name, params["type"], "object")
		}
		if _, ok := params["properties"].(map[string]any); !ok {
			t.Errorf("%s: properties is not map[string]any", tool.Name)
		}
	}
}

func TestClassifyVMStatus(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Running", "running"},
		{"running", "running"},
		{"Stopped", "stopped"},
		{"PowerOff", "stopped"},
		{"ShutOff", "stopped"},
		{"ErrorUnschedulable", "error"},
		{"CrashLoopBackOff", "error"},
		{"FailedCreate", "error"},
		{"Provisioning", "other"},
		{"Migrating", "other"},
		{"", "other"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := classifyVMStatus(tt.input)
			if got != tt.want {
				t.Errorf("classifyVMStatus(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseVMList(t *testing.T) {
	t.Run("parses VMs with status", func(t *testing.T) {
		raw := `{"items":[
			{"metadata":{"name":"vm1","namespace":"default"},
			 "spec":{"template":{"spec":{"domain":{"cpu":{"cores":2},"resources":{"requests":{"memory":"4Gi"}}}}}},
			 "status":{"printableStatus":"Running","ready":true}},
			{"metadata":{"name":"vm2","namespace":"test"},
			 "spec":{"template":{"spec":{"domain":{"cpu":{"cores":4},"resources":{"requests":{"memory":"8Gi"}}}}}},
			 "status":{"printableStatus":"Stopped","ready":false}}
		]}`
		vms, err := parseVMList(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(vms) != 2 {
			t.Fatalf("got %d VMs, want 2", len(vms))
		}
		if vms[0].Name != "vm1" || vms[0].Namespace != "default" {
			t.Errorf("vm[0] = %+v", vms[0])
		}
		if vms[0].Status != "Running" || vms[0].Ready != "true" {
			t.Errorf("vm[0] status=%q ready=%q", vms[0].Status, vms[0].Ready)
		}
		if vms[0].CPUs != 2 || vms[0].Memory != "4Gi" {
			t.Errorf("vm[0] cpus=%d memory=%q", vms[0].CPUs, vms[0].Memory)
		}
		if vms[1].Status != "Stopped" || vms[1].Ready != "false" {
			t.Errorf("vm[1] status=%q ready=%q", vms[1].Status, vms[1].Ready)
		}
	})

	t.Run("handles nil status", func(t *testing.T) {
		raw := `{"items":[
			{"metadata":{"name":"vm1","namespace":"default"},
			 "spec":{"template":{"spec":{"domain":{"cpu":{"cores":1},"resources":{"requests":{}}}}}}}
		]}`
		vms, err := parseVMList(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if vms[0].Status != "Unknown" || vms[0].Ready != "false" {
			t.Errorf("nil-status VM: status=%q ready=%q", vms[0].Status, vms[0].Ready)
		}
	})

	t.Run("empty items", func(t *testing.T) {
		raw := `{"items":[]}`
		vms, err := parseVMList(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(vms) != 0 {
			t.Errorf("got %d VMs, want 0", len(vms))
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := parseVMList(json.RawMessage(`{bad}`))
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestParseVMDetail(t *testing.T) {
	t.Run("full VM detail", func(t *testing.T) {
		raw := `{
			"metadata":{"name":"test-vm","namespace":"default","creationTimestamp":"2026-01-01T00:00:00Z"},
			"spec":{"template":{"spec":{
				"domain":{
					"cpu":{"cores":4,"sockets":2,"threads":2},
					"resources":{"requests":{"memory":"8Gi"},"limits":{"memory":"8Gi"}},
					"devices":{"disks":[
						{"name":"rootdisk","disk":{"bus":"virtio"}},
						{"name":"cdrom","cdrom":{}}
					]}
				},
				"volumes":[
					{"name":"rootdisk","dataVolume":{"name":"test-dv"}},
					{"name":"cloudinit","cloudInitNoCloud":{}}
				],
				"networks":[
					{"name":"default","pod":{}}
				]
			}}},
			"status":{
				"printableStatus":"Running","ready":true,"nodeName":"worker-1",
				"guestOSInfo":{"name":"Fedora","id":"fedora","version":"39","kernelRelease":"6.6.0"},
				"interfaces":[{"name":"default","ipAddress":"10.128.1.5","mac":"02:00:00:12:34:56"}],
				"conditions":[{"type":"Ready","status":"True"}]
			}
		}`
		detail, err := parseVMDetail(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if detail["name"] != "test-vm" {
			t.Errorf("name = %v", detail["name"])
		}
		if detail["status"] != "Running" {
			t.Errorf("status = %v", detail["status"])
		}
		if detail["node"] != "worker-1" {
			t.Errorf("node = %v", detail["node"])
		}
		cpu := detail["cpu"].(map[string]any)
		if cpu["cores"] != 4 {
			t.Errorf("cpu cores = %v", cpu["cores"])
		}
		if cpu["sockets"] != 2 {
			t.Errorf("cpu sockets = %v", cpu["sockets"])
		}
		disks := detail["disks"].([]map[string]string)
		if len(disks) != 2 {
			t.Errorf("got %d disks, want 2", len(disks))
		}
		if disks[0]["type"] != "disk" || disks[0]["bus"] != "virtio" {
			t.Errorf("disk[0] = %v", disks[0])
		}
		if disks[1]["type"] != "cdrom" {
			t.Errorf("disk[1] = %v", disks[1])
		}
		volumes := detail["volumes"].([]map[string]string)
		if len(volumes) != 2 {
			t.Errorf("got %d volumes, want 2", len(volumes))
		}
		networks := detail["networks"].([]map[string]string)
		if len(networks) != 1 || networks[0]["type"] != "pod" {
			t.Errorf("networks = %v", networks)
		}
		guestOS := detail["guestOS"].(map[string]string)
		if guestOS["name"] != "Fedora" {
			t.Errorf("guestOS name = %v", guestOS["name"])
		}
	})

	t.Run("minimal VM without status", func(t *testing.T) {
		raw := `{
			"metadata":{"name":"bare","namespace":"ns","creationTimestamp":"2026-01-01T00:00:00Z"},
			"spec":{"template":{"spec":{"domain":{"cpu":{"cores":1},"resources":{},"devices":{}}}}}
		}`
		detail, err := parseVMDetail(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if detail["name"] != "bare" {
			t.Errorf("name = %v", detail["name"])
		}
		if _, ok := detail["status"]; ok {
			t.Error("expected no status key for VM without status")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := parseVMDetail(json.RawMessage(`not-json`))
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestParseDVList(t *testing.T) {
	t.Run("parses DataVolumes", func(t *testing.T) {
		raw := `{"items":[
			{"metadata":{"name":"dv1","namespace":"default"},
			 "spec":{"source":{"http":{"url":"https://example.com/image.qcow2"}},"pvc":{"resources":{"requests":{"storage":"10Gi"}}}},
			 "status":{"phase":"Succeeded","progress":"100.0%"}},
			{"metadata":{"name":"dv2","namespace":"test"},
			 "spec":{"source":{"registry":{"url":"docker://quay.io/image"}}},
			 "status":{"phase":"ImportInProgress","progress":"45.2%"}},
			{"metadata":{"name":"dv3","namespace":"test"},
			 "spec":{"source":{"blank":{}}},
			 "status":{"phase":"Succeeded"}}
		]}`
		dvs, err := parseDVList(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(dvs) != 3 {
			t.Fatalf("got %d DVs, want 3", len(dvs))
		}
		if dvs[0].Phase != "Succeeded" || dvs[0].Size != "10Gi" {
			t.Errorf("dv[0] phase=%q size=%q", dvs[0].Phase, dvs[0].Size)
		}
		if dvs[0].Source != "http: https://example.com/image.qcow2" {
			t.Errorf("dv[0] source=%q", dvs[0].Source)
		}
		if dvs[1].Source != "registry: docker://quay.io/image" {
			t.Errorf("dv[1] source=%q", dvs[1].Source)
		}
		if dvs[2].Source != "blank" {
			t.Errorf("dv[2] source=%q", dvs[2].Source)
		}
	})

	t.Run("nil status", func(t *testing.T) {
		raw := `{"items":[{"metadata":{"name":"dv1","namespace":"ns"},"spec":{}}]}`
		dvs, err := parseDVList(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dvs[0].Phase != "Unknown" {
			t.Errorf("nil-status DV phase=%q, want Unknown", dvs[0].Phase)
		}
	})

	t.Run("empty items", func(t *testing.T) {
		dvs, err := parseDVList(`{"items":[]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(dvs) != 0 {
			t.Errorf("got %d DVs, want 0", len(dvs))
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := parseDVList(`{bad}`)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestParseNodeCapacity(t *testing.T) {
	t.Run("parses nodes with roles and resources", func(t *testing.T) {
		raw := json.RawMessage(`{"items":[
			{"metadata":{"name":"worker-1","labels":{"node-role.kubernetes.io/worker":"","node-role.kubernetes.io/infra":""}},
			 "status":{"capacity":{"cpu":"16","memory":"65536Mi"},
			           "allocatable":{"cpu":"15500m","memory":"63488Mi"}}},
			{"metadata":{"name":"master-1","labels":{"node-role.kubernetes.io/master":"","node-role.kubernetes.io/control-plane":""}},
			 "status":{"capacity":{"cpu":"8","memory":"32768Mi"},
			           "allocatable":{"cpu":"7500m","memory":"31744Mi"}}}
		]}`)
		nodes, err := parseNodeCapacity(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(nodes) != 2 {
			t.Fatalf("got %d nodes, want 2", len(nodes))
		}
		if nodes[0].Name != "worker-1" {
			t.Errorf("node[0] name = %q", nodes[0].Name)
		}
		if nodes[0].CPUCapacity != "16" || nodes[0].CPUAllocatable != "15500m" {
			t.Errorf("node[0] cpu: capacity=%q allocatable=%q", nodes[0].CPUCapacity, nodes[0].CPUAllocatable)
		}
		if nodes[0].MemoryCapacity != "65536Mi" || nodes[0].MemoryAllocatable != "63488Mi" {
			t.Errorf("node[0] memory: capacity=%q allocatable=%q", nodes[0].MemoryCapacity, nodes[0].MemoryAllocatable)
		}
		if len(nodes[0].Roles) != 2 {
			t.Errorf("node[0] roles count = %d, want 2", len(nodes[0].Roles))
		}
		if nodes[1].Name != "master-1" || nodes[1].CPUCapacity != "8" {
			t.Errorf("node[1] = %+v", nodes[1])
		}
	})

	t.Run("node without role labels", func(t *testing.T) {
		raw := json.RawMessage(`{"items":[
			{"metadata":{"name":"bare-node","labels":{"kubernetes.io/hostname":"bare-node"}},
			 "status":{"capacity":{"cpu":"4","memory":"16384Mi"},
			           "allocatable":{"cpu":"3500m","memory":"15360Mi"}}}
		]}`)
		nodes, err := parseNodeCapacity(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(nodes[0].Roles) != 0 {
			t.Errorf("expected no roles, got %v", nodes[0].Roles)
		}
	})

	t.Run("empty items", func(t *testing.T) {
		nodes, err := parseNodeCapacity(json.RawMessage(`{"items":[]}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(nodes) != 0 {
			t.Errorf("got %d nodes, want 0", len(nodes))
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := parseNodeCapacity(json.RawMessage(`{bad}`))
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

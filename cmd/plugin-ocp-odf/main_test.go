package main

import (
	"encoding/json"
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin()
	if p.ID != "ocp-odf" {
		t.Errorf("got %q, want %q", p.ID, "ocp-odf")
	}
}

func TestToolDefinitions(t *testing.T) {
	p := newPlugin()
	wantNames := []string{
		"odf_status", "odf_ceph_status", "odf_pools",
		"odf_pvcs", "odf_buckets", "odf_storage_classes", "odf_node_resources", "odf_health",
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

func TestParseStorageClusters(t *testing.T) {
	t.Run("parses clusters with status", func(t *testing.T) {
		raw := `{"items":[
			{"metadata":{"name":"ocs-storagecluster","namespace":"openshift-storage"},
			 "spec":{"version":"4.14"},
			 "status":{"phase":"Ready"}}
		]}`
		clusters, err := parseStorageClusters(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(clusters) != 1 {
			t.Fatalf("got %d, want 1", len(clusters))
		}
		if clusters[0].Name != "ocs-storagecluster" || clusters[0].Phase != "Ready" {
			t.Errorf("cluster = %+v", clusters[0])
		}
		if clusters[0].Version != "4.14" {
			t.Errorf("version = %q", clusters[0].Version)
		}
	})

	t.Run("nil status", func(t *testing.T) {
		raw := `{"items":[{"metadata":{"name":"sc","namespace":"ns"},"spec":{}}]}`
		clusters, err := parseStorageClusters(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if clusters[0].Phase != "Unknown" {
			t.Errorf("phase = %q, want Unknown", clusters[0].Phase)
		}
	})

	t.Run("empty items", func(t *testing.T) {
		clusters, err := parseStorageClusters(`{"items":[]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(clusters) != 0 {
			t.Errorf("got %d, want 0", len(clusters))
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := parseStorageClusters(`{bad}`)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestParseCephClusters(t *testing.T) {
	t.Run("parses ceph cluster", func(t *testing.T) {
		raw := `{"items":[
			{"metadata":{"name":"ocs-storagecluster-cephcluster","namespace":"openshift-storage"},
			 "spec":{"cephVersion":{"image":"registry.redhat.io/rhceph/rhceph-6-rhel9:v6.1"},"mon":{"count":3}},
			 "status":{"phase":"Ready","ceph":{"health":"HEALTH_OK","details":"all good"},
			  "storage":{"capacity":{"totalBytes":"107374182400","usedBytes":"21474836480","availableBytes":"85899345920"},
			   "osd":{"storeCount":{"bluestore":3}}}}}
		]}`
		clusters, err := parseCephClusters(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(clusters) != 1 {
			t.Fatalf("got %d, want 1", len(clusters))
		}
		c := clusters[0]
		if c.Health != "HEALTH_OK" {
			t.Errorf("health = %q", c.Health)
		}
		if c.MonCount != 3 {
			t.Errorf("monCount = %d", c.MonCount)
		}
		if c.Version != "v6.1" {
			t.Errorf("version = %q", c.Version)
		}
		if c.Capacity == nil {
			t.Fatal("capacity is nil")
		}
		if c.OSDStatus == nil || c.OSDStatus.Total != 3 {
			t.Errorf("osdStatus = %+v", c.OSDStatus)
		}
	})

	t.Run("nil status", func(t *testing.T) {
		raw := `{"items":[{"metadata":{"name":"cc","namespace":"ns"},"spec":{"cephVersion":{},"mon":{"count":0}}}]}`
		clusters, err := parseCephClusters(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if clusters[0].Phase != "Unknown" || clusters[0].Health != "Unknown" {
			t.Errorf("phase=%q health=%q", clusters[0].Phase, clusters[0].Health)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := parseCephClusters(`not-json`)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestParseBlockPools(t *testing.T) {
	t.Run("replicated pool", func(t *testing.T) {
		raw := `{"items":[
			{"metadata":{"name":"ocs-storagecluster-cephblockpool","namespace":"openshift-storage"},
			 "spec":{"replicated":{"size":3},"erasureCoded":{"dataChunks":0,"codingChunks":0}},
			 "status":{"phase":"Ready"}}
		]}`
		pools, err := parseBlockPools(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pools) != 1 {
			t.Fatalf("got %d, want 1", len(pools))
		}
		if pools[0].Kind != "CephBlockPool" || pools[0].Replication != 3 {
			t.Errorf("pool = %+v", pools[0])
		}
	})

	t.Run("erasure coded pool", func(t *testing.T) {
		raw := `{"items":[
			{"metadata":{"name":"ec-pool","namespace":"openshift-storage"},
			 "spec":{"replicated":{"size":0},"erasureCoded":{"dataChunks":2,"codingChunks":1}},
			 "status":{"phase":"Ready"}}
		]}`
		pools, err := parseBlockPools(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pools[0].ECK != 2 || pools[0].ECM != 1 {
			t.Errorf("ec pool: k=%d m=%d", pools[0].ECK, pools[0].ECM)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := parseBlockPools(`bad`)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestParseFilesystems(t *testing.T) {
	t.Run("parses filesystem", func(t *testing.T) {
		raw := `{"items":[
			{"metadata":{"name":"ocs-storagecluster-cephfilesystem","namespace":"openshift-storage"},
			 "spec":{"dataPools":[{"replicated":{"size":3}}]},
			 "status":{"phase":"Ready"}}
		]}`
		pools, err := parseFilesystems(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pools) != 1 {
			t.Fatalf("got %d, want 1", len(pools))
		}
		if pools[0].Kind != "CephFilesystem" || pools[0].Replication != 3 {
			t.Errorf("pool = %+v", pools[0])
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := parseFilesystems(`bad`)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestParsePVCs(t *testing.T) {
	t.Run("filters by ODF class", func(t *testing.T) {
		odfClasses := map[string]bool{
			"ocs-storagecluster-ceph-rbd": true,
		}
		raw := `{"items":[
			{"metadata":{"name":"pvc1","namespace":"default"},
			 "spec":{"storageClassName":"ocs-storagecluster-ceph-rbd","accessModes":["ReadWriteOnce"],"volumeName":"pv-1"},
			 "status":{"phase":"Bound","capacity":{"storage":"10Gi"}}},
			{"metadata":{"name":"pvc2","namespace":"default"},
			 "spec":{"storageClassName":"gp3","accessModes":["ReadWriteOnce"],"volumeName":"pv-2"},
			 "status":{"phase":"Bound","capacity":{"storage":"20Gi"}}}
		]}`
		pvcs, err := parsePVCs(raw, odfClasses)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pvcs) != 1 {
			t.Fatalf("got %d PVCs, want 1 (only ODF class)", len(pvcs))
		}
		if pvcs[0].Name != "pvc1" || pvcs[0].Capacity != "10Gi" {
			t.Errorf("pvc = %+v", pvcs[0])
		}
		if pvcs[0].AccessModes != "ReadWriteOnce" {
			t.Errorf("accessModes = %q", pvcs[0].AccessModes)
		}
	})

	t.Run("empty when no ODF classes match", func(t *testing.T) {
		raw := `{"items":[
			{"metadata":{"name":"pvc1","namespace":"ns"},
			 "spec":{"storageClassName":"gp2","accessModes":[],"volumeName":"pv-1"},
			 "status":{"phase":"Bound","capacity":{"storage":"5Gi"}}}
		]}`
		pvcs, err := parsePVCs(raw, map[string]bool{"ocs-rbd": true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pvcs) != 0 {
			t.Errorf("got %d PVCs, want 0", len(pvcs))
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := parsePVCs(`bad`, nil)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestParseBuckets(t *testing.T) {
	t.Run("parses buckets", func(t *testing.T) {
		raw := `{"items":[
			{"metadata":{"name":"bucket1","namespace":"default"},
			 "spec":{"storageClassName":"openshift-storage.noobaa.io"},
			 "status":{"phase":"Bound"}}
		]}`
		buckets, err := parseBuckets(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(buckets) != 1 {
			t.Fatalf("got %d, want 1", len(buckets))
		}
		if buckets[0].Phase != "Bound" || buckets[0].StorageClass != "openshift-storage.noobaa.io" {
			t.Errorf("bucket = %+v", buckets[0])
		}
	})

	t.Run("nil status", func(t *testing.T) {
		raw := `{"items":[{"metadata":{"name":"b","namespace":"ns"},"spec":{}}]}`
		buckets, err := parseBuckets(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if buckets[0].Phase != "Unknown" {
			t.Errorf("phase = %q, want Unknown", buckets[0].Phase)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := parseBuckets(`{bad}`)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestParseStorageClasses(t *testing.T) {
	t.Run("filters ODF provisioners", func(t *testing.T) {
		raw := `{"items":[
			{"metadata":{"name":"ocs-rbd","annotations":{"storageclass.kubernetes.io/is-default-class":"true"}},
			 "provisioner":"openshift-storage.rbd.csi.ceph.com","reclaimPolicy":"Delete","volumeBindingMode":"Immediate"},
			{"metadata":{"name":"gp3","annotations":{}},
			 "provisioner":"ebs.csi.aws.com","reclaimPolicy":"Delete","volumeBindingMode":"WaitForFirstConsumer"},
			{"metadata":{"name":"ocs-cephfs","annotations":{}},
			 "provisioner":"openshift-storage.cephfs.csi.ceph.com","reclaimPolicy":"Delete"}
		]}`
		classes, err := parseStorageClasses(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(classes) != 2 {
			t.Fatalf("got %d, want 2 (only ODF provisioners)", len(classes))
		}
		if classes[0].Name != "ocs-rbd" || !classes[0].Default {
			t.Errorf("class[0] = %+v", classes[0])
		}
		if classes[1].Name != "ocs-cephfs" {
			t.Errorf("class[1] = %+v", classes[1])
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := parseStorageClasses(`bad`)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestParseNodeResources(t *testing.T) {
	t.Run("parses nodes with capacity and allocatable", func(t *testing.T) {
		raw := json.RawMessage(`{"items":[
			{"metadata":{"name":"worker-1"},
			 "status":{"capacity":{"cpu":"16","memory":"65536Mi","ephemeral-storage":"200Gi"},
			           "allocatable":{"cpu":"15500m","memory":"63488Mi","ephemeral-storage":"180Gi"}}},
			{"metadata":{"name":"worker-2"},
			 "status":{"capacity":{"cpu":"8","memory":"32768Mi","ephemeral-storage":"100Gi"},
			           "allocatable":{"cpu":"7500m","memory":"31744Mi","ephemeral-storage":"90Gi"}}}
		]}`)
		nodes, err := parseNodeResources(raw)
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
		if nodes[0].StorageCapacity != "200Gi" || nodes[0].StorageAllocatable != "180Gi" {
			t.Errorf("node[0] storage: capacity=%q allocatable=%q", nodes[0].StorageCapacity, nodes[0].StorageAllocatable)
		}
		if nodes[1].Name != "worker-2" || nodes[1].CPUCapacity != "8" {
			t.Errorf("node[1] = %+v", nodes[1])
		}
	})

	t.Run("empty items", func(t *testing.T) {
		nodes, err := parseNodeResources(json.RawMessage(`{"items":[]}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(nodes) != 0 {
			t.Errorf("got %d nodes, want 0", len(nodes))
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := parseNodeResources(json.RawMessage(`{bad}`))
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input json.Number
		want  string
	}{
		{json.Number("107374182400"), "100.0 GiB"},
		{json.Number("1099511627776"), "1.0 TiB"},
		{json.Number("1048576"), "1.0 MiB"},
		{json.Number("512"), "512 B"},
		{json.Number("not-a-number"), "not-a-number"},
	}
	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got := formatBytes(tt.input)
			if got != tt.want {
				t.Errorf("formatBytes(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

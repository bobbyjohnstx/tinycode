package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"
)

// jsonValue creates a JSON-encoded Kubernetes-like resource.
func jsonValue(apiVersion, kind string) []byte {
	obj := map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]any{
			"name": "test",
		},
	}
	b, _ := json.Marshal(obj)
	return b
}

// protoValue creates a minimal k8s protobuf-encoded value with TypeMeta.
func protoValue(apiVersion, kind string) []byte {
	// Build TypeMeta submessage: field 1 = apiVersion, field 2 = kind
	var typeMeta []byte
	typeMeta = appendLengthDelimited(typeMeta, 1, []byte(apiVersion))
	typeMeta = appendLengthDelimited(typeMeta, 2, []byte(kind))

	// Build outer message: field 1 = TypeMeta submessage
	var outer []byte
	outer = appendLengthDelimited(outer, 1, typeMeta)

	// Prepend k8s magic
	result := make([]byte, 0, 4+len(outer))
	result = append(result, 'k', '8', 's', 0x00)
	result = append(result, outer...)
	return result
}

// appendLengthDelimited appends a protobuf length-delimited field.
func appendLengthDelimited(buf []byte, fieldNum int, data []byte) []byte {
	// tag = (fieldNum << 3) | 2
	tag := byte((fieldNum << 3) | 2)
	buf = append(buf, tag)
	buf = appendVarint(buf, uint64(len(data)))
	buf = append(buf, data...)
	return buf
}

// appendVarint appends a protobuf varint.
func appendVarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	buf = append(buf, byte(v))
	return buf
}

// setupTestSnapshot creates a bbolt DB with synthetic /registry/ keys.
func setupTestSnapshot(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket([]byte("key"))
		if err != nil {
			return err
		}
		b.Put([]byte("/registry/pods/default/test-pod"), jsonValue("v1", "Pod"))
		b.Put([]byte("/registry/pods/default/nginx-pod"), jsonValue("v1", "Pod"))
		b.Put([]byte("/registry/pods/kube-system/kube-proxy"), jsonValue("v1", "Pod"))
		b.Put([]byte("/registry/configmaps/kube-system/coredns"), jsonValue("v1", "ConfigMap"))
		b.Put([]byte("/registry/configmaps/default/app-config"), jsonValue("v1", "ConfigMap"))
		b.Put([]byte("/registry/deployments/default/nginx"), jsonValue("apps/v1", "Deployment"))
		b.Put([]byte("/registry/nodes/worker-1"), jsonValue("v1", "Node"))
		b.Put([]byte("/registry/nodes/worker-2"), jsonValue("v1", "Node"))
		b.Put([]byte("/registry/services/default/kubernetes"), jsonValue("v1", "Service"))
		b.Put([]byte("/registry/secrets/default/my-secret"), protoValue("v1", "Secret"))
		// Large value for storage analysis.
		b.Put([]byte("/registry/configmaps/default/large-cm"), make([]byte, 4096))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	return path
}

func TestSnapshotOpen(t *testing.T) {
	path := setupTestSnapshot(t)
	st := &state{}

	out, err := toolSnapshotOpen(st, path)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "etcd Snapshot Opened")
	assertContains(t, out, "Total keys: 11")
	assertContains(t, out, "pods")
	assertContains(t, out, "configmaps")
	assertContains(t, out, "nodes")

	// Verify DB was stored in state.
	if st.getDB() == nil {
		t.Error("DB was not stored in state after open")
	}
}

func TestSnapshotOpenClosePrevious(t *testing.T) {
	path := setupTestSnapshot(t)
	st := &state{}

	// Open first time.
	_, err := toolSnapshotOpen(st, path)
	if err != nil {
		t.Fatal(err)
	}
	firstDB := st.getDB()
	if firstDB == nil {
		t.Fatal("DB not stored")
	}

	// Create another snapshot and open it — previous should be closed.
	path2 := filepath.Join(t.TempDir(), "test2.db")
	db2, err := bolt.Open(path2, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	db2.Update(func(tx *bolt.Tx) error {
		b, _ := tx.CreateBucket([]byte("key"))
		b.Put([]byte("/registry/pods/default/p1"), jsonValue("v1", "Pod"))
		return nil
	})
	db2.Close()

	_, err = toolSnapshotOpen(st, path2)
	if err != nil {
		t.Fatal(err)
	}
	if st.getDB() == firstDB {
		t.Error("DB was not replaced after second open")
	}
}

func TestSnapshotResources(t *testing.T) {
	path := setupTestSnapshot(t)
	st := &state{}
	if _, err := toolSnapshotOpen(st, path); err != nil {
		t.Fatal(err)
	}

	db, err := requireDB(st)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolSnapshotResources(db, "count")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "etcd Snapshot Resources")
	assertContains(t, out, "TYPE")
	assertContains(t, out, "COUNT")
	assertContains(t, out, "TOTAL_SIZE")
	assertContains(t, out, "AVG_SIZE")
	assertContains(t, out, "pods")
	assertContains(t, out, "configmaps")

	// Sort by size.
	out, err = toolSnapshotResources(db, "size")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "configmaps")
}

func TestSnapshotGetJSON(t *testing.T) {
	path := setupTestSnapshot(t)
	st := &state{}
	if _, err := toolSnapshotOpen(st, path); err != nil {
		t.Fatal(err)
	}

	db, err := requireDB(st)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolSnapshotGet(db, "pods", "default", "test-pod")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Encoding: JSON")
	assertContains(t, out, `"kind": "Pod"`)
	assertContains(t, out, `"apiVersion": "v1"`)
}

func TestSnapshotGetProtobuf(t *testing.T) {
	path := setupTestSnapshot(t)
	st := &state{}
	if _, err := toolSnapshotOpen(st, path); err != nil {
		t.Fatal(err)
	}

	db, err := requireDB(st)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolSnapshotGet(db, "secrets", "default", "my-secret")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Encoding: protobuf")
	assertContains(t, out, "APIVersion: v1")
	assertContains(t, out, "Kind: Secret")
}

func TestSnapshotGetClusterScoped(t *testing.T) {
	path := setupTestSnapshot(t)
	st := &state{}
	if _, err := toolSnapshotOpen(st, path); err != nil {
		t.Fatal(err)
	}

	db, err := requireDB(st)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolSnapshotGet(db, "nodes", "", "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Encoding: JSON")
	assertContains(t, out, `"kind": "Node"`)
}

func TestSnapshotGetNotFound(t *testing.T) {
	path := setupTestSnapshot(t)
	st := &state{}
	if _, err := toolSnapshotOpen(st, path); err != nil {
		t.Fatal(err)
	}

	db, err := requireDB(st)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolSnapshotGet(db, "pods", "default", "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Key not found")
}

func TestSnapshotSearchSubstring(t *testing.T) {
	path := setupTestSnapshot(t)
	st := &state{}
	if _, err := toolSnapshotOpen(st, path); err != nil {
		t.Fatal(err)
	}

	db, err := requireDB(st)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolSnapshotSearch(db, "nginx", 50)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "etcd Snapshot Search")
	assertContains(t, out, "nginx")
	assertContains(t, out, "Matches: 2") // nginx-pod and nginx deployment
}

func TestSnapshotSearchRegex(t *testing.T) {
	path := setupTestSnapshot(t)
	st := &state{}
	if _, err := toolSnapshotOpen(st, path); err != nil {
		t.Fatal(err)
	}

	db, err := requireDB(st)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolSnapshotSearch(db, "worker-[12]", 50)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Matches: 2")
}

func TestSnapshotSearchMaxLimit(t *testing.T) {
	path := setupTestSnapshot(t)
	st := &state{}
	if _, err := toolSnapshotOpen(st, path); err != nil {
		t.Fatal(err)
	}

	db, err := requireDB(st)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolSnapshotSearch(db, "/registry/", 3)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "more matches")
}

func TestSnapshotStorage(t *testing.T) {
	path := setupTestSnapshot(t)
	st := &state{}
	if _, err := toolSnapshotOpen(st, path); err != nil {
		t.Fatal(err)
	}

	db, err := requireDB(st)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolSnapshotStorage(st, db)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "etcd Snapshot Storage Analysis")
	assertContains(t, out, "Total keys: 11")
	assertContains(t, out, "Top 20 largest keys")
	assertContains(t, out, "large-cm")
	assertContains(t, out, "Size distribution by resource type")
}

func TestRequireDBGuard(t *testing.T) {
	st := &state{}
	_, err := requireDB(st)
	if err == nil {
		t.Error("requireDB should return error when no DB is open")
	}
	if !strings.Contains(err.Error(), "no snapshot opened") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDisposeClosesDB(t *testing.T) {
	path := setupTestSnapshot(t)
	st := &state{}
	if _, err := toolSnapshotOpen(st, path); err != nil {
		t.Fatal(err)
	}
	if st.getDB() == nil {
		t.Fatal("DB should be open")
	}

	p := newPlugin()
	if p.Hooks.Dispose == nil {
		t.Fatal("Dispose hook should be registered")
	}
	// Note: Can't call Dispose on the test state directly since newPlugin creates its own,
	// but we can verify closeDB works.
	err := st.closeDB()
	if err != nil {
		t.Fatal(err)
	}
	if st.getDB() != nil {
		t.Error("DB should be nil after closeDB")
	}
}

func TestDecodeTypeMeta(t *testing.T) {
	t.Run("valid protobuf", func(t *testing.T) {
		value := protoValue("v1", "Pod")
		tm, err := decodeTypeMeta(value)
		if err != nil {
			t.Fatal(err)
		}
		if tm.APIVersion != "v1" {
			t.Errorf("APIVersion = %q, want %q", tm.APIVersion, "v1")
		}
		if tm.Kind != "Pod" {
			t.Errorf("Kind = %q, want %q", tm.Kind, "Pod")
		}
	})

	t.Run("apps/v1 Deployment", func(t *testing.T) {
		value := protoValue("apps/v1", "Deployment")
		tm, err := decodeTypeMeta(value)
		if err != nil {
			t.Fatal(err)
		}
		if tm.APIVersion != "apps/v1" {
			t.Errorf("APIVersion = %q, want %q", tm.APIVersion, "apps/v1")
		}
		if tm.Kind != "Deployment" {
			t.Errorf("Kind = %q, want %q", tm.Kind, "Deployment")
		}
	})

	t.Run("not protobuf", func(t *testing.T) {
		_, err := decodeTypeMeta([]byte(`{"kind":"Pod"}`))
		if err == nil {
			t.Error("expected error for non-protobuf data")
		}
	})

	t.Run("too short", func(t *testing.T) {
		_, err := decodeTypeMeta([]byte{0x6b})
		if err == nil {
			t.Error("expected error for short data")
		}
	})
}

package libvirtbackend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCPUList(t *testing.T) {
	got, ok := parseCPUList("0-2,5,8-9")
	if !ok || len(got) != 6 {
		t.Fatalf("parseCPUList() = %v, %v", got, ok)
	}
	for index, want := range []uint{0, 1, 2, 5, 8, 9} {
		if got[index] != want {
			t.Fatalf("CPU[%d] = %d, want %d", index, got[index], want)
		}
	}
	if _, ok := parseCPUList("3-1"); ok {
		t.Fatal("reversed range was accepted")
	}
}

func TestReadHostTopologyFromFixture(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(path, value string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(filepath.Join(root, "devices", "system", "node", "online"), "0-1\n")
	mustWrite(filepath.Join(root, "devices", "system", "node", "node0", "cpulist"), "0-3\n")
	mustWrite(filepath.Join(root, "devices", "system", "node", "node0", "meminfo"), "Node 0 MemTotal: 1048576 kB\nNode 0 MemFree: 524288 kB\n")
	mustWrite(filepath.Join(root, "devices", "system", "node", "node1", "cpulist"), "4-7\n")
	mustWrite(filepath.Join(root, "devices", "system", "node", "node1", "meminfo"), "Node 1 MemTotal: 2097152 kB\nNode 1 MemFree: 1048576 kB\n")
	mustWrite(filepath.Join(root, "kernel", "mm", "hugepages", "hugepages-2048kB", "nr_hugepages"), "1024\n")
	mustWrite(filepath.Join(root, "kernel", "mm", "hugepages", "hugepages-2048kB", "free_hugepages"), "512\n")
	nodes, pages := readHostTopologyFrom(root)
	if len(nodes) != 2 || nodes[1].ID != 1 || nodes[1].MemoryBytes != 2<<30 {
		t.Fatalf("nodes = %+v", nodes)
	}
	if len(pages) != 1 || pages[0].SizeBytes != 2<<20 || pages[0].Total != 1024 || pages[0].Free != 512 {
		t.Fatalf("hugepages = %+v", pages)
	}
}

func TestHugePageBytes(t *testing.T) {
	for value, want := range map[string]uint64{"2KiB": 2 << 10, "2MiB": 2 << 20, "1GiB": 1 << 30} {
		got, err := hugePageBytes(value)
		if err != nil || got != want {
			t.Fatalf("hugePageBytes(%q) = %d, %v; want %d", value, got, err, want)
		}
	}
	for _, value := range []string{"", "2MB", "0MiB", "18446744073709551615GiB"} {
		if _, err := hugePageBytes(value); err == nil {
			t.Fatalf("hugePageBytes(%q) accepted invalid value", value)
		}
	}
}

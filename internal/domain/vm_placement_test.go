package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeVMPlacementValidatesTopology(t *testing.T) {
	spec, err := NormalizeVMPlacement(VMPlacementSpec{
		NUMACells:    []VMNUMACell{{ID: 1, Cpuset: "4-7", MemoryMiB: 1024}, {ID: 0, Cpuset: "0-3", MemoryMiB: 1024}},
		HugePageSize: "2MiB",
		VCPUPins:     []VMCpuPin{{VCPU: 1, Cpuset: "4-5"}, {VCPU: 0, Cpuset: "0-1"}},
		EmulatorPin:  "8",
	}, 8, 2048)
	if err != nil {
		t.Fatalf("NormalizeVMPlacement() error = %v", err)
	}
	if spec.NUMACells[0].ID != 0 || spec.VCPUPins[0].VCPU != 0 {
		t.Fatalf("placement was not sorted: %+v", spec)
	}
	_, err = NormalizeVMPlacement(VMPlacementSpec{NUMACells: []VMNUMACell{{ID: 0, Cpuset: "0-1", MemoryMiB: 512}}}, 2, 1024)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("NUMA memory mismatch error = %v", err)
	}
	if _, err := NormalizeVMPlacement(VMPlacementSpec{NUMACells: []VMNUMACell{{ID: 0, Cpuset: "0-1", MemoryMiB: 512}, {ID: 1, Cpuset: "1-2", MemoryMiB: 512}}}, 3, 1024); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("overlapping NUMA cells error = %v", err)
	}
	if _, err := NormalizeVMPlacement(VMPlacementSpec{HugePageSize: "3MiB"}, 1, 1024); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("non-divisible huge page error = %v", err)
	}
}

func TestSetVMXMLPlacementWritesNUMAHugePagesAndPins(t *testing.T) {
	input := `<domain type='kvm'><name>demo</name><memory unit='MiB'>2048</memory><vcpu>4</vcpu><os><type arch='x86_64'>hvm</type></os><devices></devices></domain>`
	output, err := SetVMXMLPlacement(input, VMPlacementSpec{
		NUMACells:    []VMNUMACell{{ID: 0, Cpuset: "0-1", MemoryMiB: 1024}, {ID: 1, Cpuset: "2-3", MemoryMiB: 1024}},
		HugePageSize: "1GiB",
		VCPUPins:     []VMCpuPin{{VCPU: 0, Cpuset: "0"}, {VCPU: 1, Cpuset: "1"}},
		EmulatorPin:  "4",
	})
	if err != nil {
		t.Fatalf("SetVMXMLPlacement() error = %v", err)
	}
	for _, fragment := range []string{`placement="static"`, "<numa>", `cpus="0-1"`, `memory="1024"`, "<hugepages>", `size="1" unit="GiB"`, `<vcpupin vcpu="0" cpuset="0"`, `<emulatorpin cpuset="4"`} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, output)
		}
	}
	cleared, err := SetVMXMLPlacement(output, VMPlacementSpec{})
	if err != nil {
		t.Fatalf("clear placement error = %v", err)
	}
	for _, fragment := range []string{"<numa>", "<hugepages>", "<cputune>", `placement="static"`} {
		if strings.Contains(cleared, fragment) {
			t.Fatalf("cleared output still contains %q:\n%s", fragment, cleared)
		}
	}
}

func TestParseCPUSetRejectsOversizedRanges(t *testing.T) {
	if _, ok := ParseCPUSet("0-65535"); !ok {
		t.Fatal("maximum CPU set was rejected")
	}
	for _, value := range []string{"0-65536", "0-4294967295", "0,1-999999999"} {
		if _, ok := ParseCPUSet(value); ok {
			t.Fatalf("oversized CPU set accepted: %q", value)
		}
	}
}

func TestNormalizeVMPlacementRejectsHugePageOverflow(t *testing.T) {
	for _, value := range []string{"18446744073709551615GiB", "18446744073709551615MiB"} {
		if _, err := NormalizeVMPlacement(VMPlacementSpec{HugePageSize: value}, 1, 1024); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("overflow value %q error = %v", value, err)
		}
	}
}

func TestSetVMXMLPlacementPreservesOtherCPUTuneSettings(t *testing.T) {
	input := `<domain type='kvm'><name>demo</name><memory unit='MiB'>1024</memory><vcpu>2</vcpu><cputune><shares>2048</shares><period>100000</period><vcpupin vcpu='0' cpuset='9'/></cputune><os><type>hvm</type></os></domain>`
	output, err := SetVMXMLPlacement(input, VMPlacementSpec{VCPUPins: []VMCpuPin{{VCPU: 0, Cpuset: "1"}, {VCPU: 1, Cpuset: "2"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"<shares>2048</shares>", "<period>100000</period>", "cpuset=\"1\"", "cpuset=\"2\""} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, output)
		}
	}
	if strings.Contains(output, "cpuset=\"9\"") {
		t.Fatalf("stale vCPU pin retained:\n%s", output)
	}
}

func TestSetVMXMLPlacementDoesNotAddCPUForHugePagesOnly(t *testing.T) {
	input := `<domain type='kvm'><name>demo</name><memory unit='MiB'>1024</memory><vcpu>2</vcpu><os><type>hvm</type></os></domain>`
	output, err := SetVMXMLPlacement(input, VMPlacementSpec{HugePageSize: "2MiB"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, "<cpu") || !strings.Contains(output, "<hugepages>") {
		t.Fatalf("huge pages changed CPU configuration unexpectedly:\n%s", output)
	}
}

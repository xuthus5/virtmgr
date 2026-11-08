package libvirtbackend

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func TestParseStoragePoolXML(t *testing.T) {
	const text = `<pool type='dir'><name>default</name><uuid>aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa</uuid><target><path>/var/lib/libvirt/images</path></target></pool>`
	var parsed storagePoolXML
	if err := xml.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Type != "dir" || parsed.Target.Path != "/var/lib/libvirt/images" {
		t.Fatalf("parsed pool = %+v", parsed)
	}
}

func TestParseStorageVolumeXML(t *testing.T) {
	const text = `<volume type='file'><name>demo.qcow2</name><target><path>/var/lib/libvirt/images/demo.qcow2</path><format type='qcow2'/></target></volume>`
	var parsed storageVolumeXML
	if err := xml.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Type != "file" || parsed.Target.Format.Type != "qcow2" {
		t.Fatalf("parsed volume = %+v", parsed)
	}
}

func TestBuildStorageVolumeXML(t *testing.T) {
	text := buildStorageVolumeXML(domain.StorageVolumeCreateSpec{
		Name: "data&01.qcow2", Format: "qcow2", CapacityBytes: 20 << 30,
	})
	var parsed struct {
		Type     string `xml:"type,attr"`
		Name     string `xml:"name"`
		Capacity struct {
			Unit  string `xml:"unit,attr"`
			Value uint64 `xml:",chardata"`
		} `xml:"capacity"`
		Target struct {
			Format struct {
				Type string `xml:"type,attr"`
			} `xml:"format"`
		} `xml:"target"`
	}
	if err := xml.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Type != "file" || parsed.Name != "data&01.qcow2" || parsed.Capacity.Unit != "bytes" || parsed.Capacity.Value != 20<<30 {
		t.Fatalf("parsed volume = %+v XML=%s", parsed, text)
	}
	if parsed.Target.Format.Type != "qcow2" {
		t.Fatalf("format = %q", parsed.Target.Format.Type)
	}
}

func TestBuildStoragePoolXML(t *testing.T) {
	text := buildStoragePoolXML(domain.StoragePoolCreateSpec{Name: "data&pool", TargetPath: "/srv/virtmgr/<data>"})
	var parsed struct {
		Type   string `xml:"type,attr"`
		Name   string `xml:"name"`
		Target struct {
			Path string `xml:"path"`
		} `xml:"target"`
	}
	if err := xml.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Type != "dir" || parsed.Name != "data&pool" || parsed.Target.Path != "/srv/virtmgr/<data>" {
		t.Fatalf("parsed pool = %+v XML=%s", parsed, text)
	}
}

func TestDiskUsesVolume(t *testing.T) {
	const path = "/var/lib/libvirt/images/data.qcow2"
	tests := []struct {
		name   string
		source string
		want   bool
	}{
		{name: "absolute path", source: path, want: true},
		{name: "cleaned path", source: "/var/lib/libvirt/images/./data.qcow2", want: true},
		{name: "volume name", source: "data.qcow2", want: true},
		{name: "different path", source: "/var/lib/libvirt/images/other.qcow2", want: false},
		{name: "empty", source: " ", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := diskUsesVolume(test.source, path, "data.qcow2"); got != test.want {
				t.Fatalf("diskUsesVolume() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestMapStorageStates(t *testing.T) {
	tests := map[libvirt.StoragePoolState]domain.StoragePoolState{
		libvirt.STORAGE_POOL_RUNNING:      domain.StoragePoolStateActive,
		libvirt.STORAGE_POOL_INACTIVE:     domain.StoragePoolStateInactive,
		libvirt.STORAGE_POOL_DEGRADED:     domain.StoragePoolStateDegraded,
		libvirt.STORAGE_POOL_INACCESSIBLE: domain.StoragePoolStateInaccessible,
	}
	for state, want := range tests {
		if got := mapStoragePoolState(state); got != want {
			t.Fatalf("mapStoragePoolState(%d) = %q, want %q", state, got, want)
		}
	}
	if got := mapStorageVolumeType(libvirt.STORAGE_VOL_FILE); got != "file" {
		t.Fatalf("mapStorageVolumeType() = %q", got)
	}
	if got := mapStorageVolumeType(libvirt.STORAGE_VOL_NETWORK); got != "network" {
		t.Fatalf("mapStorageVolumeType(network) = %q", got)
	}
}

type recordingStreamSender struct {
	body []byte
	err  error
	cap  int
}

func (s *recordingStreamSender) Send(body []byte) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	if s.cap > 0 && len(body) > s.cap {
		body = body[:s.cap]
	}
	s.body = append(s.body, body...)
	return len(body), nil
}

func TestCopyToStorageStream(t *testing.T) {
	sender := &recordingStreamSender{}
	if err := copyToStorageStream(sender, strings.NewReader("hello"), 5); err != nil {
		t.Fatalf("copyToStorageStream() error = %v", err)
	}
	if string(sender.body) != "hello" {
		t.Fatalf("sent body = %q", sender.body)
	}

	if err := copyToStorageStream(&recordingStreamSender{}, strings.NewReader("hi"), 5); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("truncated copy error = %v", err)
	}
	if err := copyToStorageStream(&recordingStreamSender{}, nil, 5); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("nil reader error = %v", err)
	}
	if err := copyToStorageStream(&recordingStreamSender{}, strings.NewReader("hello"), 0); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("zero size error = %v", err)
	}
	want := errors.New("stream broken")
	if err := copyToStorageStream(&recordingStreamSender{err: want}, strings.NewReader("hello"), 5); !errors.Is(err, want) {
		t.Fatalf("sender error = %v", err)
	}
	if err := copyToStorageStream(&recordingStreamSender{cap: 3}, strings.NewReader("hello"), 5); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write error = %v", err)
	}
}

package libvirtbackend

import (
	"context"
	"io"
	"testing"
)

func TestDemoOpenConsoleHandshake(t *testing.T) {
	backend := NewDemo()
	for _, vm := range backend.vms {
		if vm.State == "running" {
			connection, err := backend.OpenConsole(context.Background(), vm.UUID, 0)
			if err != nil {
				t.Fatalf("OpenConsole() error = %v", err)
			}
			defer connection.Close()
			greeting := make([]byte, 12)
			if _, err := io.ReadFull(connection, greeting); err != nil {
				t.Fatalf("read greeting: %v", err)
			}
			if string(greeting) != "RFB 003.003\n" {
				t.Fatalf("greeting = %q", greeting)
			}
			return
		}
	}
	t.Fatal("demo backend has no running VM")
}

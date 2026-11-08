package libvirtbackend

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"time"

	"virtmgr/internal/domain"
)

const demoVNCName = "VirtMgr demo VNC console"

func (d *DemoBackend) OpenConsole(_ context.Context, uuid string, graphicsIndex uint) (io.ReadWriteCloser, error) {
	d.mu.RLock()
	vm, ok := d.vms[uuid]
	d.mu.RUnlock()
	if !ok {
		return nil, domain.ErrNotFound
	}
	if graphicsIndex >= uint(len(vm.Graphics)) || !equalFold(vm.Graphics[graphicsIndex].Type, "vnc") {
		return nil, domain.ErrConsoleUnavailable
	}
	if vm.State != domain.VMStateRunning && vm.State != domain.VMStateBlocked && vm.State != domain.VMStatePaused {
		return nil, domain.ErrConsoleUnavailable
	}
	client, server := net.Pipe()
	go serveDemoVNC(server)
	return client, nil
}

func serveDemoVNC(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.WriteString(conn, "RFB 003.003\n"); err != nil {
		return
	}
	version := make([]byte, 12)
	if _, err := io.ReadFull(conn, version); err != nil {
		return
	}
	securityNone := make([]byte, 4)
	binary.BigEndian.PutUint32(securityNone, 1)
	if _, err := conn.Write(securityNone); err != nil {
		return
	}
	clientInit := make([]byte, 1)
	if _, err := io.ReadFull(conn, clientInit); err != nil {
		return
	}
	init := make([]byte, 24+len(demoVNCName))
	binary.BigEndian.PutUint16(init[0:2], 1024)
	binary.BigEndian.PutUint16(init[2:4], 768)
	init[4] = 32
	init[5] = 24
	init[6] = 0
	init[7] = 1
	binary.BigEndian.PutUint16(init[8:10], 255)
	binary.BigEndian.PutUint16(init[10:12], 255)
	binary.BigEndian.PutUint16(init[12:14], 255)
	init[14] = 16
	init[15] = 8
	init[16] = 0
	binary.BigEndian.PutUint32(init[20:24], uint32(len(demoVNCName)))
	copy(init[24:], demoVNCName)
	if _, err := conn.Write(init); err != nil {
		return
	}
	_ = conn.SetDeadline(time.Time{})
	_, _ = io.Copy(io.Discard, conn)
}

func equalFold(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		a, b := left[index], right[index]
		if 'A' <= a && a <= 'Z' {
			a += 'a' - 'A'
		}
		if 'A' <= b && b <= 'Z' {
			b += 'a' - 'A'
		}
		if a != b {
			return false
		}
	}
	return true
}

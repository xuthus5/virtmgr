package libvirtbackend

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
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

func (d *DemoBackend) OpenSerialConsole(_ context.Context, uuid, device string) (io.ReadWriteCloser, error) {
	d.mu.RLock()
	vm, ok := d.vms[uuid]
	d.mu.RUnlock()
	if !ok {
		return nil, domain.ErrNotFound
	}
	if vm.State != domain.VMStateRunning && vm.State != domain.VMStateBlocked && vm.State != domain.VMStatePaused {
		return nil, domain.ErrConsoleUnavailable
	}
	found := false
	for _, serial := range vm.Serials {
		if strings.TrimSpace(device) == "" || serial.Device == device {
			found = true
			break
		}
	}
	if !found {
		return nil, domain.ErrConsoleUnavailable
	}
	client, server := net.Pipe()
	go serveDemoSerial(server, vm.Name)
	return client, nil
}

func (d *DemoBackend) SendConsoleKey(_ context.Context, uuid string, keycodes []uint) error {
	if len(keycodes) == 0 {
		return domain.ErrInvalidInput
	}
	d.mu.RLock()
	vm, ok := d.vms[uuid]
	d.mu.RUnlock()
	if !ok {
		return domain.ErrNotFound
	}
	if vm.State != domain.VMStateRunning && vm.State != domain.VMStateBlocked && vm.State != domain.VMStatePaused {
		return domain.ErrActionNotAllowed
	}
	for _, graphics := range vm.Graphics {
		if strings.EqualFold(graphics.Type, "vnc") {
			return nil
		}
	}
	return domain.ErrConsoleUnavailable
}

func serveDemoSerial(conn net.Conn, vmName string) {
	defer conn.Close()
	if _, err := io.WriteString(conn, "\r\nVirtMgr demo serial console\r\n"+vmName+" login: "); err != nil {
		return
	}
	buffer := make([]byte, 512)
	line := make([]byte, 0, 128)
	for {
		count, err := conn.Read(buffer)
		if count > 0 {
			for _, char := range buffer[:count] {
				switch char {
				case '\r', '\n':
					_, _ = io.WriteString(conn, "\r\n[demo] "+string(line)+"\r\n"+vmName+" login: ")
					line = line[:0]
				case 8, 127:
					if len(line) > 0 {
						line = line[:len(line)-1]
						_, _ = io.WriteString(conn, "\b \b")
					}
				default:
					line = append(line, char)
					_, _ = conn.Write([]byte{char})
				}
			}
		}
		if err != nil {
			return
		}
	}
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

package libvirtbackend

import (
	"context"
	"errors"
	"fmt"
	"io"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func (c *Client) OpenConsole(ctx context.Context, uuid string, graphicsIndex uint) (io.ReadWriteCloser, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return nil, err
	}
	dom, err := c.conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return nil, mapDomainLookupError(uuid, err)
	}
	defer func() { _ = dom.Free() }()
	file, err := dom.OpenGraphicsFD(graphicsIndex, 0)
	if err != nil {
		switch {
		case errors.Is(err, libvirt.ERR_OPERATION_INVALID), errors.Is(err, libvirt.ERR_OPERATION_DENIED):
			return nil, errors.Join(domain.ErrConsoleUnavailable, err)
		default:
			return nil, fmt.Errorf("open VNC graphics %d for domain %s: %w", graphicsIndex, uuid, err)
		}
	}
	return file, nil
}

func (c *Client) OpenSerialConsole(ctx context.Context, uuid, device string) (io.ReadWriteCloser, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return nil, err
	}
	dom, err := c.conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return nil, mapDomainLookupError(uuid, err)
	}
	defer func() { _ = dom.Free() }()
	startEventLoop()
	stream, err := c.conn.NewStream(0)
	if err != nil {
		return nil, fmt.Errorf("create libvirt console stream for domain %s: %w", uuid, err)
	}
	if err := dom.OpenConsole(device, stream, 0); err != nil {
		_ = stream.Free()
		switch {
		case errors.Is(err, libvirt.ERR_OPERATION_INVALID), errors.Is(err, libvirt.ERR_OPERATION_DENIED):
			return nil, errors.Join(domain.ErrConsoleUnavailable, err)
		default:
			return nil, fmt.Errorf("open serial console %q for domain %s: %w", device, uuid, err)
		}
	}
	return newDomainConsoleStream(stream), nil
}

func (c *Client) SendConsoleKey(ctx context.Context, uuid string, keycodes []uint) error {
	if len(keycodes) == 0 {
		return domain.ErrInvalidInput
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return err
	}
	dom, err := c.conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return mapDomainLookupError(uuid, err)
	}
	defer func() { _ = dom.Free() }()
	if err := dom.SendKey(uint(libvirt.KEYCODE_SET_LINUX), 0, keycodes, 0); err != nil {
		switch {
		case errors.Is(err, libvirt.ERR_OPERATION_INVALID), errors.Is(err, libvirt.ERR_OPERATION_DENIED):
			return errors.Join(domain.ErrActionNotAllowed, err)
		default:
			return fmt.Errorf("send console keys to domain %s: %w", uuid, err)
		}
	}
	return nil
}

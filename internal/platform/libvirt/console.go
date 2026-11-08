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

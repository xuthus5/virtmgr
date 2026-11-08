package libvirtbackend

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

type snapshotXML struct {
	Name         string `xml:"name"`
	Description  string `xml:"description"`
	State        string `xml:"state"`
	CreationTime int64  `xml:"creationTime"`
	Parent       struct {
		Name string `xml:"name"`
	} `xml:"parent"`
}

func (c *Client) ListSnapshots(ctx context.Context, uuid string) ([]domain.VMSnapshot, error) {
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
	snapshots, err := dom.ListAllSnapshots(libvirt.DOMAIN_SNAPSHOT_LIST_DESCENDANTS)
	if err != nil {
		return nil, fmt.Errorf("list snapshots for domain %s: %w", uuid, err)
	}
	result := make([]domain.VMSnapshot, 0, len(snapshots))
	for i := range snapshots {
		snapshot := &snapshots[i]
		item, err := snapshotFromLibvirt(snapshot)
		_ = snapshot.Free()
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (c *Client) CreateSnapshot(ctx context.Context, uuid string, spec domain.VMSnapshotCreateSpec) (domain.VMSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.VMSnapshot{}, err
	}
	dom, err := c.conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return domain.VMSnapshot{}, mapDomainLookupError(uuid, err)
	}
	defer func() { _ = dom.Free() }()
	if existing, err := dom.SnapshotLookupByName(spec.Name, 0); err == nil {
		_ = existing.Free()
		return domain.VMSnapshot{}, domain.ErrConflict
	} else if !errors.Is(err, libvirt.ERR_NO_DOMAIN_SNAPSHOT) {
		return domain.VMSnapshot{}, fmt.Errorf("check snapshot %s on domain %s: %w", spec.Name, uuid, err)
	}
	snapshot, err := dom.CreateSnapshotXML(buildSnapshotXML(spec), 0)
	if err != nil {
		return domain.VMSnapshot{}, mapSnapshotOperationError("create", uuid, spec.Name, err)
	}
	defer func() { _ = snapshot.Free() }()
	return snapshotFromLibvirt(snapshot)
}

func (c *Client) RevertSnapshot(ctx context.Context, uuid, name string) error {
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
	snapshot, err := dom.SnapshotLookupByName(name, 0)
	if err != nil {
		return mapSnapshotLookupError(uuid, name, err)
	}
	defer func() { _ = snapshot.Free() }()
	if err := snapshot.RevertToSnapshot(0); err != nil {
		return mapSnapshotOperationError("revert", uuid, name, err)
	}
	return nil
}

func (c *Client) DeleteSnapshot(ctx context.Context, uuid, name string) error {
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
	snapshot, err := dom.SnapshotLookupByName(name, 0)
	if err != nil {
		return mapSnapshotLookupError(uuid, name, err)
	}
	defer func() { _ = snapshot.Free() }()
	if err := snapshot.Delete(0); err != nil {
		return mapSnapshotOperationError("delete", uuid, name, err)
	}
	return nil
}

func snapshotFromLibvirt(snapshot *libvirt.DomainSnapshot) (domain.VMSnapshot, error) {
	name, err := snapshot.GetName()
	if err != nil {
		return domain.VMSnapshot{}, fmt.Errorf("read snapshot name: %w", err)
	}
	xmlText, err := snapshot.GetXMLDesc(0)
	if err != nil {
		return domain.VMSnapshot{}, fmt.Errorf("read snapshot %s xml: %w", name, err)
	}
	var parsed snapshotXML
	if err := xml.Unmarshal([]byte(xmlText), &parsed); err != nil {
		return domain.VMSnapshot{}, fmt.Errorf("decode snapshot %s xml: %w", name, err)
	}
	current, err := snapshot.IsCurrent(0)
	if err != nil {
		return domain.VMSnapshot{}, fmt.Errorf("read snapshot %s current state: %w", name, err)
	}
	return domain.VMSnapshot{
		Name:        firstNonEmpty(parsed.Name, name),
		Description: parsed.Description,
		CreatedAt:   parsed.CreationTime,
		State:       mapSnapshotState(parsed.State),
		Current:     current,
		Parent:      parsed.Parent.Name,
	}, nil
}

func buildSnapshotXML(spec domain.VMSnapshotCreateSpec) string {
	var builder strings.Builder
	builder.WriteString("<domainsnapshot>")
	fmt.Fprintf(&builder, "<name>%s</name>", escapeXML(spec.Name))
	if spec.Description != "" {
		fmt.Fprintf(&builder, "<description>%s</description>", escapeXML(spec.Description))
	}
	builder.WriteString("</domainsnapshot>")
	return builder.String()
}

func mapSnapshotState(value string) domain.VMState {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "running":
		return domain.VMStateRunning
	case "blocked":
		return domain.VMStateBlocked
	case "paused":
		return domain.VMStatePaused
	case "shutdown":
		return domain.VMStateShutdown
	case "crashed":
		return domain.VMStateCrashed
	case "pmsuspended":
		return domain.VMStatePMSuspended
	case "shutoff":
		return domain.VMStateShutoff
	default:
		return domain.VMStateUnknown
	}
}

func mapSnapshotLookupError(uuid, name string, err error) error {
	if errors.Is(err, libvirt.ERR_NO_DOMAIN_SNAPSHOT) {
		return domain.ErrNotFound
	}
	return fmt.Errorf("lookup snapshot %s on domain %s: %w", name, uuid, err)
}

func mapSnapshotOperationError(action, uuid, name string, err error) error {
	switch {
	case errors.Is(err, libvirt.ERR_NO_DOMAIN_SNAPSHOT):
		return domain.ErrNotFound
	case errors.Is(err, libvirt.ERR_SNAPSHOT_REVERT_RISKY):
		return fmt.Errorf("%w: %v", domain.ErrSnapshotRisky, err)
	case errors.Is(err, libvirt.ERR_OPERATION_INVALID):
		return fmt.Errorf("%w: snapshot %s on domain %s: %v", domain.ErrConflict, name, uuid, err)
	default:
		return fmt.Errorf("%s snapshot %s on domain %s: %w", action, name, uuid, err)
	}
}

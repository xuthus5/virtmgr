package libvirtbackend

import (
	"sync"
	"time"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

var eventLoopOnce sync.Once

func startEventLoop() {
	eventLoopOnce.Do(func() {
		_ = libvirt.EventRegisterDefaultImpl()
		go func() {
			for {
				if err := libvirt.EventRunDefaultImpl(); err != nil {
					time.Sleep(time.Second)
				}
			}
		}()
	})
}

func (c *Client) handleDomainLifecycle(_ *libvirt.Connect, dom *libvirt.Domain, lifecycle *libvirt.DomainEventLifecycle) {
	if c.events == nil || lifecycle == nil {
		return
	}
	name, _ := dom.GetName()
	uuid, _ := dom.GetUUIDString()
	kind, severity, message := mapDomainLifecycleEvent(lifecycle)
	c.events.Publish(domain.SystemEvent{
		Source:     "libvirt",
		Kind:       kind,
		Severity:   severity,
		TargetKind: "vm",
		TargetID:   uuid,
		TargetName: name,
		Message:    message,
		Detail: map[string]any{
			"eventType": int(lifecycle.Event),
			"detail":    lifecycle.Detail,
		},
	})
}

func mapDomainLifecycleEvent(lifecycle *libvirt.DomainEventLifecycle) (string, string, string) {
	switch lifecycle.Event {
	case libvirt.DOMAIN_EVENT_DEFINED:
		if libvirt.DomainEventDefinedDetailType(lifecycle.Detail) == libvirt.DOMAIN_EVENT_DEFINED_UPDATED {
			return "domain.updated", "info", "虚拟机配置已更新"
		}
		return "domain.defined", "info", "虚拟机已定义"
	case libvirt.DOMAIN_EVENT_UNDEFINED:
		return "domain.undefined", "warning", "虚拟机定义已删除"
	case libvirt.DOMAIN_EVENT_STARTED:
		return "domain.started", "info", "虚拟机已启动"
	case libvirt.DOMAIN_EVENT_SUSPENDED:
		return "domain.suspended", "warning", "虚拟机已暂停"
	case libvirt.DOMAIN_EVENT_RESUMED:
		return "domain.resumed", "info", "虚拟机已恢复运行"
	case libvirt.DOMAIN_EVENT_STOPPED:
		if libvirt.DomainEventStoppedDetailType(lifecycle.Detail) == libvirt.DOMAIN_EVENT_STOPPED_CRASHED {
			return "domain.crashed", "critical", "虚拟机异常停止"
		}
		return "domain.stopped", "warning", "虚拟机已停止"
	case libvirt.DOMAIN_EVENT_SHUTDOWN:
		return "domain.shutdown", "info", "虚拟机正在关机"
	case libvirt.DOMAIN_EVENT_CRASHED:
		return "domain.crashed", "critical", "虚拟机发生崩溃"
	case libvirt.DOMAIN_EVENT_PMSUSPENDED:
		return "domain.pmsuspended", "info", "虚拟机已进入电源休眠"
	default:
		return "domain.changed", "info", "虚拟机状态发生变化"
	}
}

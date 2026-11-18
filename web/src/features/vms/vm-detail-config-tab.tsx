import type { Role, VM } from "@/api/types";
import { DetailItem } from "@/components/data/detail-item";
import { StatusBadge } from "@/components/feedback/status-badge";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { VMDisksCard } from "@/features/vms/vm-disks-card";
import { VMMetadataCard } from "@/features/vms/vm-metadata-card";
import { VMNetworkInterfacesCard } from "@/features/vms/vm-network-interfaces-card";
import { VMPassthroughCard } from "@/features/vms/vm-passthrough-card";
import { VMResourcesCard } from "@/features/vms/vm-resources-card";
import { cn } from "@/lib/utils";

export function VMDetailConfigTab({ vm, role, readOnly, canMutate, resourcesDisabledReason, disabledReason, autostartNotice, autostartDisabledReason, autostartPending, onToggleAutostart }: {
  vm: VM;
  role: Role | undefined;
  readOnly: boolean;
  canMutate: boolean;
  resourcesDisabledReason: string;
  disabledReason: string;
  autostartNotice: { tone: "success" | "error"; text: string } | null;
  autostartDisabledReason: string;
  autostartPending: boolean;
  onToggleAutostart: () => void;
}) {
  return <>
    <div role="tabpanel" id="vm-tabpanel-config" aria-labelledby="vm-tab-config" data-testid="vm-tabpanel-config">
      <VMResourcesCard vm={vm} canMutate={canMutate && !resourcesDisabledReason} disabledReason={resourcesDisabledReason} />
      <VMMetadataCard vm={vm} canMutate={canMutate} disabledReason={disabledReason} />
    </div>

    <div className="mt-4">
      <Card data-testid="vm-basic-info">
        <CardHeader>
          <div className="flex items-center justify-between gap-3">
            <div><CardTitle>基础信息</CardTitle><CardDescription>来自 libvirt 域定义与实时状态。</CardDescription></div>
            <StatusBadge state={vm.state} />
          </div>
        </CardHeader>
        <CardContent>
          <DetailItem label="名称">{vm.name}</DetailItem>
          <DetailItem label="UUID"><span className="font-mono text-xs">{vm.uuid}</span></DetailItem>
          <DetailItem label="状态码">{vm.state} / {vm.stateReason}</DetailItem>
          <DetailItem label="操作系统">{vm.osType || "-"}</DetailItem>
          <DetailItem label="架构">{vm.arch || "-"}</DetailItem>
          <DetailItem label="机型">{vm.machine || "-"}</DetailItem>
          <DetailItem label="固件"><span data-testid="vm-firmware">{vm.firmware === "uefi" ? "UEFI" : "BIOS"}</span></DetailItem>
          <DetailItem label="安全启动">
            <Badge variant={vm.secureBoot ? "success" : "secondary"} data-testid="vm-secure-boot">{vm.secureBoot ? "已启用" : "未启用"}</Badge>
          </DetailItem>
          <DetailItem label="Cloud-init">
            <Badge variant={vm.cloudInit ? "success" : "secondary"} data-testid="vm-cloud-init">{vm.cloudInit ? "已配置" : "未配置"}</Badge>
          </DetailItem>
          <DetailItem label="持久化">{vm.persistent ? "是" : "否"}</DetailItem>
          <DetailItem label="自动启动">{vm.autostart ? "是" : "否"}</DetailItem>
          <div className="mt-3 flex items-center justify-between gap-4 rounded-md border bg-muted/20 px-3 py-3">
            <div>
              <p className="text-sm font-medium">开机自启动</p>
              <p className="mt-0.5 text-xs text-muted-foreground">{vm.autostart ? "宿主机启动 libvirt 后自动启动此虚拟机。" : "虚拟机不会随宿主机或 libvirt 自动启动。"}</p>
              {autostartNotice ? (
                <p className={cn("mt-2 text-xs", autostartNotice.tone === "success" ? "text-success" : "text-destructive")} data-testid="vm-autostart-notice" role="status">{autostartNotice.text}</p>
              ) : null}
            </div>
            <button
              type="button"
              role="switch"
              aria-checked={vm.autostart}
              aria-label="开机自启动"
              data-testid="vm-autostart-toggle"
              disabled={Boolean(autostartDisabledReason)}
              title={autostartDisabledReason || (vm.autostart ? "禁用开机自启动" : "启用开机自启动")}
              onClick={onToggleAutostart}
              className={cn(
                "relative inline-flex h-6 w-11 shrink-0 items-center rounded-full border transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
                vm.autostart ? "border-primary bg-primary" : "border-input bg-muted",
              )}
            >
              <span className={cn("pointer-events-none block h-4 w-4 rounded-full bg-background shadow-sm transition-transform", autostartPending ? "animate-pulse" : "", vm.autostart ? "translate-x-[1.35rem]" : "translate-x-1")} />
            </button>
          </div>
          {autostartDisabledReason ? <p className="mt-2 text-xs text-muted-foreground">{autostartDisabledReason}。</p> : null}
        </CardContent>
      </Card>
    </div>

    <VMDisksCard vm={vm} role={role} readOnly={readOnly} />
    <VMNetworkInterfacesCard vm={vm} canMutate={canMutate} disabledReason={disabledReason} />
    <VMPassthroughCard vm={vm} canMutate={canMutate} disabledReason={disabledReason} />
  </>;
}

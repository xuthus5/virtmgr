import { Activity, GitBranch, Info, ListPlus, LoaderCircle, Network, Pencil, Play, Router, ScrollText, Square, Trash2 } from "lucide-react";
import type { Role, VirtualNetwork, VirtualNetworkAction } from "@/api/types";
import { MetricCard } from "@/components/data/metric-card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { networkActionHint, networkEditDisabledReason } from "@/features/networks/network-edit-model";
import { addressLabel, staticHostCount } from "@/features/networks/network-ui";

export function NetworkTableCard({ networks, total, active, bridges, nat, role, readOnly, canManage, editTargetName, leasesNetwork, hostsNetwork, actionPendingName, actionPendingAction, updatePending, deletePending, onEdit, onAction, onDelete, onLeases, onHosts }: {
  networks: VirtualNetwork[];
  total: number;
  active: number;
  bridges: number;
  nat: number;
  role: Role | undefined;
  readOnly: boolean;
  canManage: boolean;
  editTargetName?: string;
  leasesNetwork: string | null;
  hostsNetwork: string | null;
  actionPendingName?: string;
  actionPendingAction?: VirtualNetworkAction;
  updatePending: boolean;
  deletePending: boolean;
  onEdit: (network: VirtualNetwork) => void;
  onAction: (network: VirtualNetwork, action: VirtualNetworkAction) => void;
  onDelete: (network: VirtualNetwork) => void;
  onLeases: (network: VirtualNetwork) => void;
  onHosts: (network: VirtualNetwork) => void;
}) {
  return <>
    <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <MetricCard label="虚拟网络" value={total} hint="已定义的全部网络" icon={Network} tone="primary" testId="network-total" />
      <MetricCard label="活跃网络" value={active} hint={`${networks.length - active} 个未激活`} icon={GitBranch} tone={active ? "success" : "neutral"} testId="network-active" />
      <MetricCard label="外部桥接" value={bridges} hint="接入宿主机已有网桥" icon={Router} tone="neutral" testId="network-bridges" />
      <MetricCard label="NAT 网络" value={nat} hint="宿主转发模式为 NAT" icon={Activity} tone={nat ? "warning" : "neutral"} testId="network-nat" />
    </div>

    <Card className="mt-4">
      <CardHeader>
        <CardTitle>网络列表</CardTitle>
        <CardDescription>地址、DHCP 范围、桥接口和自动启动状态来自 libvirt XML。</CardDescription>
      </CardHeader>
      <CardContent className="p-0">
        {networks.length ? (
          <Table className="min-w-[1080px]">
            <TableHeader><TableRow><TableHead>名称</TableHead><TableHead>模式</TableHead><TableHead>状态</TableHead><TableHead>桥接</TableHead><TableHead>地址 / DHCP</TableHead><TableHead>自动启动</TableHead><TableHead>操作</TableHead></TableRow></TableHeader>
            <TableBody>
              {networks.map((network) => {
                const hint = networkActionHint(role, readOnly, network);
                const editReason = networkEditDisabledReason(role, readOnly, network);
                const hintID = `network-action-hint-${network.name}`;
                const actionInFlight = actionPendingName !== undefined;
                return (
                  <TableRow key={network.uuid || network.name} data-testid={`network-row-${network.name}`}>
                    <TableCell>
                      <p className="font-medium">{network.name}</p>
                      <p className="font-mono text-[10px] text-muted-foreground">{network.uuid || "-"}</p>
                    </TableCell>
                    <TableCell><Badge variant="outline">{network.forwardMode || "isolated"}</Badge></TableCell>
                    <TableCell><Badge variant={network.active ? "success" : "secondary"}>{network.active ? "活跃" : "未激活"}</Badge></TableCell>
                    <TableCell><span className="font-mono text-xs">{network.bridge || "-"}</span></TableCell>
                    <TableCell>
                      {network.addresses.length ? network.addresses.map((address) => (
                        <div key={`${address.family}-${address.address}`} className="py-0.5">
                          <p className="font-mono text-xs">{addressLabel(address)}</p>
                          {address.dhcpStart || address.dhcpEnd ? (
                            <p className="font-mono text-[10px] text-muted-foreground">DHCP {address.dhcpStart || "-"} - {address.dhcpEnd || "-"}</p>
                          ) : null}
                          {address.hosts?.length ? (
                            <p className="text-[10px] text-muted-foreground">静态主机 {address.hosts.length} 条</p>
                          ) : null}
                        </div>
                      )) : <span className="text-xs text-muted-foreground">{network.forwardMode === "bridge" ? "由上游网桥分配" : "未配置地址"}</span>}
                    </TableCell>
                    <TableCell>{network.autostart ? "是" : "否"}</TableCell>
                    <TableCell className="align-top">
                      <div className="flex min-w-[17rem] flex-col gap-2">
                        <div className="flex flex-wrap items-center gap-1.5">
                          {canManage ? (
                            <>
                              <Button type="button" variant={editTargetName === network.name ? "secondary" : "outline"} size="sm" data-testid={`network-edit-${network.name}`} aria-label={`编辑网络 ${network.name}`} aria-describedby={editReason ? hintID : undefined} title={editReason || "编辑网络配置"} disabled={Boolean(editReason) || updatePending} onClick={() => onEdit(network)}>
                                <Pencil className="h-3.5 w-3.5" />编辑
                              </Button>
                              {network.active ? (
                                <Button type="button" variant="outline" size="sm" data-testid={`network-stop-${network.name}`} aria-label={`停止网络 ${network.name}`} aria-describedby={hint ? hintID : undefined} title="停止网络" disabled={actionInFlight} onClick={() => onAction(network, "stop")}>
                                  {actionPendingName === network.name && actionPendingAction === "stop" ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" /> : <Square className="h-3.5 w-3.5" />}停止
                                </Button>
                              ) : (
                                <Button type="button" variant="outline" size="sm" data-testid={`network-start-${network.name}`} aria-label={`启动网络 ${network.name}`} title="启动网络" disabled={actionInFlight} onClick={() => onAction(network, "start")}>
                                  {actionPendingName === network.name && actionPendingAction === "start" ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}启动
                                </Button>
                              )}
                              <Button type="button" variant="ghost" size="sm" className="text-destructive hover:bg-destructive/10 hover:text-destructive" data-testid={`network-delete-${network.name}`} aria-label={`删除网络 ${network.name}`} aria-describedby={network.active ? hintID : undefined} title={network.active ? "请先停止网络再删除" : "删除网络定义"} disabled={network.active || deletePending} onClick={() => onDelete(network)}>
                                <Trash2 className="h-3.5 w-3.5" />删除
                              </Button>
                            </>
                          ) : null}
                          <Button type="button" variant={leasesNetwork === network.name ? "secondary" : "outline"} size="sm" data-testid={`network-leases-${network.name}`} aria-label={`查看网络 ${network.name} 的 DHCP 租约`} title={network.forwardMode === "bridge" ? "外部桥接网络的 DHCP 由宿主机上游网络管理" : "查看 DHCP 租约"} disabled={network.forwardMode === "bridge"} onClick={() => onLeases(network)}>
                            <ScrollText className="h-3.5 w-3.5" />租约
                          </Button>
                          <Button type="button" variant={hostsNetwork === network.name ? "secondary" : "outline"} size="sm" data-testid={`network-hosts-${network.name}`} aria-label={`管理网络 ${network.name} 的静态主机`} title={network.forwardMode === "bridge" ? "外部桥接网络不提供 libvirt DHCP 静态主机配置" : staticHostCount(network) ? `管理静态主机（${staticHostCount(network)} 条）` : "管理静态主机条目"} disabled={network.forwardMode === "bridge"} onClick={() => onHosts(network)}>
                            <ListPlus className="h-3.5 w-3.5" />静态主机
                          </Button>
                        </div>
                        {hint ? (
                          <p id={hintID} data-testid={`network-action-hint-${network.name}`} className="flex items-start gap-1.5 text-[11px] leading-relaxed text-muted-foreground">
                            <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" />{hint}
                          </p>
                        ) : null}
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        ) : <p className="p-5 text-sm text-muted-foreground">当前宿主机没有定义虚拟网络。</p>}
      </CardContent>
    </Card>
  </>;
}

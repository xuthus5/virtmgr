import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Clock,
  HardDrive,
  Info,
  LoaderCircle,
  Maximize2,
  Network,
  Power,
  RefreshCw,
  RotateCcw,
  Scissors,
  Stethoscope,
} from "lucide-react";
import { ApiError, api } from "@/api/client";
import type {
  GuestAgentAction,
  GuestAgentActionInput,
  GuestAgentFilesystem,
  GuestAgentInfo,
  GuestAgentState,
  VM,
} from "@/api/types";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Modal } from "@/components/ui/modal";
import { guestAgentQueryKey, useGuestAgentQuery } from "@/features/guest-agent/use-guest-agent-query";
import { guestAgentActionUnavailableReason, guestAgentCanAddChannel } from "@/features/guest-agent/guest-agent-model";
import { GuestAgentControls } from "@/features/guest-agent/guest-agent-controls";
import { vmQueryKey } from "@/features/vms/use-vm-query";
import { cn, formatBytes, percent } from "@/lib/utils";

const stateMeta: Record<GuestAgentState, { label: string; variant: "success" | "warning" | "destructive" | "secondary" }> = {
  connected: { label: "已连接", variant: "success" },
  not_configured: { label: "未配置通道", variant: "secondary" },
  disconnected: { label: "未连接", variant: "warning" },
  unresponsive: { label: "无响应", variant: "destructive" },
  error: { label: "探测异常", variant: "destructive" },
  unknown: { label: "状态未知", variant: "secondary" },
};

const actionMeta: Record<GuestAgentAction, { label: string; icon: typeof Power; destructive?: boolean; confirm?: boolean }> = {
  shutdown: { label: "由代理关机", icon: Power, confirm: true },
  reboot: { label: "由代理重启", icon: RotateCcw, confirm: true },
  "sync-time": { label: "同步时间", icon: Clock },
  trim: { label: "执行 TRIM", icon: Scissors, confirm: true },
};

const dateTimeFormatter = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});

function formatDateTime(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "-" : dateTimeFormatter.format(date);
}

function capabilityBadges(info: GuestAgentInfo) {
  const capabilities = [
    ["主机名", info.capabilities.hostname],
    ["网络", info.capabilities.network],
    ["系统信息", info.capabilities.osInfo],
    ["文件系统", info.capabilities.fsInfo],
    ["关机", info.capabilities.shutdown],
    ["重启", info.capabilities.reboot],
    ["时间同步", info.capabilities.timeSync],
    ["TRIM", info.capabilities.trim],
    ["静默快照", info.capabilities.quiesce],
    ["文件读写", info.capabilities.fileAccess],
    ["密码重置", info.capabilities.passwordReset],
    ["SSH Key", info.capabilities.sshKeys],
    ["进程管理", info.capabilities.processes],
    ["登录用户", info.capabilities.users],
    ["时区/路由", info.capabilities.timezone || info.capabilities.networkRoutes],
  ] as const;
  return capabilities.map(([label, enabled]) => (
    <Badge key={label} variant={enabled ? "success" : "secondary"}>{label}</Badge>
  ));
}

export function VMGuestAgentCard({ vm, canMutate, disabledReason }: {
  vm: VM;
  canMutate: boolean;
  disabledReason: string;
}) {
  const queryClient = useQueryClient();
  const query = useGuestAgentQuery(vm.uuid);
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [confirmAction, setConfirmAction] = useState<GuestAgentAction | null>(null);
  const [trimMount, setTrimMount] = useState("/");
  const [trimMinimumBytes, setTrimMinimumBytes] = useState("0");
  const [growFilesystem, setGrowFilesystem] = useState<GuestAgentFilesystem | null>(null);
  const [diagnosticsOpen, setDiagnosticsOpen] = useState(false);

  const refreshMutation = useMutation({
    mutationFn: () => api.guestAgent(vm.uuid, true),
    onSuccess: (info) => {
      queryClient.setQueryData(guestAgentQueryKey(vm.uuid), info);
      setNotice({ tone: "success", text: "Guest Agent 状态已刷新。" });
    },
    onError: (cause) => {
      setNotice({ tone: "error", text: cause instanceof ApiError ? cause.message : "刷新 Guest Agent 状态失败。" });
    },
  });

  const actionMutation = useMutation({
    mutationFn: (input: GuestAgentActionInput) => api.performGuestAgentAction(vm.uuid, input),
    onSuccess: (_result, input) => {
      setConfirmAction(null);
      setNotice({ tone: "success", text: `已提交“${actionMeta[input.action].label}”请求。` });
      void queryClient.invalidateQueries({ queryKey: guestAgentQueryKey(vm.uuid) });
      void queryClient.invalidateQueries({ queryKey: vmQueryKey(vm.uuid) });
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
    },
    onError: (cause) => {
      setConfirmAction(null);
      setNotice({ tone: "error", text: cause instanceof ApiError ? cause.message : "Guest Agent 操作失败。" });
    },
  });

  const channelMutation = useMutation({
    mutationFn: () => api.ensureGuestAgentChannel(vm.uuid),
    onSuccess: (updated) => {
      queryClient.setQueryData(vmQueryKey(vm.uuid), updated);
      setNotice({ tone: "success", text: "已补充 QEMU Guest Agent 通道，启动虚拟机后即可探测代理。" });
      void queryClient.invalidateQueries({ queryKey: guestAgentQueryKey(vm.uuid) });
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
    },
    onError: (cause) => {
      setNotice({ tone: "error", text: cause instanceof ApiError ? cause.message : "补充 Guest Agent 通道失败。" });
    },
  });

  const growMutation = useMutation({
    mutationFn: (mountPoint: string) => api.growGuestAgentFilesystem(vm.uuid, { mountPoint }),
    onSuccess: (response) => {
      setGrowFilesystem(null);
      setNotice({ tone: "success", text: `${response.result.mountPoint} 已扩展至 ${formatBytes(response.result.totalBytes)}，可用 ${formatBytes(response.result.freeBytes)}。` });
      void queryClient.invalidateQueries({ queryKey: guestAgentQueryKey(vm.uuid) });
    },
    onError: (cause) => {
      setNotice({ tone: "error", text: cause instanceof ApiError ? cause.message : "客户机文件系统扩容失败。" });
    },
  });

  function runAction(action: GuestAgentAction) {
    const meta = actionMeta[action];
    if (meta.confirm) {
      setConfirmAction(action);
      return;
    }
    setNotice(null);
    actionMutation.mutate({ action });
  }

  const info = query.data;
  const state = info ? stateMeta[info.state] : null;
  const canAddChannel = guestAgentCanAddChannel(vm.state);
  const diagnosticsQuery = useQuery({
    queryKey: ["vm", vm.uuid, "guest-agent", "diagnostics"],
    queryFn: () => api.guestAgentDiagnostics(vm.uuid),
    enabled: diagnosticsOpen && info?.state === "connected" && info.capabilities.processes,
    staleTime: 0,
    retry: 0,
  });

  return (
    <Card className="mt-4 scroll-mt-20" id="vm-guest-agent-card" data-testid="vm-guest-agent-card">
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2"><Activity className="h-4 w-4 text-primary" />Guest Agent</CardTitle>
            <CardDescription>通过 QEMU Guest Agent 读取客户机信息，并执行受控的关机、重启、时间同步与 TRIM。</CardDescription>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            data-testid="guest-agent-refresh"
            disabled={query.isFetching || refreshMutation.isPending}
            onClick={() => { setNotice(null); refreshMutation.mutate(); }}
          >
            <RefreshCw className={cn("h-4 w-4", (query.isFetching || refreshMutation.isPending) && "animate-spin")} />刷新
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {query.isLoading ? <LoadingState label="正在探测 Guest Agent" /> : null}
        {query.isError ? <ErrorState message={query.error instanceof Error ? query.error.message : "无法读取 Guest Agent 状态"} onRetry={() => void query.refetch()} /> : null}
        {info && state ? (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant={state.variant} data-testid="guest-agent-state">{state.label}</Badge>
              <Badge variant={info.channelConfigured ? "outline" : "secondary"}>{info.channelConfigured ? "QGA 通道已配置" : "QGA 通道未配置"}</Badge>
              {info.stale ? <Badge variant="warning">显示上次成功状态</Badge> : null}
              {info.state === "connected" && info.version ? <span className="text-xs text-muted-foreground">qemu-guest-agent {info.version}</span> : null}
            </div>

            {info.state === "connected" ? (
              <div className="grid gap-3 text-sm sm:grid-cols-2 xl:grid-cols-4">
                <div className="rounded-md border bg-muted/20 p-3"><p className="text-xs text-muted-foreground">主机名</p><p className="mt-1 truncate font-mono text-xs" data-testid="guest-agent-hostname">{info.hostname || vm.hostname || "-"}</p></div>
                <div className="rounded-md border bg-muted/20 p-3"><p className="text-xs text-muted-foreground">操作系统</p><p className="mt-1 truncate" title={info.os.prettyName || info.os.name}>{info.os.prettyName || info.os.name || info.os.id || "-"}</p></div>
                <div className="rounded-md border bg-muted/20 p-3"><p className="text-xs text-muted-foreground">内核 / 架构</p><p className="mt-1 truncate font-mono text-xs">{info.os.kernelRelease || "-"} · {info.os.arch || "-"}</p></div>
                <div className="rounded-md border bg-muted/20 p-3"><p className="text-xs text-muted-foreground">最近响应</p><p className="mt-1">{formatDateTime(info.lastSeenAt)}</p></div>
              </div>
            ) : null}

            {info.lastError ? (
              <div className="flex items-start gap-2 rounded-md border border-warning/35 bg-warning/10 px-3 py-2 text-xs text-warning" data-testid="guest-agent-error">
                <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" /><span className="break-all">{info.lastError}{info.diagnosticCode ? ` [${info.diagnosticCode}]` : ""}{info.diagnosticHint ? ` ${info.diagnosticHint}` : ""}</span>
              </div>
            ) : null}

            {info.state !== "connected" ? (
              <div className="rounded-md border border-dashed px-4 py-4 text-sm">
                <p className="font-medium">连接引导</p>
                <p className="mt-1 text-xs text-muted-foreground">
                  Guest Agent 需要在客户机内安装并启动。新建虚拟机默认会挂载 virtio-serial 通道；已有虚拟机可在关机状态下补充通道。
                </p>
                <div className="mt-3 flex flex-wrap items-center gap-2">
                  {!info.channelConfigured ? (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      data-testid="guest-agent-add-channel"
                      disabled={!canMutate || !canAddChannel || channelMutation.isPending}
                      title={!canMutate ? disabledReason : !canAddChannel ? "请先关闭虚拟机" : "补充 QEMU Guest Agent 通道"}
                      onClick={() => { setNotice(null); channelMutation.mutate(); }}
                    >
                      {channelMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : null}补充 QGA 通道
                    </Button>
                  ) : null}
                  {!info.channelConfigured && !canAddChannel ? <span className="text-xs text-muted-foreground">关机后可补充通道。</span> : null}
                </div>
                <details className="mt-3 rounded-md border bg-muted/20 px-3 py-2 text-xs" data-testid="guest-agent-install-help">
                  <summary className="cursor-pointer font-medium">客户机安装命令</summary>
                  <div className="mt-3 space-y-2 text-muted-foreground">
                    <p>Debian / Ubuntu：<code className="ml-1 rounded bg-background px-1.5 py-0.5 font-mono text-foreground">sudo apt install qemu-guest-agent && sudo systemctl enable --now qemu-guest-agent</code></p>
                    <p>RHEL / Fedora：<code className="ml-1 rounded bg-background px-1.5 py-0.5 font-mono text-foreground">sudo dnf install qemu-guest-agent && sudo systemctl enable --now qemu-guest-agent</code></p>
                    <p>Windows：通过 virtio-win ISO 安装 <span className="font-mono text-foreground">guest-agent\qemu-ga-x86_64.msi</span>，并确认 QEMU Guest Agent 服务已启动。</p>
                  </div>
                </details>
              </div>
            ) : null}

            {info.state === "connected" ? (
              <>
                <div>
                  <p className="mb-2 text-xs font-medium text-muted-foreground">可用能力</p>
                  <div className="flex flex-wrap gap-1.5" data-testid="guest-agent-capabilities">{capabilityBadges(info)}</div>
                </div>

                {info.timezone || info.routes.length ? (
                  <div className="grid gap-3 text-xs sm:grid-cols-2">
                    <div className="rounded-md border bg-muted/20 p-3"><p className="text-muted-foreground">客户机时区</p><p className="mt-1 font-mono">{info.timezone || "-"}</p></div>
                    <div className="rounded-md border bg-muted/20 p-3"><p className="text-muted-foreground">路由</p>{info.routes.length ? info.routes.slice(0, 6).map((route) => <p key={`${route.family}-${route.destination}-${route.gateway}`} className="mt-1 truncate font-mono">{route.destination || "-"}{route.gateway ? ` via ${route.gateway}` : ""} · {route.interface || "-"}</p>) : <p className="mt-1 text-muted-foreground">未返回路由。</p>}</div>
                  </div>
                ) : null}

                <div className="grid gap-4 xl:grid-cols-2">
                  <div className="overflow-hidden rounded-md border">
                    <div className="flex items-center gap-2 border-b bg-muted/30 px-3 py-2 text-sm font-medium"><Network className="h-4 w-4" />网络接口</div>
                    {info.interfaces.length ? (
                      <div className="divide-y">
                        {info.interfaces.map((networkInterface) => (
                          <div key={`${networkInterface.name}-${networkInterface.mac}`} className="px-3 py-2 text-xs">
                            <div className="flex flex-wrap items-center gap-2"><span className="font-medium">{networkInterface.name || "未命名接口"}</span>{networkInterface.mac ? <span className="font-mono text-muted-foreground">{networkInterface.mac}</span> : null}</div>
                            <div className="mt-1 flex flex-wrap gap-1.5">
                              {networkInterface.addresses.length ? networkInterface.addresses.map((address) => (
                                <span key={`${address.family}-${address.address}`} className="rounded bg-muted px-1.5 py-0.5 font-mono">{address.address}{address.prefix ? `/${address.prefix}` : ""}</span>
                              )) : <span className="text-muted-foreground">无可用 IP 地址</span>}
                            </div>
                          </div>
                        ))}
                      </div>
                    ) : <p className="px-3 py-4 text-xs text-muted-foreground">代理未返回网络接口。</p>}
                  </div>

                  <div className="overflow-hidden rounded-md border">
                    <div className="flex items-center gap-2 border-b bg-muted/30 px-3 py-2 text-sm font-medium"><HardDrive className="h-4 w-4" />文件系统</div>
                    {info.filesystems.length ? (
                      <div className="divide-y">
                        {info.filesystems.map((filesystem) => (
                          <div key={`${filesystem.mountPoint}-${filesystem.device}`} className="px-3 py-2 text-xs">
                            <div className="flex flex-wrap items-center justify-between gap-2">
                              <span className="min-w-0 truncate font-mono font-medium">{filesystem.mountPoint || "-"}</span>
                              <div className="flex items-center gap-2">
                                <span className="text-muted-foreground">{filesystem.type || "-"}</span>
                                {filesystem.type === "ext4" || filesystem.type === "xfs" ? (
                                  <Button
                                    type="button"
                                    variant="outline"
                                    size="sm"
                                    className="h-7 px-2 text-[11px]"
                                    data-testid={`guest-filesystem-grow-${filesystem.mountPoint}`}
                                    disabled={!canMutate || !info.capabilities.filesystemResize || growMutation.isPending}
                                    title={!canMutate ? disabledReason : !info.capabilities.filesystemResize ? "当前 Guest Agent 不支持在线扩容" : `在线扩展 ${filesystem.mountPoint} 的分区和文件系统`}
                                    onClick={() => { setNotice(null); setGrowFilesystem(filesystem); }}
                                  >
                                    <Maximize2 className="h-3 w-3" />在线扩容
                                  </Button>
                                ) : null}
                              </div>
                            </div>
                            <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-muted"><div className="h-full bg-primary" style={{ width: `${percent(filesystem.usedBytes ?? 0, filesystem.totalBytes ?? 0)}%` }} /></div>
                            <p className="mt-1 text-muted-foreground">{formatBytes(filesystem.usedBytes ?? 0)} / {formatBytes(filesystem.totalBytes ?? 0)} · 可用 {formatBytes(filesystem.freeBytes ?? 0)}</p>
                          </div>
                        ))}
                      </div>
                    ) : <p className="px-3 py-4 text-xs text-muted-foreground">代理未返回文件系统信息。</p>}
                  </div>
                </div>

                <div className="rounded-md border" data-testid="guest-agent-diagnostics">
                  <button
                    type="button"
                    className="flex w-full items-center justify-between gap-3 bg-muted/20 px-3 py-2 text-left text-sm font-medium"
                    aria-expanded={diagnosticsOpen}
                    data-testid="guest-agent-diagnostics-toggle"
                    onClick={() => setDiagnosticsOpen((open) => !open)}
                  >
                    <span className="flex items-center gap-2"><Stethoscope className="h-4 w-4" />启动与故障诊断</span>
                    <span className="text-xs font-normal text-muted-foreground">{diagnosticsOpen ? "收起" : "检查失败服务、OOM 与系统日志"}</span>
                  </button>
                  {diagnosticsOpen ? (
                    <div className="space-y-3 border-t px-3 py-3 text-xs">
                      {!info.capabilities.processes ? <p className="text-muted-foreground">客户机 Guest Agent 未提供固定命令执行能力，无法进行启动诊断。</p> : null}
                      {diagnosticsQuery.isLoading ? <p className="flex items-center gap-2 text-muted-foreground"><LoaderCircle className="h-3.5 w-3.5 animate-spin" />正在执行固定诊断命令...</p> : null}
                      {diagnosticsQuery.isError ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-destructive">{diagnosticsQuery.error instanceof ApiError ? diagnosticsQuery.error.message : "读取诊断信息失败。"}</p> : null}
                      {diagnosticsQuery.data ? (
                        <>
                          <div className="flex flex-wrap items-center gap-2">
                            <Badge variant={diagnosticsQuery.data.failedServices.length || diagnosticsQuery.data.oomEvents.length ? "warning" : "success"}>
                              {diagnosticsQuery.data.failedServices.length ? `${diagnosticsQuery.data.failedServices.length} 个服务失败` : "无失败服务"}
                            </Badge>
                            <Badge variant={diagnosticsQuery.data.oomEvents.length ? "destructive" : "secondary"}>{diagnosticsQuery.data.oomEvents.length} 个 OOM 事件</Badge>
                            <span className="text-muted-foreground">检查于 {formatDateTime(diagnosticsQuery.data.checkedAt)}</span>
                          </div>
                          {diagnosticsQuery.data.failedServices.length ? (
                            <div><p className="mb-1 font-medium">失败服务</p><div className="flex flex-wrap gap-1.5">{diagnosticsQuery.data.failedServices.map((service) => <Badge key={service.unit} variant="outline" className="font-mono">{service.unit}</Badge>)}</div></div>
                          ) : null}
                          {diagnosticsQuery.data.serviceLogs.length ? (
                            <div className="space-y-2"><p className="font-medium">服务日志</p>{diagnosticsQuery.data.serviceLogs.map((log) => <div key={log.unit} className="rounded-md bg-muted/35 p-2"><p className="font-mono text-muted-foreground">{log.unit}{log.truncated ? " · 已截断" : ""}</p><pre className="mt-1 max-h-32 overflow-auto whitespace-pre-wrap break-words text-[11px]">{log.content || "-"}</pre></div>)}</div>
                          ) : null}
                          {diagnosticsQuery.data.kernelWarnings.length ? <DiagnosticLines title="内核警告" lines={diagnosticsQuery.data.kernelWarnings} /> : null}
                          {diagnosticsQuery.data.systemErrors.length ? <DiagnosticLines title="系统错误" lines={diagnosticsQuery.data.systemErrors} /> : null}
                          {diagnosticsQuery.data.oomEvents.length ? <DiagnosticLines title="OOM 事件" lines={diagnosticsQuery.data.oomEvents} /> : null}
                          {diagnosticsQuery.data.commandErrors.length ? <DiagnosticLines title="命令不可用" lines={diagnosticsQuery.data.commandErrors} /> : null}
                          <div><p className="mb-1 font-medium">建议</p><ul className="list-disc space-y-1 pl-4 text-muted-foreground">{diagnosticsQuery.data.recommendations.map((item) => <li key={item}>{item}</li>)}</ul></div>
                        </>
                      ) : null}
                    </div>
                  ) : null}
                </div>

                <div className="flex flex-wrap gap-2 border-t pt-4" data-testid="guest-agent-actions">
                  {(Object.keys(actionMeta) as GuestAgentAction[]).map((action) => {
                    const meta = actionMeta[action];
                    const Icon = meta.icon;
                    const reason = guestAgentActionUnavailableReason(info, action);
                    const disabled = !canMutate || Boolean(reason) || actionMutation.isPending;
                    return (
                      <Button
                        key={action}
                        type="button"
                        variant={meta.destructive ? "destructive" : "outline"}
                        size="sm"
                        data-testid={`guest-agent-action-${action}`}
                        disabled={disabled}
                        title={reason || (!canMutate ? disabledReason : meta.label)}
                        onClick={() => runAction(action)}
                      >
                        <Icon className={cn("h-4 w-4", actionMutation.isPending && actionMutation.variables?.action === action && "animate-spin")} />{meta.label}
                      </Button>
                    );
                  })}
                </div>

                <GuestAgentControls
                  uuid={vm.uuid}
                  canMutate={canMutate}
                  disabledReason={disabledReason}
                  notify={(tone, text) => setNotice({ tone, text })}
                />
              </>
            ) : null}
          </>
        ) : null}

        {notice ? (
          <p role="status" className={cn("flex items-start gap-2 rounded-md border px-3 py-2 text-xs", notice.tone === "success" ? "border-success/30 bg-success/10 text-success" : "border-destructive/30 bg-destructive/5 text-destructive")} data-testid="guest-agent-notice">
            {notice.tone === "success" ? <CheckCircle2 className="mt-0.5 h-3.5 w-3.5" /> : <AlertTriangle className="mt-0.5 h-3.5 w-3.5" />}{notice.text}
          </p>
        ) : null}

        <p className="flex items-start gap-2 text-xs text-muted-foreground"><Info className="mt-0.5 h-3.5 w-3.5 shrink-0" />Guest Agent 不提供任意命令执行；文件、密码、SSH Key 与进程操作均走固定接口并记录脱敏审计。</p>
      </CardContent>

      <AlertDialog open={confirmAction !== null} onOpenChange={(open) => { if (!open) setConfirmAction(null); }}>
        <AlertDialogContent data-testid="guest-agent-confirm-dialog">
          <AlertDialogHeader>
            <AlertDialogTitle>{confirmAction ? actionMeta[confirmAction].label : "确认操作"}？</AlertDialogTitle>
            <AlertDialogDescription>该操作会通过客户机内的 QEMU Guest Agent 执行。请确认客户机内的服务与工作负载已做好准备。</AlertDialogDescription>
            {confirmAction === "trim" ? (
              <div className="space-y-3 text-left">
                <div className="space-y-1"><Label htmlFor="guest-trim-mount">文件系统挂载点</Label><select id="guest-trim-mount" value={trimMount} onChange={(event) => setTrimMount(event.target.value)} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm"><option value="">全部支持的文件系统</option>{info?.filesystems.map((filesystem) => <option key={filesystem.mountPoint} value={filesystem.mountPoint}>{filesystem.mountPoint} ({filesystem.type || "unknown"})</option>)}</select></div>
                <div className="space-y-1"><Label htmlFor="guest-trim-minimum">最小回收字节数</Label><Input id="guest-trim-minimum" type="number" min={0} value={trimMinimumBytes} onChange={(event) => setTrimMinimumBytes(event.target.value)} /></div>
              </div>
            ) : null}
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={actionMutation.isPending}>取消</AlertDialogCancel>
            <Button
              data-testid="guest-agent-confirm-action"
              disabled={!confirmAction || actionMutation.isPending}
              onClick={() => { if (confirmAction) actionMutation.mutate({ action: confirmAction, mount: confirmAction === "trim" && trimMount ? trimMount : undefined, minimumBytes: confirmAction === "trim" ? Number(trimMinimumBytes) || undefined : undefined }); }}
            >
              {actionMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : null}确认执行
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Modal
        open={growFilesystem !== null}
        onClose={() => { if (!growMutation.isPending) setGrowFilesystem(null); }}
        title={`在线扩容 ${growFilesystem?.mountPoint || ""}`}
        description="宿主机磁盘扩容后，客户机内通过 growpart、resize2fs 或 xfs_growfs 扩展当前分区和文件系统。操作仅支持 ext4/xfs 分区块设备，不执行任意命令。"
        testId="guest-filesystem-grow-modal"
        footer={<><Button variant="ghost" disabled={growMutation.isPending} onClick={() => setGrowFilesystem(null)}>取消</Button><Button data-testid="guest-filesystem-grow-submit" disabled={growMutation.isPending || !growFilesystem} onClick={() => { if (growFilesystem) growMutation.mutate(growFilesystem.mountPoint); }}>{growMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Maximize2 className="h-4 w-4" />}{growMutation.isPending ? "正在扩容..." : "确认在线扩容"}</Button></>}
      >
        <div className="space-y-3 text-sm">
          <div className="grid gap-2 rounded-md border bg-muted/20 p-3 sm:grid-cols-3">
            <div><p className="text-xs text-muted-foreground">挂载点</p><p className="mt-1 font-mono">{growFilesystem?.mountPoint || "-"}</p></div>
            <div><p className="text-xs text-muted-foreground">文件系统</p><p className="mt-1">{growFilesystem?.type || "-"}</p></div>
            <div><p className="text-xs text-muted-foreground">当前容量</p><p className="mt-1 tabular-nums">{formatBytes(growFilesystem?.totalBytes ?? 0)}</p></div>
          </div>
          <p className="text-xs text-muted-foreground">请先确认虚拟磁盘已在宿主机侧扩容，并确保客户机内安装了 growpart 与对应文件系统工具。</p>
        </div>
      </Modal>
    </Card>
  );
}

function DiagnosticLines({ title, lines }: { title: string; lines: string[] }) {
  return <div><p className="mb-1 font-medium">{title}</p><pre className="max-h-36 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted/35 p-2 text-[11px] text-muted-foreground">{lines.join("\n")}</pre></div>;
}


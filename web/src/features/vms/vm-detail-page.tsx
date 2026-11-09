import { lazy, Suspense, useEffect, useState, type ReactNode } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  ArrowLeft,
  Check,
  CircleDot,
  Clipboard,
  Copy,
  Cpu,
  Download,
  HardDrive,
  LoaderCircle,
  MemoryStick,
  Monitor,
  MonitorPlay,
  Network,
  Pause,
  Pencil,
  Play,
  Power,
  RefreshCw,
  RotateCcw,
  Save,
  Trash2,
  X,
  type LucideIcon,
} from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { TaskStatus, VMAction, VMXMLFormat } from "@/api/types";
import { DetailItem } from "@/components/data/detail-item";
import { MetricCard } from "@/components/data/metric-card";
import { StatusBadge } from "@/components/feedback/status-badge";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { PageHeader } from "@/components/layout/page-header";
import {
  AlertDialog,
  AlertDialogAction,
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
import { Progress } from "@/components/ui/progress";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useSessionQuery } from "@/features/auth/use-session";
import { overviewQueryKey } from "@/features/overview/use-overview-query";
import { useTaskQuery } from "@/features/tasks/use-tasks-query";
import { useVMMetricsQuery, useVMQuery, useVMXMLQuery, vmQueryKey, vmXMLQueryKey } from "@/features/vms/use-vm-query";
import { downloadVMExport } from "@/features/vms/vm-export-model";
import { canEditVMConfigXML, validateVMXMLDraft } from "@/features/vms/vm-xml-edit-model";
import { VMBackupCard } from "@/features/vms/vm-backup-card";
import { VMDisksCard } from "@/features/vms/vm-disks-card";
import { VMPassthroughCard } from "@/features/vms/vm-passthrough-card";
import { VMResourcesCard } from "@/features/vms/vm-resources-card";
import { vmResourcesDisabledReason } from "@/features/vms/vm-resources-model";
import { VMSnapshotPoliciesCard } from "@/features/vms/vm-snapshot-policies-card";
import { VMSnapshotsCard } from "@/features/vms/vm-snapshots-card";
import { ResourceTrendCard } from "@/features/metrics/resource-trend-card";
import { formatBytes, cn } from "@/lib/utils";

type ActionMeta = {
  label: string;
  description: string;
  icon: LucideIcon;
  destructive?: boolean;
  confirm?: boolean;
};

const XMLCodeEditor = lazy(() =>
  import("@/components/code/xml-code-editor").then((module) => ({ default: module.XMLCodeEditor })),
);

const actionOrder: VMAction[] = ["start", "shutdown", "reboot", "reset", "pause", "resume", "destroy"];

const taskStatusLabel: Record<TaskStatus, string> = {
  queued: "排队中",
  running: "执行中",
  succeeded: "已完成",
  failed: "失败",
  interrupted: "已中断",
};

const actionMeta: Record<VMAction, ActionMeta> = {
  start: { label: "启动", description: "启动这台虚拟机", icon: Play },
  shutdown: { label: "正常关机", description: "向客户机发送 ACPI 关机请求", icon: Power },
  reboot: { label: "重启", description: "请求客户机正常重启", icon: RefreshCw },
  reset: { label: "强制重启", description: "立即重置虚拟机，可能丢失未保存数据", icon: RotateCcw, confirm: true },
  pause: { label: "暂停", description: "暂停 vCPU 执行", icon: Pause },
  resume: { label: "恢复", description: "恢复虚拟机运行", icon: CircleDot },
  destroy: { label: "强制下电", description: "立即切断虚拟机电源，可能丢失未保存数据", icon: Power, destructive: true, confirm: true },
};

function confirmCopy(action: VMAction | null) {
  if (action === "destroy") {
    return {
      title: "确认强制下电？",
      description: "该操作会立即终止虚拟机，不会等待客户机保存数据。未保存的磁盘写入可能丢失。",
      button: "确认强制下电",
    };
  }
  return {
    title: "确认强制重启？",
    description: "该操作会立即重置虚拟机并重启，未保存的数据可能丢失。",
    button: "确认强制重启",
  };
}

export function VMDetailPage({ uuid }: { uuid: string }) {
  const vmQuery = useVMQuery(uuid);
  const metricsQuery = useVMMetricsQuery(uuid);
  const session = useSessionQuery();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [confirmation, setConfirmation] = useState<VMAction | null>(null);
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [autostartNotice, setAutostartNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteName, setDeleteName] = useState("");
  const [deleteError, setDeleteError] = useState("");
  const [cloneOpen, setCloneOpen] = useState(false);
  const [cloneName, setCloneName] = useState("");
  const [cloneAutostart, setCloneAutostart] = useState(false);
  const [cloneStart, setCloneStart] = useState(false);
  const [cloneError, setCloneError] = useState("");
  const [cloneTaskID, setCloneTaskID] = useState<string | null>(null);
  const [xmlFormat, setXmlFormat] = useState<VMXMLFormat>("live");
  const [copyNotice, setCopyNotice] = useState<"copied" | "error" | "">("");
  const [xmlEditing, setXmlEditing] = useState(false);
  const [xmlDraft, setXmlDraft] = useState("");
  const [xmlEditError, setXmlEditError] = useState("");
  const [xmlEditNotice, setXmlEditNotice] = useState("");
  const [xmlConfirmOpen, setXmlConfirmOpen] = useState(false);
  const [exportNotice, setExportNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const xmlQuery = useVMXMLQuery(uuid, xmlFormat);
  const cloneTaskQuery = useTaskQuery(cloneTaskID);

  const mutation = useMutation({
    mutationFn: (action: VMAction) => api.vmAction(uuid, action),
    onSuccess: (updated, action) => {
      queryClient.setQueryData(vmQueryKey(uuid), updated);
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
      void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
      setNotice({ tone: "success", text: `${actionMeta[action].label}指令已提交，状态已刷新。` });
    },
    onError: (error) => {
      setNotice({ tone: "error", text: error instanceof ApiError ? error.message : "操作失败，请稍后重试。" });
    },
  });

  const autostartMutation = useMutation({
    mutationFn: (enabled: boolean) => api.setVMAutostart(uuid, { enabled }),
    onSuccess: (updated, enabled) => {
      queryClient.setQueryData(vmQueryKey(uuid), updated);
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
      void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
      setAutostartNotice({ tone: "success", text: enabled ? "开机自启动已启用。" : "开机自启动已禁用。" });
    },
    onError: (error) => {
      setAutostartNotice({ tone: "error", text: error instanceof ApiError ? error.message : "更新开机自启动失败，请稍后重试。" });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: () => api.deleteVM(uuid),
    onSuccess: async () => {
      setDeleteOpen(false);
      queryClient.removeQueries({ queryKey: vmQueryKey(uuid) });
      await queryClient.invalidateQueries({ queryKey: ["vms"] });
      await queryClient.invalidateQueries({ queryKey: overviewQueryKey });
      await navigate({ to: "/vms" });
    },
    onError: (error) => {
      setDeleteError(error instanceof ApiError ? error.message : "删除虚拟机失败，请稍后重试。");
    },
  });

  const cloneMutation = useMutation({
    mutationFn: (input: { name: string; autostart: boolean; start: boolean }) => api.cloneVM(uuid, input),
    onSuccess: (task) => {
      setCloneTaskID(task.id);
      setCloneOpen(false);
      setCloneError("");
      void queryClient.invalidateQueries({ queryKey: ["tasks"] });
    },
    onError: (error) => {
      setCloneError(error instanceof ApiError ? error.message : "创建克隆任务失败，请稍后重试。");
    },
  });

  const exportMutation = useMutation({
    mutationFn: () => api.exportVM(uuid),
    onSuccess: (exported) => {
      downloadVMExport(exported);
      setExportNotice({ tone: "success", text: `已导出 ${exported.vm.name} 的配置与元数据。` });
    },
    onError: (error) => {
      setExportNotice({ tone: "error", text: error instanceof ApiError ? error.message : "导出虚拟机配置失败，请稍后重试。" });
    },
  });

  const xmlUpdateMutation = useMutation({
    mutationFn: (xmlText: string) => api.updateVMXML(uuid, { xml: xmlText }),
    onSuccess: (updated) => {
      queryClient.setQueryData(vmXMLQueryKey(uuid, "config"), updated);
      void queryClient.invalidateQueries({ queryKey: vmQueryKey(uuid) });
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
      void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
      setXmlConfirmOpen(false);
      setXmlEditing(false);
      setXmlDraft("");
      setXmlEditError("");
      setXmlEditNotice("持久配置 XML 已保存；运行中的域将在下次启动时使用新定义。");
    },
    onError: (error) => {
      setXmlConfirmOpen(false);
      setXmlEditError(error instanceof ApiError ? error.message : "保存域 XML 失败，请检查配置后重试。");
    },
  });

  useEffect(() => {
    if (cloneTaskQuery.data?.status !== "succeeded") return;
    void queryClient.invalidateQueries({ queryKey: ["vms"] });
    void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
  }, [cloneTaskQuery.data?.status, queryClient]);

  const vm = vmQuery.data;
  const readOnly = Boolean(session.data?.readOnly);
  const viewer = session.data?.user.role === "viewer";
  const canMutate = !readOnly && !viewer;
  const canEditXML = vm ? canEditVMConfigXML(session.data?.user.role, readOnly, vm.persistent, xmlFormat) : false;
  const unavailableReason = readOnly
    ? "系统当前处于只读模式"
    : viewer
      ? "当前账号没有虚拟机操作权限"
      : mutation.isPending
        ? "正在执行上一个操作"
        : "";
  const snapshotDisabledReason = readOnly
    ? "系统当前处于只读模式"
    : viewer
      ? "当前账号没有虚拟机操作权限"
      : "";
  const resourcesDisabledReason = vm ? vmResourcesDisabledReason(session.data?.user.role, readOnly, vm) : "";
  function submitAction(action: VMAction) {
    setNotice(null);
    mutation.mutate(action);
  }

  function requestAction(action: VMAction) {
    if (actionMeta[action].confirm) {
      setConfirmation(action);
      return;
    }
    submitAction(action);
  }

  async function copyXML() {
    if (!xmlQuery.data?.xml) return;
    try {
      await navigator.clipboard.writeText(xmlQuery.data.xml);
      setCopyNotice("copied");
    } catch {
      setCopyNotice("error");
    }
  }

  function beginXMLEdit() {
    if (!xmlQuery.data?.xml) return;
    setXmlDraft(xmlQuery.data.xml);
    setXmlEditError("");
    setXmlEditNotice("");
    setCopyNotice("");
    setXmlEditing(true);
  }

  function cancelXMLEdit() {
    setXmlEditing(false);
    setXmlDraft("");
    setXmlEditError("");
  }

  function requestSaveXML() {
    const validationError = validateVMXMLDraft(xmlDraft);
    if (validationError) {
      setXmlEditError(validationError);
      return;
    }
    setXmlEditError("");
    setXmlConfirmOpen(true);
  }

  if (vmQuery.isLoading) return <LoadingState label="正在读取虚拟机详情" />;
  if (vmQuery.isError || !vm) {
    return <ErrorState message={vmQuery.error instanceof Error ? vmQuery.error.message : "无法读取虚拟机详情"} onRetry={() => void vmQuery.refetch()} />;
  }

  const cloneDisabledReason = readOnly
    ? "系统当前处于只读模式"
    : viewer
      ? "当前账号没有虚拟机操作权限"
      : !vm.persistent
        ? "临时域不支持克隆"
        : vm.state !== "shutoff" && vm.state !== "crashed"
          ? "请先关闭虚拟机"
          : "";
  const backupDisabledReason = readOnly
    ? "系统当前处于只读模式"
    : viewer
      ? "当前账号没有虚拟机操作权限"
      : !vm.persistent
        ? "临时域不支持备份"
        : vm.state !== "shutoff" && vm.state !== "crashed"
          ? "请先关闭虚拟机"
          : "";
  const allowedActions = actionOrder.filter((action) => vm.allowedActions.includes(action));
  const dialogCopy = confirmCopy(confirmation);
  const consoleReady = canMutate && ["running", "blocked", "paused"].includes(vm.state) && vm.graphics.some((graphics) => graphics.type.toLowerCase() === "vnc");
  const consoleReason = readOnly
    ? "只读模式下不能打开可交互控制台"
    : viewer
      ? "当前账号没有控制台权限"
      : !vm.graphics.some((graphics) => graphics.type.toLowerCase() === "vnc")
        ? "当前虚拟机没有 VNC 图形设备"
        : "请先启动虚拟机";
  const autostartDisabledReason = readOnly
    ? "系统当前处于只读模式"
    : viewer
      ? "当前账号没有虚拟机操作权限"
      : !vm.persistent
        ? "临时域不能设置开机自启动"
        : autostartMutation.isPending
          ? "正在更新开机自启动"
          : "";

  return (
    <>
      <PageHeader
        title={<span data-testid="vm-detail-name">{vm.name}</span>}
        documentTitle={vm.name}
        breadcrumb={<span className="inline-flex items-center gap-1.5"><Link to="/vms" className="inline-flex items-center gap-1 hover:text-foreground"><ArrowLeft className="h-3 w-3" />虚拟机</Link><span>/</span><span className="font-mono">{vm.uuid.slice(0, 8)}</span></span>}
        description={<span className="font-mono text-xs">{vm.uuid}</span>}
        actions={<div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            data-testid="vm-export-button"
            disabled={exportMutation.isPending}
            title="下载包含虚拟机定义、XML 与快照元数据的 JSON 文件"
            onClick={() => { setExportNotice(null); exportMutation.mutate(); }}
          >
            {exportMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}导出配置
          </Button>
          <Button
            variant="outline"
            size="sm"
            data-testid="vm-console-open"
            disabled={!consoleReady}
            title={consoleReady ? "打开 VNC 控制台" : consoleReason}
            onClick={() => void navigate({ to: "/console/vms/$uuid", params: { uuid } })}
          ><MonitorPlay className="h-4 w-4" />打开控制台</Button>
          <Button variant="outline" size="sm" onClick={() => void vmQuery.refetch()} disabled={vmQuery.isFetching}><RefreshCw className={cn("h-4 w-4", vmQuery.isFetching && "animate-spin")} />刷新</Button>
        </div>}
      />

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <StatusBadge state={vm.state} />
        <Badge variant={vm.persistent ? "outline" : "secondary"}>{vm.persistent ? "持久化域" : "临时域"}</Badge>
        {vm.autostart ? <Badge variant="success">开机自启动</Badge> : <Badge variant="secondary">未设置自启动</Badge>}
        <span className="text-xs text-muted-foreground">{vm.osType || "未知 OS"} · {vm.arch || "未知架构"} · {vm.machine || "未知机型"}</span>
      </div>

      {exportNotice ? (
        <p
          data-testid="vm-export-notice"
          role="status"
          className={cn("mb-4 rounded-md border px-3 py-2 text-xs", exportNotice.tone === "success" ? "border-success/30 bg-success/10 text-success" : "border-destructive/30 bg-destructive/5 text-destructive")}
        >
          {exportNotice.text}
        </p>
      ) : null}

      {readOnly ? (
        <div className="mb-4 flex items-start gap-3 rounded-lg border border-warning/35 bg-warning/10 px-4 py-3 text-sm text-warning" data-testid="vm-read-only-notice" role="status">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <p>只读模式已开启，生命周期操作已禁用。管理员可通过 <span className="font-mono">VIRTMGR_READ_ONLY=false</span> 启用写操作。</p>
        </div>
      ) : null}
      {viewer ? (
        <div className="mb-4 flex items-start gap-3 rounded-lg border border-warning/35 bg-warning/10 px-4 py-3 text-sm text-warning" role="status">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <p>当前账号角色为 Viewer，仅允许查看虚拟机状态和硬件信息。</p>
        </div>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>生命周期操作</CardTitle>
          <CardDescription>操作由后端按 libvirt 当前状态校验；危险动作需要二次确认。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {allowedActions.length ? (
            <div className="flex flex-wrap gap-2">
              {allowedActions.map((action) => {
                const meta = actionMeta[action];
                const Icon = mutation.isPending && mutation.variables === action ? LoaderCircle : meta.icon;
                return (
                  <Button
                    key={action}
                    variant={meta.destructive ? "destructive" : "outline"}
                    size="sm"
                    disabled={Boolean(unavailableReason)}
                    title={unavailableReason || meta.description}
                    data-testid={`vm-action-${action}`}
                    onClick={() => requestAction(action)}
                  >
                    <Icon className={cn("h-4 w-4", mutation.isPending && mutation.variables === action && "animate-spin")} />
                    {meta.label}
                  </Button>
                );
              })}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">当前状态没有可用的生命周期操作。</p>
          )}
          {unavailableReason ? <p className="text-xs text-muted-foreground">{unavailableReason}。</p> : null}
          {notice ? (
            <p className={cn("rounded-md border px-3 py-2 text-xs", notice.tone === "success" ? "border-success/30 bg-success/10 text-success" : "border-destructive/30 bg-destructive/5 text-destructive")} role="status">
              {notice.text}
            </p>
          ) : null}
        </CardContent>
      </Card>

      <div className="mt-4 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard label="vCPU" value={vm.vcpus} hint={`上限 ${vm.maxVcpus || vm.vcpus} vCPU`} icon={Cpu} tone="primary" testId="vm-detail-vcpus" />
        <MetricCard label="内存" value={formatBytes(vm.memoryBytes)} hint={`最大 ${formatBytes(vm.maxMemoryBytes)}`} icon={MemoryStick} tone="success" testId="vm-detail-memory" />
        <MetricCard label="磁盘" value={vm.disks.length} hint="已连接块设备" icon={HardDrive} tone="neutral" testId="vm-detail-disks" />
        <MetricCard label="网络接口" value={vm.interfaces.length} hint="虚拟网卡" icon={Network} tone="neutral" testId="vm-detail-interfaces" />
      </div>

      <VMResourcesCard vm={vm} canMutate={canMutate && !resourcesDisabledReason} disabledReason={resourcesDisabledReason} />

      <div className="mt-4">
        <ResourceTrendCard
          title="资源趋势"
          description="当前虚拟机的 CPU、内存和虚拟网卡采样；运行中或阻塞状态会持续更新。"
          metrics={metricsQuery.data}
          isLoading={metricsQuery.isLoading}
          error={metricsQuery.isError ? (metricsQuery.error instanceof Error ? metricsQuery.error.message : "无法读取虚拟机资源指标") : undefined}
          testId="vm-metrics"
          historyKey={uuid}
        />
      </div>

      <Card className="mt-4" data-testid="vm-clone-card">
        <CardHeader>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <CardTitle>克隆虚拟机</CardTitle>
              <CardDescription>复制持久域定义和可写磁盘，并生成新的 UUID、MAC 和磁盘文件。源虚拟机必须处于关机或异常停止状态。</CardDescription>
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              data-testid="vm-clone-open"
              disabled={Boolean(cloneDisabledReason) || cloneMutation.isPending}
              title={cloneDisabledReason || "创建虚拟机克隆"}
              onClick={() => {
                setCloneName(`${vm.name}-clone`.slice(0, 64));
                setCloneAutostart(false);
                setCloneStart(false);
                setCloneError("");
                setCloneOpen((open) => !open);
              }}
            >
              <Copy className="h-4 w-4" />克隆虚拟机
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {cloneDisabledReason ? <p className="text-xs text-muted-foreground">{cloneDisabledReason}。</p> : null}
          {cloneOpen ? (
            <form
              data-testid="vm-clone-form"
              className="rounded-lg border bg-muted/20 p-4"
              onSubmit={(event) => {
                event.preventDefault();
                setCloneError("");
                cloneMutation.mutate({ name: cloneName.trim(), autostart: cloneAutostart, start: cloneStart });
              }}
            >
              <div className="grid gap-4 md:grid-cols-[1fr_auto] md:items-end">
                <div className="space-y-2">
                  <Label htmlFor="vm-clone-name">克隆名称</Label>
                  <Input
                    id="vm-clone-name"
                    value={cloneName}
                    onChange={(event) => setCloneName(event.target.value)}
                    placeholder={`${vm.name}-clone`}
                    autoComplete="off"
                    required
                    pattern="[A-Za-z0-9][A-Za-z0-9_.-]{0,63}"
                  />
                </div>
                <div className="flex flex-col gap-2 text-sm">
                  <label className="inline-flex items-center gap-2"><input type="checkbox" checked={cloneStart} onChange={(event) => setCloneStart(event.target.checked)} />克隆后启动</label>
                  <label className="inline-flex items-center gap-2"><input type="checkbox" checked={cloneAutostart} onChange={(event) => setCloneAutostart(event.target.checked)} />设置开机自启动</label>
                </div>
              </div>
              <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
                <p className="text-xs text-muted-foreground">磁盘会复制到源卷所在的活跃存储池；克隆任务可在任务中心查看。</p>
                <div className="flex items-center gap-2">
                  <Button type="button" variant="ghost" size="sm" onClick={() => { setCloneOpen(false); setCloneError(""); }}>取消</Button>
                  <Button type="submit" size="sm" data-testid="vm-clone-submit" disabled={cloneMutation.isPending || !cloneName.trim()}>
                    {cloneMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Copy className="h-4 w-4" />}
                    {cloneMutation.isPending ? "正在提交..." : "开始克隆"}
                  </Button>
                </div>
              </div>
              {cloneError ? <p role="alert" className="mt-3 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{cloneError}</p> : null}
            </form>
          ) : null}
          {cloneTaskQuery.data ? (
            <div className="space-y-3 rounded-lg border bg-card px-4 py-3" data-testid="vm-clone-task" data-status={cloneTaskQuery.data.status}>
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant={cloneTaskQuery.data.status === "succeeded" ? "success" : cloneTaskQuery.data.status === "failed" ? "destructive" : "warning"}>
                  {taskStatusLabel[cloneTaskQuery.data.status]}
                </Badge>
                <span className="text-sm font-medium">{cloneTaskQuery.data.step}</span>
                <span className="ml-auto text-xs tabular-nums text-muted-foreground">{cloneTaskQuery.data.progress}%</span>
              </div>
              <Progress value={cloneTaskQuery.data.progress} />
              {cloneTaskQuery.data.error ? <p className="text-xs text-destructive">{cloneTaskQuery.data.error}</p> : null}
              <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                <span className="font-mono">{cloneTaskQuery.data.id}</span>
                <Link to="/tasks" className="text-primary hover:underline">在任务中心查看</Link>
              </div>
            </div>
          ) : null}
        </CardContent>
      </Card>

      <VMBackupCard
        uuid={uuid}
        vmName={vm.name}
        state={vm.state}
        persistent={vm.persistent}
        role={session.data?.user.role}
        readOnly={readOnly}
        disabledReason={backupDisabledReason}
      />

      <div className="mt-4 grid gap-4 xl:grid-cols-[0.9fr_1.1fr]">
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between gap-3">
              <div><CardTitle>基础信息</CardTitle><CardDescription>来自 libvirt 域定义与实时状态。</CardDescription></div>
              <StatusBadge state={vm.state} />
            </div>
          </CardHeader>
          <CardContent>
            <DetailItem label="名称">{vm.name}</DetailItem>
            <DetailItem label="主机名"><span data-testid="vm-hostname">{vm.hostname || "-"}</span></DetailItem>
            <DetailItem label="IP 地址">
              <span data-testid="vm-ip-addresses" className="font-mono text-xs">{vm.ipAddresses?.length ? vm.ipAddresses.join(", ") : "-"}</span>
            </DetailItem>
            <DetailItem label="Guest Agent">
              <Badge variant={vm.agentConnected ? "success" : "secondary"} data-testid="vm-agent-status">{vm.agentConnected ? "已连接" : "未连接"}</Badge>
            </DetailItem>
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
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {vm.autostart ? "宿主机启动 libvirt 后自动启动此虚拟机。" : "虚拟机不会随宿主机或 libvirt 自动启动。"}
                </p>
                {autostartNotice ? (
                  <p
                    className={cn("mt-2 text-xs", autostartNotice.tone === "success" ? "text-success" : "text-destructive")}
                    data-testid="vm-autostart-notice"
                    role="status"
                  >
                    {autostartNotice.text}
                  </p>
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
                onClick={() => {
                  setAutostartNotice(null);
                  autostartMutation.mutate(!vm.autostart);
                }}
                className={cn(
                  "relative inline-flex h-6 w-11 shrink-0 items-center rounded-full border transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
                  vm.autostart ? "border-primary bg-primary" : "border-input bg-muted",
                )}
              >
                <span
                  className={cn(
                    "pointer-events-none block h-4 w-4 rounded-full bg-background shadow-sm transition-transform",
                    autostartMutation.isPending ? "animate-pulse" : "",
                    vm.autostart ? "translate-x-[1.35rem]" : "translate-x-1",
                  )}
                />
              </button>
            </div>
            {autostartDisabledReason ? <p className="mt-2 text-xs text-muted-foreground">{autostartDisabledReason}。</p> : null}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>图形控制台</CardTitle>
            <CardDescription>由 libvirt 域 XML 暴露的控制台端点。</CardDescription>
          </CardHeader>
          <CardContent>
            {vm.graphics.length ? vm.graphics.map((graphics, index) => (
              <DetailItem key={`${graphics.type}-${index}`} label={graphics.type.toUpperCase()}>
                <span className="inline-flex items-center gap-1.5 font-mono text-xs"><Monitor className="h-3.5 w-3.5" />{graphics.listen || "*"}:{graphics.port || "-"}</span>
              </DetailItem>
            )) : <p className="text-sm text-muted-foreground">当前虚拟机未配置图形控制台。</p>}
          </CardContent>
        </Card>
      </div>

      <VMDisksCard vm={vm} role={session.data?.user.role} readOnly={readOnly} />

      <ResourceTable
        title="网络接口"
        description="libvirt 网络、桥接或直通网卡配置。"
        empty="当前虚拟机没有网络接口。"
        headers={["类型", "来源", "MAC", "模型", "PCI 地址"]}
        rows={vm.interfaces.flatMap((network, index) => [
          { key: `${index}-0`, cell: <span>{network.type || "-"}</span> },
          { key: `${index}-1`, cell: <span className="font-mono text-xs">{network.source || "-"}</span> },
          { key: `${index}-2`, cell: <span className="font-mono text-xs">{network.mac || "-"}</span> },
          { key: `${index}-3`, cell: <span>{network.model || "-"}</span> },
          { key: `${index}-4`, cell: <span className="font-mono text-xs text-muted-foreground">{network.address || "-"}</span> },
        ])}
      />

      <VMPassthroughCard vm={vm} canMutate={canMutate} disabledReason={snapshotDisabledReason} />

      <VMSnapshotPoliciesCard uuid={uuid} canMutate={canMutate} disabledReason={snapshotDisabledReason} />

      <VMSnapshotsCard uuid={uuid} canMutate={canMutate} disabledReason={snapshotDisabledReason} />

      <Card className="mt-4">
        <CardHeader>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <CardTitle>域 XML</CardTitle>
              <CardDescription>{xmlEditing ? "编辑持久配置 XML。名称和 UUID 必须保持不变；仅更新定义，不启动虚拟机。" : "查看 libvirt 域定义。运行态包含当前运行时覆盖，持久配置为 inactive XML。"}</CardDescription>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <select
                aria-label="XML 类型"
                data-testid="vm-xml-format"
                value={xmlFormat}
                disabled={xmlEditing}
                onChange={(event) => { setXmlFormat(event.target.value as VMXMLFormat); setCopyNotice(""); setXmlEditNotice(""); setXmlEditError(""); }}
                className="h-8 rounded-md border border-input bg-card px-3 text-xs shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              >
                <option value="live">运行态 XML</option>
                <option value="config">持久配置 XML</option>
              </select>
              <Button
                variant="outline"
                size="sm"
                data-testid="vm-xml-copy"
                disabled={!xmlQuery.data?.xml || xmlQuery.isFetching}
                onClick={() => void copyXML()}
              >
                {copyNotice === "copied" ? <Check className="h-4 w-4" /> : <Clipboard className="h-4 w-4" />}复制 XML
              </Button>
              {canEditXML && !xmlEditing ? (
                <Button variant="outline" size="sm" data-testid="vm-xml-edit" disabled={!xmlQuery.data?.xml || xmlQuery.isFetching} onClick={beginXMLEdit}>
                  <Pencil className="h-4 w-4" />编辑 XML
                </Button>
              ) : null}
              {xmlEditing ? (
                <>
                  <Button variant="ghost" size="sm" data-testid="vm-xml-cancel" disabled={xmlUpdateMutation.isPending} onClick={cancelXMLEdit}>
                    <X className="h-4 w-4" />取消
                  </Button>
                  <Button size="sm" data-testid="vm-xml-save" disabled={xmlUpdateMutation.isPending} onClick={requestSaveXML}>
                    {xmlUpdateMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}保存定义
                  </Button>
                </>
              ) : null}
            </div>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <div className="flex flex-wrap items-center gap-3 border-b px-4 py-2 text-xs text-muted-foreground">
            <Badge variant="outline" data-testid="vm-xml-format-badge">{xmlQuery.data?.format === "config" ? "持久配置" : "运行态"}</Badge>
            <span>{xmlEditing ? "编辑模式" : "只读视图"}</span>
            {xmlQuery.data ? <span>{xmlQuery.data.xml.length.toLocaleString()} 字符</span> : null}
            {copyNotice === "copied" ? <span className="text-success" data-testid="vm-xml-copy-notice">已复制到剪贴板</span> : null}
            {copyNotice === "error" ? <span className="text-destructive" data-testid="vm-xml-copy-notice">复制失败，请手动选择 XML</span> : null}
            {xmlEditNotice ? <span className="text-success" data-testid="vm-xml-edit-notice">{xmlEditNotice}</span> : null}
            {xmlEditError ? <span className="text-destructive" data-testid="vm-xml-edit-error" role="alert">{xmlEditError}</span> : null}
          </div>
          {xmlQuery.isLoading ? <LoadingState label="正在读取域 XML" /> : null}
          {xmlQuery.isError ? <ErrorState message={xmlQuery.error instanceof Error ? xmlQuery.error.message : "无法读取域 XML"} onRetry={() => void xmlQuery.refetch()} className="m-4" /> : null}
          {xmlQuery.data && xmlEditing ? (
            <div className="h-[32rem] overflow-hidden">
              <Suspense fallback={<LoadingState label="正在加载 XML 编辑器" />}>
                <XMLCodeEditor
                  value={xmlDraft}
                  onChange={(value) => { setXmlDraft(value); setXmlEditError(""); }}
                  ariaLabel="域 XML 编辑器"
                  testId="vm-xml-editor"
                  className="h-full"
                />
              </Suspense>
            </div>
          ) : xmlQuery.data ? (
            <div className="h-[32rem] overflow-hidden">
              <Suspense fallback={<LoadingState label="正在加载 XML 编辑器" />}>
                <XMLCodeEditor
                  value={xmlQuery.data.xml}
                  readOnly
                  ariaLabel="域 XML 只读内容"
                  testId="vm-xml-content"
                  format={xmlQuery.data.format}
                  className="h-full"
                />
              </Suspense>
            </div>
          ) : null}
        </CardContent>
      </Card>

      {canMutate ? (
        <Card className="mt-4 border-destructive/30">
          <CardHeader>
            <CardTitle>危险操作</CardTitle>
            <CardDescription>删除只移除 libvirt 域定义，磁盘文件会保留。此操作不可撤销。</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-3 sm:flex-row sm:items-center">
            <Button
              variant="destructive"
              data-testid="vm-delete-button"
              disabled={vm.state !== "shutoff" && vm.state !== "crashed"}
              title={vm.state !== "shutoff" && vm.state !== "crashed" ? "请先关闭虚拟机" : "删除虚拟机域定义"}
              onClick={() => { setDeleteName(""); setDeleteError(""); setDeleteOpen(true); }}
            >
              <Trash2 className="h-4 w-4" />删除虚拟机
            </Button>
            <p className="text-xs text-muted-foreground">{vm.state === "shutoff" || vm.state === "crashed" ? "删除前需要输入虚拟机名称确认。" : "运行中的虚拟机不能删除，请先执行正常关机或强制下电。"}</p>
          </CardContent>
        </Card>
      ) : null}

      <AlertDialog open={xmlConfirmOpen} onOpenChange={setXmlConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>保存持久配置 XML？</AlertDialogTitle>
            <AlertDialogDescription>
              将使用 libvirt 校验并更新 <span className="font-medium text-foreground">{vm.name}</span> 的持久域定义。名称和 UUID 必须保持不变；不会启动虚拟机，运行中的域将在下次启动时使用新定义。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={xmlUpdateMutation.isPending}>取消</AlertDialogCancel>
            <Button data-testid="confirm-vm-xml-save" disabled={xmlUpdateMutation.isPending} onClick={() => xmlUpdateMutation.mutate(xmlDraft)}>
              {xmlUpdateMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}保存定义
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={confirmation !== null} onOpenChange={(open) => { if (!open) setConfirmation(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{dialogCopy.title}</AlertDialogTitle>
            <AlertDialogDescription>{dialogCopy.description}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={() => { const action = confirmation; setConfirmation(null); if (action) submitAction(action); }}>{dialogCopy.button}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={deleteOpen} onOpenChange={(open) => { if (!open) { setDeleteOpen(false); setDeleteError(""); } }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除虚拟机？</AlertDialogTitle>
            <AlertDialogDescription>将删除 <span className="font-medium text-foreground">{vm.name}</span> 的 libvirt 域定义，磁盘文件保留。此操作不可撤销。</AlertDialogDescription>
          </AlertDialogHeader>
          <div className="space-y-2">
            <Label htmlFor="delete-vm-name">输入虚拟机名称确认</Label>
            <Input id="delete-vm-name" value={deleteName} onChange={(event) => setDeleteName(event.target.value)} placeholder={vm.name} autoComplete="off" />
          </div>
          {deleteError ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert">{deleteError}</p> : null}
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <Button
              variant="destructive"
              data-testid="confirm-delete-vm"
              disabled={deleteName !== vm.name || deleteMutation.isPending}
              onClick={() => { setDeleteError(""); deleteMutation.mutate(); }}
            >
              {deleteMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : null}
              确认删除
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

type ResourceRow = Array<{ key: string; cell: ReactNode }>;

function ResourceTable({ title, description, empty, headers, rows }: {
  title: string;
  description: string;
  empty: string;
  headers: string[];
  rows: ResourceRow;
}) {
  return (
    <Card className="mt-4">
      <CardHeader><CardTitle>{title}</CardTitle><CardDescription>{description}</CardDescription></CardHeader>
      <CardContent className="p-0">
        {rows.length ? (
          <Table className="min-w-[760px]">
            <TableHeader><TableRow>{headers.map((header) => <TableHead key={header}>{header}</TableHead>)}</TableRow></TableHeader>
            <TableBody>
              {Array.from({ length: rows.length / headers.length }, (_, rowIndex) => (
                <TableRow key={rowIndex}>{rows.slice(rowIndex * headers.length, (rowIndex + 1) * headers.length).map((item) => <TableCell key={item.key}>{item.cell}</TableCell>)}</TableRow>
              ))}
            </TableBody>
          </Table>
        ) : <p className="p-5 text-sm text-muted-foreground">{empty}</p>}
      </CardContent>
    </Card>
  );
}

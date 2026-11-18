import { useEffect, useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, ArrowLeft, CircleDot, KeyRound, LoaderCircle, MonitorPlay, Pause, Play, Power, RefreshCw, RotateCcw, type LucideIcon } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { VMAction, VMXMLFormat } from "@/api/types";
import { StatusBadge } from "@/components/feedback/status-badge";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useSessionQuery } from "@/features/auth/use-session";
import { VMGuestAgentCard } from "@/features/guest-agent/vm-guest-agent-card";
import { VMPasswordDialog } from "@/features/guest-agent/vm-password-dialog";
import { overviewQueryKey } from "@/features/overview/use-overview-query";
import { useTaskQuery } from "@/features/tasks/use-tasks-query";
import { useVMConfigVersionsQuery } from "@/features/vms/use-vm-config-versions-query";
import { useVMMetricsQuery, useVMQuery, useVMXMLQuery, vmQueryKey, vmXMLQueryKey } from "@/features/vms/use-vm-query";
import { VMDetailAdvancedTab } from "@/features/vms/vm-detail-advanced-tab";
import { VMDetailConfigTab } from "@/features/vms/vm-detail-config-tab";
import { VMActionConfirmDialog, VMDeleteDialog, VMXMLSaveDialog } from "@/features/vms/vm-detail-dialogs";
import { VMDetailOverviewTab } from "@/features/vms/vm-detail-overview-tab";
import { VMDetailSnapshotsTab } from "@/features/vms/vm-detail-snapshots-tab";
import { downloadVMExport } from "@/features/vms/vm-export-model";
import { canEditVMConfigXML, validateVMXMLDraft } from "@/features/vms/vm-xml-edit-model";
import { vmResourcesDisabledReason } from "@/features/vms/vm-resources-model";
import { queryKeys } from "@/lib/query-keys";
import { cn } from "@/lib/utils";

type ActionMeta = {
  label: string;
  description: string;
  icon: LucideIcon;
  destructive?: boolean;
  confirm?: boolean;
};

type VMTab = "overview" | "config" | "guest" | "snapshots" | "advanced";

const vmTabs: Array<{ id: VMTab; label: string }> = [
  { id: "overview", label: "概览" },
  { id: "config", label: "配置" },
  { id: "guest", label: "客户机" },
  { id: "snapshots", label: "快照与备份" },
  { id: "advanced", label: "高级" },
];

const actionOrder: VMAction[] = ["start", "shutdown", "reboot", "reset", "pause", "resume", "destroy"];

const actionMeta: Record<VMAction, ActionMeta> = {
  start: { label: "启动", description: "启动这台虚拟机", icon: Play },
  shutdown: { label: "正常关机", description: "向客户机发送 ACPI 关机请求", icon: Power },
  reboot: { label: "重启", description: "请求客户机正常重启", icon: RefreshCw },
  reset: { label: "强制重启", description: "立即重置虚拟机，可能丢失未保存数据", icon: RotateCcw, confirm: true },
  pause: { label: "暂停", description: "暂停 vCPU 执行", icon: Pause },
  resume: { label: "恢复", description: "恢复虚拟机运行", icon: CircleDot },
  destroy: { label: "强制下电", description: "立即切断虚拟机电源，可能丢失未保存数据", icon: Power, destructive: true, confirm: true },
};

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
  const [versionsOpen, setVersionsOpen] = useState(false);
  const [exportNotice, setExportNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [activeTab, setActiveTab] = useState<VMTab>("overview");
  const [passwordOpen, setPasswordOpen] = useState(false);
  const xmlQuery = useVMXMLQuery(uuid, xmlFormat);
  const persistentXMLQuery = useVMXMLQuery(uuid, "config");
  const versionsQuery = useVMConfigVersionsQuery(uuid);
  const cloneTaskQuery = useTaskQuery(cloneTaskID);

  const mutation = useMutation({
    mutationFn: (action: VMAction) => api.vmAction(uuid, action),
    onSuccess: (updated, action) => {
      queryClient.setQueryData(vmQueryKey(uuid), updated);
      void queryClient.invalidateQueries({ queryKey: queryKeys.vms.root });
      void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
      setNotice({ tone: "success", text: `${actionMeta[action].label}指令已提交，状态已刷新。` });
    },
    onError: (error) => setNotice({ tone: "error", text: error instanceof ApiError ? error.message : "操作失败，请稍后重试。" }),
  });

  const autostartMutation = useMutation({
    mutationFn: (enabled: boolean) => api.setVMAutostart(uuid, { enabled }),
    onSuccess: (updated, enabled) => {
      queryClient.setQueryData(vmQueryKey(uuid), updated);
      void queryClient.invalidateQueries({ queryKey: queryKeys.vms.root });
      void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
      setAutostartNotice({ tone: "success", text: enabled ? "开机自启动已启用。" : "开机自启动已禁用。" });
    },
    onError: (error) => setAutostartNotice({ tone: "error", text: error instanceof ApiError ? error.message : "更新开机自启动失败，请稍后重试。" }),
  });

  const deleteMutation = useMutation({
    mutationFn: () => api.deleteVM(uuid),
    onSuccess: async () => {
      setDeleteOpen(false);
      queryClient.removeQueries({ queryKey: vmQueryKey(uuid) });
      await queryClient.invalidateQueries({ queryKey: queryKeys.vms.root });
      await queryClient.invalidateQueries({ queryKey: overviewQueryKey });
      await navigate({ to: "/vms" });
    },
    onError: (error) => setDeleteError(error instanceof ApiError ? error.message : "删除虚拟机失败，请稍后重试。"),
  });

  const cloneMutation = useMutation({
    mutationFn: (input: { name: string; autostart: boolean; start: boolean }) => api.cloneVM(uuid, input),
    onSuccess: (task) => {
      setCloneTaskID(task.id);
      setCloneOpen(false);
      setCloneError("");
      void queryClient.invalidateQueries({ queryKey: queryKeys.tasks.root });
    },
    onError: (error) => setCloneError(error instanceof ApiError ? error.message : "创建克隆任务失败，请稍后重试。"),
  });

  const exportMutation = useMutation({
    mutationFn: () => api.exportVM(uuid),
    onSuccess: (exported) => {
      downloadVMExport(exported);
      setExportNotice({ tone: "success", text: `已导出 ${exported.vm.name} 的配置与元数据。` });
    },
    onError: (error) => setExportNotice({ tone: "error", text: error instanceof ApiError ? error.message : "导出虚拟机配置失败，请稍后重试。" }),
  });

  const xmlUpdateMutation = useMutation({
    mutationFn: (xmlText: string) => api.updateVMXML(uuid, { xml: xmlText }),
    onSuccess: (updated) => {
      queryClient.setQueryData(vmXMLQueryKey(uuid, "config"), updated);
      void queryClient.invalidateQueries({ queryKey: vmQueryKey(uuid) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.vms.root });
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
    void queryClient.invalidateQueries({ queryKey: queryKeys.vms.root });
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
          ? "仅支持关机或异常停止的虚拟机"
          : "";
  const backupDisabledReason = readOnly
    ? "系统当前处于只读模式"
    : viewer
      ? "当前账号没有虚拟机操作权限"
      : !vm.persistent
        ? "临时域不支持备份"
        : vm.state !== "shutoff" && vm.state !== "crashed" && vm.state !== "running"
          ? "仅支持关机、异常停止或运行中的虚拟机"
          : "";
  const allowedActions = actionOrder.filter((action) => vm.allowedActions.includes(action));
  const hasConsole = vm.graphics.some((graphics) => graphics.type.toLowerCase() === "vnc") || Boolean(vm.serials?.length);
  const consoleStateReady = ["running", "blocked", "paused"].includes(vm.state);
  const consoleReady = canMutate && consoleStateReady && hasConsole;
  const consoleReason = readOnly
    ? "只读模式下不能打开可交互控制台"
    : viewer
      ? "当前账号没有控制台权限"
      : !hasConsole
        ? "当前虚拟机没有配置图形或串口控制台"
        : !consoleStateReady
          ? "请先启动虚拟机"
          : "";
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
          <Button variant="outline" size="sm" data-testid="vm-password-open" title="修改或重置虚拟机内已有用户的密码" onClick={() => setPasswordOpen(true)}><KeyRound className="h-4 w-4" />修改客户机密码</Button>
          <Button variant="outline" size="sm" data-testid="vm-console-open" disabled={!consoleReady} title={consoleReady ? "打开 VNC 控制台" : consoleReason} onClick={() => void navigate({ to: "/console/vms/$uuid", params: { uuid } })}><MonitorPlay className="h-4 w-4" />打开控制台</Button>
          <Button variant="outline" size="icon-sm" aria-label="刷新虚拟机状态" title="刷新虚拟机状态" onClick={() => void vmQuery.refetch()} disabled={vmQuery.isFetching}><RefreshCw className={cn("h-4 w-4", vmQuery.isFetching && "animate-spin")} /></Button>
        </div>}
      />

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <StatusBadge state={vm.state} />
        <Badge variant={vm.persistent ? "outline" : "secondary"}>{vm.persistent ? "持久化域" : "临时域"}</Badge>
        {vm.autostart ? <Badge variant="success">开机自启动</Badge> : <Badge variant="secondary">未设置自启动</Badge>}
        {vm.agentConnected ? <Badge variant="success" data-testid="vm-agent-status">QGA 已连接</Badge> : null}
        <span className="text-xs text-muted-foreground">主机名 <span className="font-mono text-foreground" data-testid="vm-hostname">{vm.hostname || "-"}</span></span>
        <span className="text-xs text-muted-foreground">IP <span className="font-mono text-foreground" data-testid="vm-ip-addresses">{vm.ipAddresses?.length ? vm.ipAddresses.join(", ") : "-"}</span></span>
        <span className="text-xs text-muted-foreground">{vm.osType || "未知 OS"} · {vm.arch || "未知架构"} · {vm.machine || "未知机型"}</span>
      </div>

      {readOnly ? (
        <div className="mb-4 flex items-start gap-3 rounded-lg border border-warning/35 bg-warning/10 px-4 py-3 text-sm text-warning" data-testid="vm-read-only-notice" role="status">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <p>只读模式已开启，生命周期操作已禁用。管理员可在系统设置中关闭只读模式。</p>
        </div>
      ) : null}
      {viewer ? (
        <div className="mb-4 flex items-start gap-3 rounded-lg border border-warning/35 bg-warning/10 px-4 py-3 text-sm text-warning" role="status">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <p>当前账号角色为 Viewer，仅允许查看虚拟机状态和硬件信息。</p>
        </div>
      ) : null}

      <section className="mb-4 rounded-lg border bg-card px-3 py-2.5" aria-label="虚拟机电源操作">
        <div className="flex flex-wrap items-center gap-2">
          <span className="mr-1 text-xs font-medium text-muted-foreground">电源操作</span>
          {allowedActions.length ? allowedActions.map((action) => {
            const meta = actionMeta[action];
            const Icon = mutation.isPending && mutation.variables === action ? LoaderCircle : meta.icon;
            return (
              <Button key={action} variant={meta.destructive ? "destructive" : "outline"} size="sm" disabled={Boolean(unavailableReason)} title={unavailableReason || meta.description} data-testid={`vm-action-${action}`} onClick={() => requestAction(action)}>
                <Icon className={cn("h-4 w-4", mutation.isPending && mutation.variables === action && "animate-spin")} />
                {meta.label}
              </Button>
            );
          }) : <span className="text-sm text-muted-foreground">当前状态没有可用的电源操作。</span>}
        </div>
        {unavailableReason ? <p className="mt-2 text-xs text-muted-foreground">{unavailableReason}。</p> : null}
        {notice ? <p className={cn("mt-2 rounded-md border px-3 py-2 text-xs", notice.tone === "success" ? "border-success/30 bg-success/10 text-success" : "border-destructive/30 bg-destructive/5 text-destructive")} role="status">{notice.text}</p> : null}
      </section>

      <div className="mb-4 overflow-x-auto border-b" role="tablist" aria-label="虚拟机详情分类" data-testid="vm-tabs">
        <div className="flex min-w-max gap-1">
          {vmTabs.map((tab) => (
            <button key={tab.id} type="button" role="tab" id={`vm-tab-${tab.id}`} aria-selected={activeTab === tab.id} aria-controls={`vm-tabpanel-${tab.id}`} data-testid={`vm-tab-${tab.id}`} onClick={() => setActiveTab(tab.id)} className={cn("border-b-2 px-3 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring", activeTab === tab.id ? "border-primary text-foreground" : "border-transparent text-muted-foreground hover:text-foreground")}>
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      {activeTab === "overview" ? (
        <VMDetailOverviewTab
          vm={vm}
          uuid={uuid}
          canMutate={canMutate}
          consoleReason={consoleReason}
          metrics={metricsQuery.data}
          metricsLoading={metricsQuery.isLoading}
          metricsError={metricsQuery.isError ? (metricsQuery.error instanceof Error ? metricsQuery.error.message : "无法读取虚拟机资源指标") : undefined}
        />
      ) : null}
      {activeTab === "config" ? (
        <VMDetailConfigTab
          vm={vm}
          role={session.data?.user.role}
          readOnly={readOnly}
          canMutate={canMutate}
          resourcesDisabledReason={resourcesDisabledReason}
          disabledReason={snapshotDisabledReason}
          autostartNotice={autostartNotice}
          autostartDisabledReason={autostartDisabledReason}
          autostartPending={autostartMutation.isPending}
          onToggleAutostart={() => { setAutostartNotice(null); autostartMutation.mutate(!vm.autostart); }}
        />
      ) : null}
      {activeTab === "guest" ? (
        <div role="tabpanel" id="vm-tabpanel-guest" aria-labelledby="vm-tab-guest" data-testid="vm-tabpanel-guest">
          <VMGuestAgentCard vm={vm} canMutate={canMutate} disabledReason={snapshotDisabledReason} />
        </div>
      ) : null}
      {activeTab === "snapshots" ? (
        <VMDetailSnapshotsTab vm={vm} role={session.data?.user.role} readOnly={readOnly} canMutate={canMutate} disabledReason={snapshotDisabledReason} backupDisabledReason={backupDisabledReason} />
      ) : null}
      {activeTab === "advanced" ? (
        <VMDetailAdvancedTab
          vm={vm}
          canMutate={canMutate}
          exportNotice={exportNotice}
          exportPending={exportMutation.isPending}
          onExport={() => { setExportNotice(null); exportMutation.mutate(); }}
          onDelete={() => { setDeleteName(""); setDeleteError(""); setDeleteOpen(true); }}
          cloneProps={{
            vm,
            disabledReason: cloneDisabledReason,
            open: cloneOpen,
            name: cloneName,
            start: cloneStart,
            autostart: cloneAutostart,
            error: cloneError,
            pending: cloneMutation.isPending,
            task: cloneTaskQuery.data,
            onOpen: () => { setCloneName(`${vm.name}-clone`.slice(0, 64)); setCloneAutostart(false); setCloneStart(false); setCloneError(""); setCloneOpen((open) => !open); },
            onNameChange: setCloneName,
            onStartChange: setCloneStart,
            onAutostartChange: setCloneAutostart,
            onSubmit: (event) => { event.preventDefault(); setCloneError(""); cloneMutation.mutate({ name: cloneName.trim(), autostart: cloneAutostart, start: cloneStart }); },
            onCancel: () => { setCloneOpen(false); setCloneError(""); },
          }}
          xmlProps={{
            vmName: vm.name,
            uuid,
            role: session.data?.user.role,
            readOnly,
            format: xmlFormat,
            xml: xmlQuery.data,
            loading: xmlQuery.isLoading,
            error: xmlQuery.isError ? (xmlQuery.error instanceof Error ? xmlQuery.error.message : "无法读取域 XML") : undefined,
            fetching: xmlQuery.isFetching,
            editing: xmlEditing,
            draft: xmlDraft,
            copyNotice,
            editNotice: xmlEditNotice,
            editError: xmlEditError,
            canEdit: canEditXML,
            saving: xmlUpdateMutation.isPending,
            versionsOpen,
            versionsTotal: versionsQuery.data?.total,
            currentXml: persistentXMLQuery.data?.xml,
            onFormatChange: (format) => { setXmlFormat(format); setCopyNotice(""); setXmlEditNotice(""); setXmlEditError(""); },
            onCopy: () => void copyXML(),
            onBeginEdit: beginXMLEdit,
            onCancel: cancelXMLEdit,
            onDraftChange: (value) => { setXmlDraft(value); setXmlEditError(""); },
            onRequestSave: requestSaveXML,
            onRetry: () => void xmlQuery.refetch(),
            onVersionsOpenChange: setVersionsOpen,
            onRolledBack: (version) => {
              setXmlFormat("config");
              setXmlEditing(false);
              setXmlDraft("");
              setXmlEditError("");
              setXmlEditNotice(`持久配置已回滚到历史版本，并归档为 v${version.versionNo}；运行中的域需重启后生效。`);
            },
          }}
        />
      ) : null}

      <VMPasswordDialog vm={vm} open={passwordOpen} onOpenChange={setPasswordOpen} canMutate={canMutate} disabledReason={snapshotDisabledReason} onOpenGuest={() => setActiveTab("guest")} />

      <VMXMLSaveDialog vmName={vm.name} open={xmlConfirmOpen} pending={xmlUpdateMutation.isPending} onOpenChange={setXmlConfirmOpen} onConfirm={() => xmlUpdateMutation.mutate(xmlDraft)} />

      <VMActionConfirmDialog
        action={confirmation}
        pending={mutation.isPending}
        onOpenChange={(open) => { if (!open) setConfirmation(null); }}
        onConfirm={(action) => { setConfirmation(null); submitAction(action); }}
      />

      <VMDeleteDialog
        vmName={vm.name}
        open={deleteOpen}
        name={deleteName}
        error={deleteError}
        pending={deleteMutation.isPending}
        onOpenChange={(open) => { if (!open) { setDeleteOpen(false); setDeleteError(""); } }}
        onNameChange={setDeleteName}
        onConfirm={() => { setDeleteError(""); deleteMutation.mutate(); }}
      />
    </>
  );
}

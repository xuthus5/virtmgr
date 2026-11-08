import { useDeferredValue, useEffect, useState } from "react";
import { Link } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, LoaderCircle, Play, Plus, Search } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { VMAction, VMState } from "@/api/types";
import { StatusBadge, stateLabel } from "@/components/feedback/status-badge";
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
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useSessionQuery } from "@/features/auth/use-session";
import { overviewQueryKey } from "@/features/overview/use-overview-query";
import { useVMsQuery } from "@/features/vms/use-vms-query";
import { formatBytes } from "@/lib/utils";

const states: Array<{ value: "" | VMState; label: string }> = [
  { value: "", label: "全部状态" },
  { value: "running", label: stateLabel("running") },
  { value: "paused", label: stateLabel("paused") },
  { value: "shutoff", label: stateLabel("shutoff") },
  { value: "crashed", label: stateLabel("crashed") },
];

const batchActions: Array<{ value: VMAction; label: string; confirm?: boolean }> = [
  { value: "start", label: "启动" },
  { value: "shutdown", label: "正常关机" },
  { value: "reboot", label: "重启" },
  { value: "reset", label: "强制重启", confirm: true },
  { value: "pause", label: "暂停" },
  { value: "resume", label: "恢复" },
  { value: "destroy", label: "强制下电", confirm: true },
];

type BatchNotice = {
  tone: "success" | "error";
  text: string;
  failures: string[];
};

function batchActionLabel(action: VMAction) {
  return batchActions.find((item) => item.value === action)?.label ?? action;
}

export function VMsPage() {
  const [search, setSearch] = useState("");
  const [state, setState] = useState("" as "" | VMState);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [batchAction, setBatchAction] = useState<VMAction>("start");
  const [confirmAction, setConfirmAction] = useState<VMAction | null>(null);
  const [batchNotice, setBatchNotice] = useState<BatchNotice | null>(null);
  const deferredSearch = useDeferredValue(search);
  const vms = useVMsQuery(deferredSearch, state);
  const session = useSessionQuery();
  const queryClient = useQueryClient();
  const readOnly = Boolean(session.data?.readOnly);
  const canOperate = Boolean(session.data) && !session.data?.readOnly && session.data?.user.role !== "viewer";
  const disabledReason = session.data?.readOnly ? "系统当前处于只读模式" : "当前账号没有创建虚拟机权限";
  const items = vms.data?.items ?? [];
  const allVisibleSelected = items.length > 0 && items.every((vm) => selected.has(vm.uuid));

  useEffect(() => {
    const available = new Set(items.map((vm) => vm.uuid));
    setSelected((current) => {
      const next = new Set([...current].filter((uuid) => available.has(uuid)));
      return next.size === current.size ? current : next;
    });
  }, [vms.data]);

  const batchMutation = useMutation({
    mutationFn: ({ action, uuids }: { action: VMAction; uuids: string[] }) => api.batchVMAction({ action, uuids }),
    onSuccess: (result) => {
      const failures = result.items
        .filter((item) => !item.success)
        .map((item) => `${item.name || item.uuid}: ${item.error || "操作失败"}`);
      setBatchNotice({
        tone: result.failed ? "error" : "success",
        text: `批量${batchActionLabel(result.action)}完成：成功 ${result.succeeded} 台，失败 ${result.failed} 台。`,
        failures,
      });
      setSelected(new Set());
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
      void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
    },
    onError: (error) => {
      setBatchNotice({
        tone: "error",
        text: error instanceof ApiError ? error.message : "批量操作失败，请稍后重试。",
        failures: [],
      });
    },
  });

  function toggleSelected(uuid: string) {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(uuid)) next.delete(uuid);
      else next.add(uuid);
      return next;
    });
  }

  function toggleAll() {
    setSelected((current) => {
      const next = new Set(current);
      if (allVisibleSelected) items.forEach((vm) => next.delete(vm.uuid));
      else items.forEach((vm) => next.add(vm.uuid));
      return next;
    });
  }

  function executeBatch(action: VMAction) {
    if (!selected.size) return;
    setBatchNotice(null);
    batchMutation.mutate({ action, uuids: [...selected] });
  }

  function requestBatch() {
    if (!selected.size) return;
    const action = batchActions.find((item) => item.value === batchAction);
    if (action?.confirm) {
      setConfirmAction(batchAction);
      return;
    }
    executeBatch(batchAction);
  }

  return (
    <>
      <PageHeader
        title="虚拟机"
        breadcrumb="总览 / 虚拟机"
        description="查看由当前 libvirt 主机管理的全部域。"
        actions={canOperate
          ? <Button asChild><Link to="/vms/new"><Plus className="h-4 w-4" />新建虚拟机</Link></Button>
          : <Button disabled title={disabledReason}><Plus className="h-4 w-4" />新建虚拟机</Button>}
      />
      <Card>
        <CardContent className="p-0">
          <div className="flex flex-col gap-3 border-b p-4 md:flex-row md:items-center">
            <div className="relative min-w-0 flex-1 md:max-w-sm">
              <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input aria-label="搜索虚拟机" placeholder="搜索名称或 UUID" className="pl-9" value={search} onChange={(event) => setSearch(event.target.value)} />
            </div>
            <select aria-label="状态筛选" value={state} onChange={(event) => setState(event.target.value as "" | VMState)} className="h-9 rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
              {states.map((item) => <option key={item.value || "all"} value={item.value}>{item.label}</option>)}
            </select>
            <p className="text-xs text-muted-foreground md:ml-auto" data-testid="vm-total-label">共 {vms.data?.total ?? 0} 台虚拟机</p>
          </div>

          {canOperate ? (
            <div className="border-b bg-muted/25 p-3" data-testid="vm-batch-toolbar">
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
                <p className="text-sm md:mr-auto" data-testid="vm-batch-selected">
                  已选择 <span className="font-semibold tabular-nums">{selected.size}</span> 台虚拟机
                  {selected.size === 0 ? <span className="ml-2 text-xs text-muted-foreground">勾选后可批量执行生命周期操作</span> : null}
                </p>
                <select
                  aria-label="批量动作"
                  data-testid="vm-batch-action"
                  value={batchAction}
                  onChange={(event) => setBatchAction(event.target.value as VMAction)}
                  disabled={!selected.size || batchMutation.isPending}
                  className="h-9 rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                >
                  {batchActions.map((action) => <option key={action.value} value={action.value}>{action.label}</option>)}
                </select>
                <Button
                  size="sm"
                  variant="outline"
                  data-testid="vm-batch-submit"
                  disabled={!selected.size || batchMutation.isPending}
                  onClick={requestBatch}
                >
                  {batchMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
                  {batchMutation.isPending ? "执行中" : "执行批量操作"}
                </Button>
              </div>
              {batchNotice ? (
                <div
                  role="status"
                  data-testid="vm-batch-result"
                  className={batchNotice.tone === "error" ? "mt-3 rounded-md border border-warning/35 bg-warning/10 px-3 py-2 text-sm text-warning" : "mt-3 rounded-md border border-success/35 bg-success/10 px-3 py-2 text-sm text-success"}
                >
                  <p>{batchNotice.text}</p>
                  {batchNotice.failures.length ? <ul className="mt-1 space-y-0.5 text-xs">{batchNotice.failures.map((failure) => <li key={failure}>{failure}</li>)}</ul> : null}
                </div>
              ) : null}
            </div>
          ) : null}

          {readOnly ? (
            <div className="flex items-start gap-3 border-b border-warning/35 bg-warning/10 px-4 py-3 text-sm text-warning" data-testid="vm-read-only-notice" role="status">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <p>只读模式已开启，批量操作和新虚拟机创建已禁用。管理员可在系统设置中关闭只读模式。</p>
            </div>
          ) : null}

          {vms.isLoading ? <LoadingState label="正在加载虚拟机" /> : null}
          {vms.isError ? <ErrorState message={vms.error instanceof Error ? vms.error.message : "无法读取虚拟机"} onRetry={() => void vms.refetch()} className="m-4" /> : null}
          {vms.data ? (
            <Table
              aria-label="虚拟机列表"
              className="min-w-[1080px]"
              containerProps={{
                role: "region",
                tabIndex: 0,
                "aria-label": "虚拟机列表，可横向滚动",
                "data-testid": "vm-table-scroll",
                className: "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring",
              }}
            >
                <TableHeader>
                  <TableRow>
                    <TableHead className="sticky left-0 z-20 min-w-52 bg-card">
                      <span className="flex items-center gap-2">
                        {canOperate ? <input type="checkbox" aria-label="选择全部虚拟机" data-testid="vm-select-all" checked={allVisibleSelected} onChange={toggleAll} disabled={!items.length} className="h-4 w-4 accent-primary" /> : null}
                        名称
                      </span>
                    </TableHead>
                    <TableHead>状态</TableHead><TableHead>vCPU</TableHead><TableHead>内存</TableHead><TableHead>IP</TableHead><TableHead>磁盘</TableHead><TableHead>网络</TableHead><TableHead>自动启动</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.length ? items.map((vm) => (
                    <TableRow key={vm.uuid} className="group">
                      <TableCell className="sticky left-0 z-10 min-w-52 bg-card group-hover:bg-muted/35" data-testid={`vm-name-cell-${vm.uuid}`}>
                        <div className="flex items-start gap-2">
                          {canOperate ? <input type="checkbox" aria-label={`选择 ${vm.name}`} data-testid={`vm-select-${vm.uuid}`} checked={selected.has(vm.uuid)} onChange={() => toggleSelected(vm.uuid)} className="mt-0.5 h-4 w-4 shrink-0 accent-primary" /> : null}
                          <div className="min-w-0">
                            <Link to="/vms/$uuid" params={{ uuid: vm.uuid }} className="font-medium text-primary hover:underline">{vm.name}</Link>
                            <p className="truncate font-mono text-[10px] text-muted-foreground">{vm.uuid}</p>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell><StatusBadge state={vm.state} /></TableCell>
                      <TableCell className="tabular-nums">{vm.vcpus}</TableCell>
                      <TableCell className="tabular-nums">{formatBytes(vm.memoryBytes)}</TableCell>
                      <TableCell><span className="font-mono text-xs text-muted-foreground">{vm.ipAddresses?.length ? vm.ipAddresses.join(", ") : "-"}</span></TableCell>
                      <TableCell className="text-muted-foreground">{vm.disks.length ? `${vm.disks[0].target} · ${vm.disks[0].format ?? vm.disks[0].device}` : "-"}</TableCell>
                      <TableCell className="text-muted-foreground">{vm.interfaces[0]?.source ?? "-"}</TableCell>
                      <TableCell>{vm.autostart ? "是" : "否"}</TableCell>
                    </TableRow>
                  )) : (
                    <TableRow><TableCell colSpan={8} className="h-32 text-center text-muted-foreground">没有匹配的虚拟机</TableCell></TableRow>
                  )}
                </TableBody>
            </Table>
          ) : null}
        </CardContent>
      </Card>

      <AlertDialog open={confirmAction !== null} onOpenChange={(open) => { if (!open) setConfirmAction(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认批量{batchActionLabel(confirmAction ?? "destroy")}？</AlertDialogTitle>
            <AlertDialogDescription>
              将对已选择的 {selected.size} 台虚拟机执行该操作。危险动作可能丢失客户机未保存的数据，且每台虚拟机仍会单独校验状态。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction data-testid="vm-batch-confirm" onClick={() => { const action = confirmAction; setConfirmAction(null); if (action) executeBatch(action); }}>
              确认执行
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

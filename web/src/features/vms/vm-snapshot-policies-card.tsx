import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CalendarClock, CheckCircle2, LoaderCircle, Pause, Pencil, Play, Plus, Trash2 } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { SnapshotPolicy, SnapshotPolicyInput } from "@/api/types";
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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
  formatSnapshotPolicyTime,
  initialSnapshotPolicyForm,
  snapshotPolicyFormFromPolicy,
  snapshotPolicyInputFromForm,
  type SnapshotPolicyFormValues,
} from "@/features/vms/vm-snapshot-policy-model";
import {
  snapshotPoliciesQueryKey,
  useSnapshotPoliciesQuery,
  vmSnapshotsQueryKey,
} from "@/features/vms/use-vm-query";

type SaveVariables = {
  editing: SnapshotPolicy | null;
  input: SnapshotPolicyInput;
};

export function VMSnapshotPoliciesCard({ uuid, canMutate, disabledReason }: {
  uuid: string;
  canMutate: boolean;
  disabledReason: string;
}) {
  const queryClient = useQueryClient();
  const policiesQuery = useSnapshotPoliciesQuery(uuid);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<SnapshotPolicy | null>(null);
  const [form, setForm] = useState<SnapshotPolicyFormValues>(initialSnapshotPolicyForm);
  const [formError, setFormError] = useState("");
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<SnapshotPolicy | null>(null);

  function invalidatePolicies() {
    void queryClient.invalidateQueries({ queryKey: snapshotPoliciesQueryKey(uuid) });
  }

  const saveMutation = useMutation({
    mutationFn: ({ editing: target, input }: SaveVariables) => target
      ? api.updateSnapshotPolicy(target.id, input)
      : api.createSnapshotPolicy(uuid, input),
    onSuccess: (policy, variables) => {
      setNotice({
        tone: "success",
        text: variables.editing ? `策略 ${policy.name} 已更新。` : `策略 ${policy.name} 已创建。`,
      });
      setFormError("");
      setFormOpen(false);
      setEditing(null);
      setForm(initialSnapshotPolicyForm());
      invalidatePolicies();
    },
    onError: (cause) => {
      setFormError(cause instanceof ApiError ? cause.message : "保存快照策略失败，请稍后重试。");
    },
  });

  const toggleMutation = useMutation({
    mutationFn: (policy: SnapshotPolicy) => api.updateSnapshotPolicy(policy.id, {
      name: policy.name,
      intervalMinutes: policy.intervalMinutes,
      scheduleMode: policy.scheduleMode ?? "interval",
      dailyTime: policy.dailyTime || "02:00",
      retention: policy.retention,
      enabled: !policy.enabled,
    }),
    onSuccess: (policy) => {
      setNotice({ tone: "success", text: `策略 ${policy.name} 已${policy.enabled ? "启用" : "禁用"}。` });
      invalidatePolicies();
    },
    onError: (cause) => {
      setNotice({ tone: "error", text: cause instanceof ApiError ? cause.message : "切换策略状态失败，请稍后重试。" });
    },
  });

  const runMutation = useMutation({
    mutationFn: (policy: SnapshotPolicy) => api.runSnapshotPolicy(policy.id),
    onSuccess: (result) => {
      const snapshotName = result.snapshot?.name || result.policy.lastSnapshot || "已创建";
      setNotice({ tone: "success", text: `策略 ${result.policy.name} 已执行，快照 ${snapshotName} 已创建。` });
      invalidatePolicies();
      void queryClient.invalidateQueries({ queryKey: vmSnapshotsQueryKey(uuid) });
    },
    onError: (cause) => {
      setNotice({ tone: "error", text: cause instanceof ApiError ? cause.message : "执行快照策略失败，请稍后重试。" });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (policy: SnapshotPolicy) => api.deleteSnapshotPolicy(policy.id),
    onSuccess: (_result, policy) => {
      setNotice({ tone: "success", text: `策略 ${policy.name} 已删除。` });
      setDeleteTarget(null);
      invalidatePolicies();
    },
    onError: (cause) => {
      setNotice({ tone: "error", text: cause instanceof ApiError ? cause.message : "删除快照策略失败，请稍后重试。" });
    },
  });

  function startCreate() {
    setEditing(null);
    setForm(initialSnapshotPolicyForm());
    setFormError("");
    setNotice(null);
    setFormOpen(true);
  }

  function startEdit(policy: SnapshotPolicy) {
    setEditing(policy);
    setForm(snapshotPolicyFormFromPolicy(policy));
    setFormError("");
    setNotice(null);
    setFormOpen(true);
  }

  function submitForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canMutate) return;
    const parsed = snapshotPolicyInputFromForm(form);
    if (!parsed.input) {
      setFormError(parsed.error);
      return;
    }
    setFormError("");
    setNotice(null);
    saveMutation.mutate({ editing, input: parsed.input });
  }

  const policies = policiesQuery.data?.items ?? [];

  return (
    <>
      <Card className="mt-4" data-testid="vm-snapshot-policy-card">
        <CardHeader>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <CardTitle className="flex items-center gap-2"><CalendarClock className="h-4 w-4 text-primary" />定时快照策略</CardTitle>
              <CardDescription>按固定间隔自动创建 libvirt 快照，并按保留数量清理本策略创建的历史快照。</CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Badge variant="outline" data-testid="vm-snapshot-policy-total">{policies.length} 个策略</Badge>
              <Button
                type="button"
                size="sm"
                variant="outline"
                data-testid="vm-snapshot-policy-create-toggle"
                disabled={!canMutate || saveMutation.isPending}
                title={canMutate ? "创建定时快照策略" : disabledReason}
                onClick={startCreate}
              >
                <Plus className="h-4 w-4" />创建策略
              </Button>
            </div>
          </div>
        </CardHeader>

        {canMutate ? null : (
          <div className="border-y bg-muted/20 px-4 py-2 text-xs text-muted-foreground" data-testid="vm-snapshot-policy-readonly">
            {disabledReason}，快照策略变更已禁用。
          </div>
        )}

        {formOpen && canMutate ? (
          <form onSubmit={submitForm} className="border-y bg-muted/20 p-4" data-testid="vm-snapshot-policy-form">
            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
              <div className="space-y-2">
                <Label htmlFor="vm-snapshot-policy-name">策略名称</Label>
                <Input
                  id="vm-snapshot-policy-name"
                  value={form.name}
                  onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
                  placeholder="例如 nightly"
                  autoComplete="off"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="vm-snapshot-policy-mode">调度方式</Label>
                <select id="vm-snapshot-policy-mode" value={form.scheduleMode} onChange={(event) => setForm((current) => ({ ...current, scheduleMode: event.target.value === "daily" ? "daily" : "interval" }))} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                  <option value="interval">固定间隔</option>
                  <option value="daily">每日时刻</option>
                </select>
              </div>
              <div className="space-y-2">
                {form.scheduleMode === "daily" ? <>
                  <Label htmlFor="vm-snapshot-policy-daily-time">每日执行时间</Label>
                  <Input id="vm-snapshot-policy-daily-time" type="time" value={form.dailyTime} onChange={(event) => setForm((current) => ({ ...current, dailyTime: event.target.value }))} />
                </> : <>
                  <Label htmlFor="vm-snapshot-policy-interval">间隔（分钟）</Label>
                  <Input
                    id="vm-snapshot-policy-interval"
                    type="number"
                    min={5}
                    max={10080}
                    value={form.intervalMinutes}
                    onChange={(event) => setForm((current) => ({ ...current, intervalMinutes: event.target.value }))}
                  />
                </>}
              </div>
              <div className="space-y-2">
                <Label htmlFor="vm-snapshot-policy-retention">保留快照数</Label>
                <Input
                  id="vm-snapshot-policy-retention"
                  type="number"
                  min={1}
                  max={50}
                  value={form.retention}
                  onChange={(event) => setForm((current) => ({ ...current, retention: event.target.value }))}
                />
              </div>
            </div>
            <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
              <label htmlFor="vm-snapshot-policy-enabled" className="inline-flex items-center gap-2 text-sm">
                <input
                  id="vm-snapshot-policy-enabled"
                  type="checkbox"
                  checked={form.enabled}
                  onChange={(event) => setForm((current) => ({ ...current, enabled: event.target.checked }))}
                  className="h-4 w-4 rounded border-input accent-primary"
                />
                启用策略
              </label>
              <div className="flex items-center gap-2">
                <Button type="button" variant="ghost" size="sm" onClick={() => { setFormOpen(false); setEditing(null); setFormError(""); }}>取消</Button>
                <Button type="submit" size="sm" data-testid="vm-snapshot-policy-submit" disabled={saveMutation.isPending}>
                  {saveMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <CalendarClock className="h-4 w-4" />}
                  {saveMutation.isPending ? "保存中..." : editing ? "保存修改" : "创建策略"}
                </Button>
              </div>
            </div>
            {formError ? (
              <p role="alert" className="mt-3 flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" data-testid="vm-snapshot-policy-form-error">
                <AlertTriangle className="h-3.5 w-3.5" />{formError}
              </p>
            ) : null}
          </form>
        ) : null}

        {notice ? (
          <div className={`border-b px-4 py-2 text-xs ${notice.tone === "success" ? "bg-success/10 text-success" : "bg-destructive/5 text-destructive"}`} role="status" data-testid="vm-snapshot-policy-notice">
            <span className="inline-flex items-center gap-2">{notice.tone === "success" ? <CheckCircle2 className="h-3.5 w-3.5" /> : <AlertTriangle className="h-3.5 w-3.5" />}{notice.text}</span>
          </div>
        ) : null}

        <CardContent className="p-0">
          {policiesQuery.isLoading ? <LoadingState label="正在读取快照策略" /> : null}
          {policiesQuery.isError ? <ErrorState message={policiesQuery.error instanceof Error ? policiesQuery.error.message : "无法读取快照策略"} onRetry={() => void policiesQuery.refetch()} className="m-4" /> : null}
          {policiesQuery.data && policies.length === 0 ? (
            <p className="p-5 text-sm text-muted-foreground" data-testid="vm-snapshot-policy-empty">暂无快照策略。创建策略后由服务端每分钟检查到期任务。</p>
          ) : null}
          {policies.length ? (
            <Table className="min-w-[960px]">
              <TableHeader><TableRow><TableHead>策略</TableHead><TableHead>间隔</TableHead><TableHead>保留</TableHead><TableHead>状态</TableHead><TableHead>最近执行</TableHead><TableHead>下次执行</TableHead><TableHead>最近快照</TableHead><TableHead>操作</TableHead></TableRow></TableHeader>
              <TableBody>
                {policies.map((policy) => (
                  <TableRow key={policy.id} data-testid={`vm-snapshot-policy-row-${policy.name}`}>
                    <TableCell>
                      <p className="font-medium">{policy.name}</p>
                      <p className="font-mono text-[10px] text-muted-foreground">#{policy.id}</p>
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-xs">{policy.scheduleMode === "daily" ? `每日 ${policy.dailyTime || "02:00"}` : `${policy.intervalMinutes} 分钟`}</TableCell>
                    <TableCell className="whitespace-nowrap text-xs">{policy.retention} 份</TableCell>
                    <TableCell>{policy.enabled ? <Badge variant="success">已启用</Badge> : <Badge variant="secondary">已禁用</Badge>}</TableCell>
                    <TableCell className="whitespace-nowrap text-xs text-muted-foreground">{formatSnapshotPolicyTime(policy.lastRunAt)}</TableCell>
                    <TableCell className="whitespace-nowrap text-xs text-muted-foreground">{formatSnapshotPolicyTime(policy.nextRunAt)}</TableCell>
                    <TableCell className="max-w-56 text-xs">
                      <p className="truncate font-mono" title={policy.lastSnapshot}>{policy.lastSnapshot || "-"}</p>
                      {policy.lastError ? <p className="mt-1 truncate text-destructive" title={policy.lastError}>{policy.lastError}</p> : null}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1">
                        <Button type="button" variant="ghost" size="icon-sm" data-testid={`vm-snapshot-policy-run-${policy.name}`} aria-label={`立即执行策略 ${policy.name}`} disabled={!canMutate || runMutation.isPending} title={canMutate ? "立即创建一次快照" : disabledReason} onClick={() => { setNotice(null); runMutation.mutate(policy); }}>
                          {runMutation.isPending && runMutation.variables?.id === policy.id ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
                        </Button>
                        <Button type="button" variant="ghost" size="icon-sm" data-testid={`vm-snapshot-policy-toggle-${policy.name}`} aria-label={`${policy.enabled ? "禁用" : "启用"}策略 ${policy.name}`} disabled={!canMutate || toggleMutation.isPending} title={canMutate ? (policy.enabled ? "禁用策略" : "启用策略") : disabledReason} onClick={() => { setNotice(null); toggleMutation.mutate(policy); }}>
                          {policy.enabled ? <Pause className="h-4 w-4" /> : <Play className="h-4 w-4" />}
                        </Button>
                        <Button type="button" variant="ghost" size="icon-sm" data-testid={`vm-snapshot-policy-edit-${policy.name}`} aria-label={`编辑策略 ${policy.name}`} disabled={!canMutate || saveMutation.isPending} title={canMutate ? "编辑策略" : disabledReason} onClick={() => startEdit(policy)}>
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button type="button" variant="ghost" size="icon-sm" data-testid={`vm-snapshot-policy-delete-${policy.name}`} aria-label={`删除策略 ${policy.name}`} disabled={!canMutate || deleteMutation.isPending} title={canMutate ? "删除策略" : disabledReason} onClick={() => { setNotice(null); setDeleteTarget(policy); }}>
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : null}
        </CardContent>
      </Card>

      <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => { if (!open) setDeleteTarget(null); }}>
        <AlertDialogContent data-testid="vm-snapshot-policy-delete-dialog">
          <AlertDialogHeader>
            <AlertDialogTitle>删除快照策略？</AlertDialogTitle>
            <AlertDialogDescription>
              将停止策略 <span className="font-medium text-foreground">{deleteTarget?.name}</span> 的后续调度，已创建的快照不会被删除。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <Button variant="destructive" data-testid="vm-snapshot-policy-confirm-delete" disabled={deleteMutation.isPending} onClick={() => { if (deleteTarget) deleteMutation.mutate(deleteTarget); }}>
              {deleteMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
              确认删除
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

import { useEffect, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BellRing, LoaderCircle, Pencil, Plus, Trash2 } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { AlertMetric, AlertOperator, AlertRule, AlertRuleInput } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Modal } from "@/components/ui/modal";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useSessionQuery } from "@/features/auth/use-session";

const metricLabels: Record<AlertMetric, string> = {
  host_cpu_percent: "宿主机 CPU 使用率",
  host_memory_percent: "宿主机内存使用率",
  vm_cpu_percent: "虚拟机 CPU 使用率",
  vm_memory_percent: "虚拟机内存使用率",
};

const operatorLabels: Record<AlertOperator, string> = {
  gt: "大于",
  gte: "大于等于",
  lt: "小于",
  lte: "小于等于",
};

const stateLabels = { ok: "正常", pending: "等待持续时间", firing: "告警中" } as const;
const stateVariants = { ok: "success", pending: "warning", firing: "destructive" } as const;

function initialForm(): AlertRuleInput {
  return { name: "", metric: "host_cpu_percent", operator: "gt", threshold: 90, durationMinutes: 5, enabled: true, webhookUrl: "" };
}

function formFromRule(rule: AlertRule): AlertRuleInput {
  return { name: rule.name, metric: rule.metric, operator: rule.operator, threshold: rule.threshold, durationMinutes: rule.durationMinutes, enabled: rule.enabled, webhookUrl: rule.webhookUrl };
}

export function AlertRulesCard() {
  const session = useSessionQuery();
  const queryClient = useQueryClient();
  const query = useQuery({ queryKey: ["alert-rules"], queryFn: api.alertRules, staleTime: 15_000 });
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<AlertRule | null>(null);
  const [form, setForm] = useState<AlertRuleInput>(initialForm);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<AlertRule | null>(null);
  const admin = session.data?.user.role === "admin";
  const rules = query.data?.items ?? [];

  useEffect(() => {
    if (!open) return;
    setForm(editing ? formFromRule(editing) : initialForm());
    setError("");
  }, [editing, open]);

  function refresh() {
    void queryClient.invalidateQueries({ queryKey: ["alert-rules"] });
  }

  const saveMutation = useMutation({
    mutationFn: () => editing ? api.updateAlertRule(editing.id, form) : api.createAlertRule(form),
    onSuccess: (rule) => {
      setOpen(false);
      setEditing(null);
      setNotice({ tone: "success", text: `告警规则 ${rule.name} 已保存。` });
      refresh();
    },
    onError: (cause) => setError(cause instanceof ApiError ? cause.message : "保存告警规则失败，请稍后重试。"),
  });

  const deleteMutation = useMutation({
    mutationFn: (rule: AlertRule) => api.deleteAlertRule(rule.id),
    onSuccess: (_result, rule) => {
      setDeleteTarget(null);
      setNotice({ tone: "success", text: `告警规则 ${rule.name} 已删除。` });
      refresh();
    },
    onError: (cause) => setNotice({ tone: "error", text: cause instanceof ApiError ? cause.message : "删除告警规则失败，请稍后重试。" }),
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!form.name.trim()) {
      setError("请填写规则名称。");
      return;
    }
    if (!Number.isFinite(form.threshold)) {
      setError("阈值必须是数字。");
      return;
    }
    setError("");
    saveMutation.mutate();
  }

  return (
    <Card data-testid="host-alert-rules">
      <CardHeader>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div><CardTitle className="flex items-center gap-2"><BellRing className="h-4 w-4 text-primary" />基础告警</CardTitle><CardDescription>按宿主机和虚拟机 CPU/内存阈值触发，可配置持续时间与 Webhook。仅 Admin 可管理规则。</CardDescription></div>
          <Button size="sm" disabled={!admin} title={admin ? "新建告警规则" : "只有管理员可以管理告警规则"} onClick={() => { setEditing(null); setOpen(true); }}><Plus className="h-4 w-4" />新建规则</Button>
        </div>
      </CardHeader>
      <CardContent className="p-0">
        {notice ? <p role="status" className={notice.tone === "success" ? "border-b border-success/30 bg-success/10 px-4 py-2 text-sm text-success" : "border-b border-destructive/30 bg-destructive/5 px-4 py-2 text-sm text-destructive"}>{notice.text}</p> : null}
        {query.isLoading ? <p className="flex items-center gap-2 p-5 text-sm text-muted-foreground"><LoaderCircle className="h-4 w-4 animate-spin" />正在读取告警规则…</p> : null}
        {query.isError ? <p role="alert" className="p-4 text-sm text-destructive">{query.error instanceof Error ? query.error.message : "无法读取告警规则"}</p> : null}
        {!query.isLoading && !query.isError && rules.length === 0 ? <p className="p-5 text-sm text-muted-foreground">还没有告警规则。建议先为宿主机 CPU 和内存设置 90% 阈值。</p> : null}
        {rules.length ? <Table className="min-w-[860px]"><TableHeader><TableRow><TableHead>规则</TableHead><TableHead>条件</TableHead><TableHead>持续时间</TableHead><TableHead>状态</TableHead><TableHead>Webhook</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader><TableBody>{rules.map((rule) => (
          <TableRow key={rule.id} data-testid={`alert-rule-row-${rule.id}`}>
            <TableCell><p className="font-medium">{rule.name}</p><p className="text-[10px] text-muted-foreground">{rule.enabled ? "已启用" : "已禁用"}</p></TableCell>
            <TableCell className="text-xs">{metricLabels[rule.metric]} {operatorLabels[rule.operator]} <span className="font-mono font-medium">{rule.threshold}</span></TableCell>
            <TableCell className="text-xs">{rule.durationMinutes ? `${rule.durationMinutes} 分钟` : "立即"}</TableCell>
            <TableCell><Badge variant={stateVariants[rule.lastState]}>{stateLabels[rule.lastState]}</Badge></TableCell>
            <TableCell className="max-w-48 truncate font-mono text-[10px] text-muted-foreground" title={rule.webhookUrl}>{rule.webhookUrl || "-"}</TableCell>
            <TableCell className="text-right"><div className="flex justify-end gap-1">
              <Button variant="ghost" size="icon-sm" aria-label={`编辑规则 ${rule.name}`} disabled={!admin} onClick={() => { setEditing(rule); setOpen(true); }}><Pencil className="h-4 w-4" /></Button>
              <Button variant="ghost" size="icon-sm" aria-label={`删除规则 ${rule.name}`} disabled={!admin || deleteMutation.isPending} onClick={() => setDeleteTarget(rule)}><Trash2 className="h-4 w-4" /></Button>
            </div></TableCell>
          </TableRow>
        ))}</TableBody></Table> : null}
      </CardContent>

      <Modal open={open} onClose={() => { if (!saveMutation.isPending) setOpen(false); }} title={editing ? "编辑告警规则" : "新建告警规则"} description="条件持续达到设定时间后触发；恢复后发送恢复通知并写入审计日志。" testId="alert-rule-modal" footer={<><Button variant="ghost" disabled={saveMutation.isPending} onClick={() => setOpen(false)}>取消</Button><Button form="alert-rule-form" type="submit" disabled={saveMutation.isPending}>{saveMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : null}保存规则</Button></>}>
        <form id="alert-rule-form" className="space-y-4" onSubmit={submit}>
          <div className="space-y-2"><Label htmlFor="alert-rule-name">规则名称</Label><Input id="alert-rule-name" data-testid="alert-rule-name" value={form.name} maxLength={64} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="例如 host-cpu-high" /></div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-2"><Label htmlFor="alert-rule-metric">指标</Label><select id="alert-rule-metric" value={form.metric} onChange={(event) => setForm({ ...form, metric: event.target.value as AlertMetric })} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">{Object.entries(metricLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></div>
            <div className="space-y-2"><Label htmlFor="alert-rule-operator">条件</Label><select id="alert-rule-operator" value={form.operator} onChange={(event) => setForm({ ...form, operator: event.target.value as AlertOperator })} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">{Object.entries(operatorLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></div>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-2"><Label htmlFor="alert-rule-threshold">阈值（百分比）</Label><Input id="alert-rule-threshold" data-testid="alert-rule-threshold" type="number" min={0} max={100} step={0.1} value={form.threshold} onChange={(event) => setForm({ ...form, threshold: Number(event.target.value) })} /></div>
            <div className="space-y-2"><Label htmlFor="alert-rule-duration">持续时间（分钟）</Label><Input id="alert-rule-duration" type="number" min={0} max={1440} value={form.durationMinutes} onChange={(event) => setForm({ ...form, durationMinutes: Number(event.target.value) })} /></div>
          </div>
          <div className="space-y-2"><Label htmlFor="alert-rule-webhook">Webhook URL（可选）</Label><Input id="alert-rule-webhook" value={form.webhookUrl} onChange={(event) => setForm({ ...form, webhookUrl: event.target.value })} placeholder="https://hooks.example.com/virtmgr" /></div>
          <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.enabled} onChange={(event) => setForm({ ...form, enabled: event.target.checked })} className="h-4 w-4 accent-primary" />启用规则</label>
          {error ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{error}</p> : null}
        </form>
      </Modal>

      <Modal open={deleteTarget !== null} onClose={() => setDeleteTarget(null)} title="删除告警规则" description="删除后不再评估该规则，历史审计记录会保留。" testId="alert-rule-delete-modal" footer={<><Button variant="ghost" onClick={() => setDeleteTarget(null)}>取消</Button><Button variant="destructive" disabled={deleteMutation.isPending} onClick={() => { if (deleteTarget) deleteMutation.mutate(deleteTarget); }}>{deleteMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}确认删除</Button></>}>
        <p className="text-sm">确认删除规则 <span className="font-medium">{deleteTarget?.name}</span>？</p>
      </Modal>
    </Card>
  );
}

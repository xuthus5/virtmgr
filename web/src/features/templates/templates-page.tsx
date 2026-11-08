import { useState, type FormEvent } from "react";
import { Link } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, LayoutTemplate, LoaderCircle, Pencil, Plus, Rocket, Trash2 } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { VMTemplate, VMTemplateInput } from "@/api/types";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { PageHeader } from "@/components/layout/page-header";
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useSessionQuery } from "@/features/auth/use-session";
import { initialTemplateForm, formToTemplateInput, templateToForm, type TemplateFormState } from "@/features/templates/template-form";
import { templatesQueryKey, useTemplatesQuery } from "@/features/templates/use-templates-query";

const timeFormatter = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric", month: "2-digit", day: "2-digit",
  hour: "2-digit", minute: "2-digit", hour12: false,
});

function formatTime(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : timeFormatter.format(date);
}

function actionError(cause: unknown, fallback: string) {
  return cause instanceof ApiError ? cause.message : fallback;
}

export function TemplatesPage() {
  const session = useSessionQuery();
  const queryClient = useQueryClient();
  const query = useTemplatesQuery();
  const [formOpen, setFormOpen] = useState(false);
  const [editingID, setEditingID] = useState<number | null>(null);
  const [form, setForm] = useState<TemplateFormState>(initialTemplateForm);
  const [formError, setFormError] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [actionErrorText, setActionErrorText] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<VMTemplate | null>(null);
  const [deleteName, setDeleteName] = useState("");
  const [deleteError, setDeleteError] = useState("");

  const readOnly = Boolean(session.data?.readOnly);
  const viewer = session.data?.user.role === "viewer";
  const canManage = Boolean(session.data) && !readOnly && !viewer;
  const disabledReason = readOnly ? "系统当前处于只读模式" : viewer ? "当前账号没有管理模板的权限" : "";
  const templates = query.data?.items ?? [];

  const createMutation = useMutation({
    mutationFn: (input: VMTemplateInput) => api.createTemplate(input),
    onSuccess: (template) => {
      setSuccessMessage(`模板 ${template.name} 已创建。`);
      setFormError("");
      setActionErrorText("");
      closeForm();
      void queryClient.invalidateQueries({ queryKey: templatesQueryKey });
    },
    onError: (cause) => setFormError(actionError(cause, "创建模板失败，请稍后重试。")),
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, input }: { id: number; input: VMTemplateInput }) => api.updateTemplate(id, input),
    onSuccess: (template) => {
      setSuccessMessage(`模板 ${template.name} 已更新。`);
      setFormError("");
      setActionErrorText("");
      closeForm();
      void queryClient.invalidateQueries({ queryKey: templatesQueryKey });
    },
    onError: (cause) => setFormError(actionError(cause, "更新模板失败，请稍后重试。")),
  });

  const deleteMutation = useMutation({
    mutationFn: (template: VMTemplate) => api.deleteTemplate(template.id),
    onSuccess: (_, template) => {
      setSuccessMessage(`模板 ${template.name} 已删除。`);
      setDeleteTarget(null);
      setDeleteName("");
      setDeleteError("");
      setActionErrorText("");
      void queryClient.invalidateQueries({ queryKey: templatesQueryKey });
    },
    onError: (cause) => setDeleteError(actionError(cause, "删除模板失败，请稍后重试。")),
  });

  function updateField<K extends keyof TemplateFormState>(key: K, value: TemplateFormState[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  function closeForm() {
    setFormOpen(false);
    setEditingID(null);
    setForm(initialTemplateForm);
    setFormError("");
  }

  function openCreate() {
    setFormOpen(true);
    setEditingID(null);
    setForm(initialTemplateForm);
    setFormError("");
    setSuccessMessage("");
  }

  function openEdit(template: VMTemplate) {
    setFormOpen(true);
    setEditingID(template.id);
    setForm(templateToForm(template));
    setFormError("");
    setSuccessMessage("");
  }

  function submitForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canManage) return;
    setFormError("");
    setSuccessMessage("");
    const input = formToTemplateInput(form);
    if (editingID) updateMutation.mutate({ id: editingID, input });
    else createMutation.mutate(input);
  }

  function confirmDelete() {
    if (!deleteTarget || deleteName !== deleteTarget.name) return;
    deleteMutation.mutate(deleteTarget);
  }

  if (!session.data) return <LoadingState label="正在确认访问权限" />;

  return (
    <>
      <PageHeader
        title="虚拟机模板"
        breadcrumb="总览 / 虚拟机模板"
        description="集中维护可复用的计算、固件、网络和 Cloud-init 默认值，并从模板快速进入新建虚拟机。"
        actions={(
          <Button data-testid="template-create-toggle" onClick={openCreate} disabled={!canManage} title={canManage ? undefined : disabledReason}>
            <Plus className="h-4 w-4" />新建模板
          </Button>
        )}
      />

      <div className="mb-4 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <Badge variant="outline"><LayoutTemplate className="h-3 w-3" />共 <span data-testid="template-total">{templates.length}</span> 个模板</Badge>
        {readOnly || viewer ? <span className="text-warning" data-testid="template-read-only-notice">{disabledReason}，模板写操作已禁用。</span> : null}
      </div>

      {successMessage ? <p className="mb-4 flex items-center gap-2 rounded-md border border-success/30 bg-success/5 px-3 py-2 text-sm text-success" data-testid="template-success"><CheckCircle2 className="h-4 w-4" />{successMessage}</p> : null}
      {actionErrorText ? <p className="mb-4 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive" role="alert">{actionErrorText}</p> : null}

      {formOpen ? (
        <Card className="mb-4" data-testid="template-form-card">
          <CardHeader>
            <CardTitle>{editingID ? "编辑模板" : "新建模板"}</CardTitle>
            <CardDescription>模板不包含虚拟机名称和磁盘路径，应用时仍由操作者填写实例专属信息。</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={submitForm} data-testid="template-form">
              <div className="grid gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="template-name">模板名称</Label>
                  <Input id="template-name" value={form.name} onChange={(event) => updateField("name", event.target.value)} placeholder="linux-base" required disabled={!canManage} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="template-description">描述</Label>
                  <Input id="template-description" value={form.description} onChange={(event) => updateField("description", event.target.value)} placeholder="通用 Linux 开发机" disabled={!canManage} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="template-vcpus">vCPU</Label>
                  <Input id="template-vcpus" type="number" min={1} max={255} value={form.vcpus} onChange={(event) => updateField("vcpus", event.target.value)} required disabled={!canManage} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="template-memory">内存 (MiB)</Label>
                  <Input id="template-memory" type="number" min={128} value={form.memoryMiB} onChange={(event) => updateField("memoryMiB", event.target.value)} required disabled={!canManage} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="template-disk-format">磁盘格式</Label>
                  <select id="template-disk-format" value={form.diskFormat} onChange={(event) => updateField("diskFormat", event.target.value)} disabled={!canManage} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                    <option value="qcow2">qcow2</option>
                    <option value="raw">raw</option>
                  </select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="template-network">网络名称</Label>
                  <Input id="template-network" value={form.network} onChange={(event) => updateField("network", event.target.value)} placeholder="default" disabled={!canManage} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="template-firmware">固件</Label>
                  <select id="template-firmware" value={form.firmware} onChange={(event) => { const firmware = event.target.value as VMTemplate["firmware"]; updateField("firmware", firmware); if (firmware === "bios") updateField("secureBoot", false); }} disabled={!canManage} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                    <option value="bios">BIOS</option>
                    <option value="uefi">UEFI</option>
                  </select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="template-iso-path">ISO 路径（可选）</Label>
                  <Input id="template-iso-path" value={form.isoPath} onChange={(event) => updateField("isoPath", event.target.value)} placeholder="/var/lib/libvirt/images/install.iso" disabled={!canManage} />
                </div>
              </div>

              <div className="grid gap-3 border-t pt-4 sm:grid-cols-2">
                <label className="flex items-start gap-3 text-sm">
                  <input aria-label="模板安全启动" type="checkbox" className="mt-0.5 h-4 w-4" checked={form.secureBoot} onChange={(event) => updateField("secureBoot", event.target.checked)} disabled={!canManage || form.firmware !== "uefi"} />
                  <span><span className="block font-medium">安全启动 (Secure Boot)</span><span className="mt-0.5 block text-xs text-muted-foreground">仅 UEFI 固件可用。</span></span>
                </label>
                <label className="flex items-start gap-3 text-sm">
                  <input aria-label="模板启用 Cloud-init" type="checkbox" className="mt-0.5 h-4 w-4" checked={form.cloudInit} onChange={(event) => updateField("cloudInit", event.target.checked)} disabled={!canManage} />
                  <span><span className="block font-medium">启用 Cloud-init</span><span className="mt-0.5 block text-xs text-muted-foreground">保存为新建虚拟机的默认 seed 内容。</span></span>
                </label>
                <label className="flex items-start gap-3 text-sm">
                  <input aria-label="模板开机自动启动" type="checkbox" className="mt-0.5 h-4 w-4" checked={form.autostart} onChange={(event) => updateField("autostart", event.target.checked)} disabled={!canManage} />
                  <span><span className="block font-medium">开机自动启动</span><span className="mt-0.5 block text-xs text-muted-foreground">作为新建虚拟机的默认值。</span></span>
                </label>
                <label className="flex items-start gap-3 text-sm">
                  <input aria-label="模板创建后启动" type="checkbox" className="mt-0.5 h-4 w-4" checked={form.start} onChange={(event) => updateField("start", event.target.checked)} disabled={!canManage} />
                  <span><span className="block font-medium">创建后立即启动</span><span className="mt-0.5 block text-xs text-muted-foreground">作为新建虚拟机的默认值。</span></span>
                </label>
              </div>

              {form.cloudInit ? (
                <div className="space-y-2">
                  <Label htmlFor="template-cloud-init">Cloud-init 用户数据</Label>
                  <textarea
                    id="template-cloud-init"
                    value={form.cloudInitUserData}
                    onChange={(event) => updateField("cloudInitUserData", event.target.value)}
                    required
                    rows={7}
                    spellCheck={false}
                    disabled={!canManage}
                    className="w-full rounded-md border border-input bg-card px-3 py-2 font-mono text-xs shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
                  />
                </div>
              ) : null}

              {formError ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert">{formError}</p> : null}
              <div className="flex flex-wrap gap-2">
                <Button type="submit" data-testid="template-submit" disabled={!canManage || createMutation.isPending || updateMutation.isPending}>
                  {createMutation.isPending || updateMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
                  {editingID ? "保存模板" : "创建模板"}
                </Button>
                <Button type="button" variant="outline" onClick={closeForm}>取消</Button>
              </div>
            </form>
          </CardContent>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>模板清单</CardTitle>
          <CardDescription>模板只保存复用配置，不会直接创建或修改 libvirt 域。</CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {query.isLoading ? <LoadingState label="正在读取虚拟机模板" /> : null}
          {query.isError ? <ErrorState message={query.error instanceof Error ? query.error.message : "无法读取虚拟机模板"} onRetry={() => void query.refetch()} className="m-4" /> : null}
          {!query.isLoading && !query.isError && templates.length === 0 ? (
            <div className="px-5 py-16 text-center" data-testid="template-empty">
              <LayoutTemplate className="mx-auto h-7 w-7 text-muted-foreground" />
              <p className="mt-3 text-sm font-medium">还没有虚拟机模板</p>
              <p className="mt-1 text-xs text-muted-foreground">创建模板后，可在新建虚拟机页面一键预填规格。</p>
            </div>
          ) : null}
          {templates.length ? (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader><TableRow><TableHead>名称</TableHead><TableHead>规格</TableHead><TableHead>固件</TableHead><TableHead>网络 / 磁盘</TableHead><TableHead>Cloud-init</TableHead><TableHead>启动策略</TableHead><TableHead>更新时间</TableHead><TableHead>操作</TableHead></TableRow></TableHeader>
                <TableBody>
                  {templates.map((template) => (
                    <TableRow key={template.id} data-testid={`template-row-${template.name}`}>
                      <TableCell>
                        <p className="font-medium">{template.name}</p>
                        <p className="max-w-64 truncate text-xs text-muted-foreground">{template.description || "无描述"}</p>
                      </TableCell>
                      <TableCell className="whitespace-nowrap text-xs">{template.vcpus} vCPU · {template.memoryMiB} MiB</TableCell>
                      <TableCell><Badge variant={template.firmware === "uefi" ? "default" : "outline"}>{template.firmware === "uefi" ? "UEFI" : "BIOS"}{template.secureBoot ? " · Secure Boot" : ""}</Badge></TableCell>
                      <TableCell className="text-xs"><p>{template.network}</p><p className="text-muted-foreground">{template.diskFormat}{template.isoPath ? " · ISO" : ""}</p></TableCell>
                      <TableCell><Badge variant={template.cloudInit ? "success" : "secondary"}>{template.cloudInit ? "已启用" : "未启用"}</Badge></TableCell>
                      <TableCell className="whitespace-nowrap text-xs">{template.autostart ? "自动启动" : "手动启动"}{template.start ? " · 创建后启动" : ""}</TableCell>
                      <TableCell className="whitespace-nowrap text-xs text-muted-foreground">{formatTime(template.updatedAt)}</TableCell>
                      <TableCell>
                        <div className="flex items-center gap-1">
                          <Button asChild variant="ghost" size="icon-sm" title="使用模板新建虚拟机">
                            <Link to="/vms/new" search={{ template: template.id }} data-testid={`template-use-${template.name}`} aria-label={`使用模板 ${template.name}`}><Rocket className="h-4 w-4" /></Link>
                          </Button>
                          <Button type="button" variant="ghost" size="icon-sm" data-testid={`template-edit-${template.name}`} aria-label={`编辑模板 ${template.name}`} title={canManage ? "编辑模板" : disabledReason} disabled={!canManage} onClick={() => openEdit(template)}><Pencil className="h-4 w-4" /></Button>
                          <Button type="button" variant="ghost" size="icon-sm" data-testid={`template-delete-${template.name}`} aria-label={`删除模板 ${template.name}`} title={canManage ? "删除模板" : disabledReason} disabled={!canManage} onClick={() => { setDeleteTarget(template); setDeleteName(""); setDeleteError(""); setActionErrorText(""); }}><Trash2 className="h-4 w-4" /></Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          ) : null}
        </CardContent>
      </Card>

      <AlertDialog open={Boolean(deleteTarget)} onOpenChange={(open) => { if (!open) { setDeleteTarget(null); setDeleteName(""); setDeleteError(""); } }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除虚拟机模板？</AlertDialogTitle>
            <AlertDialogDescription>模板删除后不会影响已经创建的虚拟机。请输入模板名称确认。</AlertDialogDescription>
          </AlertDialogHeader>
          <div className="space-y-2">
            <Label htmlFor="template-delete-confirm">输入模板名称确认</Label>
            <Input id="template-delete-confirm" value={deleteName} onChange={(event) => setDeleteName(event.target.value)} placeholder={deleteTarget?.name} />
            {deleteError ? <p className="text-xs text-destructive" role="alert">{deleteError}</p> : null}
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <Button variant="destructive" data-testid="confirm-delete-template" disabled={!deleteTarget || deleteName !== deleteTarget.name || deleteMutation.isPending} onClick={confirmDelete}>
              {deleteMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}删除模板
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

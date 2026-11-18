import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Database, LoaderCircle, Plus, Trash2 } from "lucide-react";
import { api } from "@/api/client";
import type { BackupRepository, BackupRepositoryInput, Role } from "@/api/types";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Modal } from "@/components/ui/modal";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useBackupRepositoriesQuery } from "@/features/backups/use-backups-query";
import { actionReason, errorText, formatBytes, formatTime, type BackupNotice } from "@/features/backups/backup-ui";
import { useStoragePoolsQuery } from "@/features/storage/use-storage-query";
import { queryKeys } from "@/lib/query-keys";

export function RepositoriesPanel({ role, readOnly, onNotice }: {
  role: Role | undefined;
  readOnly: boolean;
  onNotice: (notice: BackupNotice) => void;
}) {
  const queryClient = useQueryClient();
  const repositories = useBackupRepositoriesQuery();
  const pools = useStoragePoolsQuery();
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<BackupRepository | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<BackupRepository | null>(null);
  const [form, setForm] = useState<BackupRepositoryInput>({ name: "", poolName: "", isDefault: false });
  const [error, setError] = useState("");
  const disabledReason = actionReason(role, readOnly, "admin");
  const items = repositories.data?.items ?? [];
  const activePools = (pools.data?.items ?? []).filter((pool) => pool.state === "active");
  const mutation = useMutation({
    mutationFn: (input: BackupRepositoryInput) => editing
      ? api.updateBackupRepository(editing.id, input)
      : api.createBackupRepository(input),
  });
  const deleteMutation = useMutation({ mutationFn: api.deleteBackupRepository });

  function openCreate() {
    setEditing(null);
    setForm({ name: "", poolName: activePools[0]?.name ?? "", isDefault: !items.some((item) => item.isDefault) });
    setError("");
    setFormOpen(true);
  }

  function openEdit(repository: BackupRepository) {
    setEditing(repository);
    setForm({ name: repository.name, poolName: repository.poolName, isDefault: repository.isDefault });
    setError("");
    setFormOpen(true);
  }

  async function refresh() {
    await queryClient.invalidateQueries({ queryKey: queryKeys.backups.repositories });
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      await mutation.mutateAsync(form);
      setFormOpen(false);
      await refresh();
      onNotice({ tone: "success", text: editing ? "备份存储库已更新。" : "备份存储库已创建。" });
    } catch (cause) {
      setError(errorText(cause, "保存备份存储库失败"));
      onNotice({ tone: "error", text: errorText(cause, "保存备份存储库失败") });
    }
  }

  async function remove() {
    if (!deleteTarget || deleteMutation.isPending) return;
    try {
      await deleteMutation.mutateAsync(deleteTarget.id);
      setDeleteTarget(null);
      await refresh();
      onNotice({ tone: "success", text: "备份存储库已删除。" });
    } catch (cause) {
      onNotice({ tone: "error", text: errorText(cause, "删除备份存储库失败") });
      setDeleteTarget(null);
    }
  }

  function closeForm() {
    if (!mutation.isPending) setFormOpen(false);
  }

  return <>
    <Card>
      <CardHeader><div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between"><div><CardTitle>备份存储库</CardTitle><CardDescription>首期仅支持活跃的 libvirt 存储池。建议使用独立磁盘或独立 NFS 目录，不与虚拟机主磁盘共用。</CardDescription></div><Button size="sm" data-testid="backup-repository-create" disabled={Boolean(disabledReason)} title={disabledReason || "添加备份存储库"} onClick={openCreate}><Plus className="h-4 w-4" />添加存储库</Button></div></CardHeader>
      <CardContent className="p-0">
        {repositories.isLoading ? <LoadingState label="正在读取备份存储库" /> : null}
        {repositories.isError ? <ErrorState className="m-4" message={errorText(repositories.error, "无法读取备份存储库")} onRetry={() => void repositories.refetch()} /> : null}
        {!repositories.isLoading && !repositories.isError && items.length === 0 ? <div className="px-5 py-16 text-center" data-testid="backup-repositories-empty"><Database className="mx-auto h-8 w-8 text-muted-foreground" /><p className="mt-3 font-medium">没有可用的备份存储库</p><p className="mt-1 text-sm text-muted-foreground">先创建并启动 libvirt 存储池，再添加为备份存储库。</p></div> : null}
        {items.length ? <Table className="min-w-[760px]"><TableHeader><TableRow><TableHead>名称</TableHead><TableHead>类型</TableHead><TableHead>libvirt 存储池</TableHead><TableHead>默认</TableHead><TableHead>更新时间</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader><TableBody>{items.map((repository) => <TableRow key={repository.id} data-testid={`backup-repository-row-${repository.id}`}><TableCell className="font-medium">{repository.name}</TableCell><TableCell><Badge variant="outline">libvirt-pool</Badge></TableCell><TableCell className="font-mono text-xs">{repository.poolName}</TableCell><TableCell>{repository.isDefault ? <Badge variant="success">默认</Badge> : <span className="text-muted-foreground">-</span>}</TableCell><TableCell className="text-xs">{formatTime(repository.updatedAt)}</TableCell><TableCell><div className="flex justify-end gap-1"><Button variant="ghost" size="sm" disabled={Boolean(disabledReason)} title={disabledReason || "编辑存储库"} data-testid={`backup-repository-edit-${repository.id}`} onClick={() => openEdit(repository)}>编辑</Button><Button variant="ghost" size="sm" disabled={Boolean(disabledReason)} title={disabledReason || "删除存储库；仍有关联备份或计划时会被阻止"} data-testid={`backup-repository-delete-${repository.id}`} onClick={() => setDeleteTarget(repository)}><Trash2 className="h-4 w-4" />删除</Button></div></TableCell></TableRow>)}</TableBody></Table> : null}</CardContent>
    </Card>
    <Modal open={formOpen} onClose={closeForm} title={editing ? "编辑备份存储库" : "添加备份存储库"} description="只有活跃的 libvirt 存储池可以被添加；存储池名称创建后不可更改。" testId="backup-repository-modal" footer={<><Button variant="ghost" disabled={mutation.isPending} onClick={closeForm}>取消</Button><Button form="backup-repository-form" type="submit" disabled={mutation.isPending}>{mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : null}{mutation.isPending ? "正在保存" : editing ? "保存修改" : "添加存储库"}</Button></>}>
      <form id="backup-repository-form" className="space-y-4" onSubmit={(event) => void submit(event)}>
        <div className="space-y-2"><Label htmlFor="repository-name">名称</Label><Input id="repository-name" data-testid="backup-repository-name" value={form.name} maxLength={64} onChange={(event) => setForm({ ...form, name: event.target.value })} /></div>
        <div className="space-y-2"><Label htmlFor="repository-pool">libvirt 存储池</Label><select id="repository-pool" data-testid="backup-repository-pool" disabled={Boolean(editing)} value={form.poolName} onChange={(event) => setForm({ ...form, poolName: event.target.value })} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm disabled:opacity-60">{editing && !activePools.some((pool) => pool.name === form.poolName) ? <option value={form.poolName}>{form.poolName}</option> : null}{activePools.map((pool) => <option key={pool.name} value={pool.name}>{pool.name} · 可用 {formatBytes(pool.availableBytes)}</option>)}</select></div>
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.isDefault} onChange={(event) => setForm({ ...form, isDefault: event.target.checked })} />设为默认备份存储库</label>
        {error ? <p role="alert" className="text-xs text-destructive">{error}</p> : null}
      </form>
    </Modal>
    <Modal open={Boolean(deleteTarget)} onClose={() => { if (!deleteMutation.isPending) setDeleteTarget(null); }} title="删除备份存储库" description="不会删除存储池或备份产物；仍有关联备份和定时计划时删除会被阻止。" footer={<><Button variant="ghost" disabled={deleteMutation.isPending} onClick={() => setDeleteTarget(null)}>取消</Button><Button variant="destructive" data-testid="backup-repository-delete-confirm" disabled={deleteMutation.isPending} onClick={() => void remove()}>{deleteMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}{deleteMutation.isPending ? "删除中..." : "确认删除"}</Button></>}>
      {deleteTarget ? <p className="text-sm">确认删除备份存储库 <span className="font-medium">{deleteTarget.name}</span>？</p> : null}
    </Modal>
  </>;
}

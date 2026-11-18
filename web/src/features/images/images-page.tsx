import { useEffect, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, Database, HardDrive, Image as ImageIcon, Layers3, LoaderCircle, Plus, RefreshCw, ShieldCheck, Trash2, Upload } from "lucide-react";
import { api } from "@/api/client";
import type { VMImage, VMImageInput } from "@/api/types";
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
import { imagesQueryKey, useImagesQuery } from "@/features/images/use-images-query";
import { useStoragePoolsQuery, useStorageVolumesQuery } from "@/features/storage/use-storage-query";
import { queryKeys } from "@/lib/query-keys";
import { formatBytes } from "@/lib/utils";

const timeFormatter = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false,
});

type ImageForm = {
  name: string;
  description: string;
  pool: string;
  volume: string;
  osFamily: string;
  osName: string;
  architecture: string;
};

const initialForm: ImageForm = {
  name: "", description: "", pool: "", volume: "", osFamily: "linux", osName: "", architecture: "x86_64",
};

const minImageBytes = 1 << 20;
const maxImageBytes = 256 * 1024 ** 3;

type ImageFormMode = "register" | "upload";

function formatTime(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : timeFormatter.format(date);
}

function imageError(cause: unknown, fallback: string) {
  return cause instanceof Error && cause.message ? cause.message : fallback;
}

export function ImagesPage() {
  const session = useSessionQuery();
  const queryClient = useQueryClient();
  const query = useImagesQuery();
  const poolsQuery = useStoragePoolsQuery();
  const [formOpen, setFormOpen] = useState(false);
  const [formMode, setFormMode] = useState<ImageFormMode>("register");
  const [form, setForm] = useState<ImageForm>(initialForm);
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<VMImage | null>(null);
  const activePools = (poolsQuery.data?.items ?? []).filter((pool) => pool.state === "active");
  const volumesQuery = useStorageVolumesQuery(form.pool || activePools[0]?.name || "");
  const volumes = (volumesQuery.data?.items ?? []).filter((volume) => {
    const format = (volume.format || "").toLowerCase();
    return format === "qcow2" || format === "raw" || /\.(qcow2|raw|img)$/i.test(volume.name);
  });
  const images = query.data?.items ?? [];
  const readOnly = Boolean(session.data?.readOnly);
  const viewer = session.data?.user.role === "viewer";
  const canManage = Boolean(session.data) && !readOnly && !viewer;
  const disabledReason = readOnly ? "系统当前处于只读模式" : viewer ? "当前账号没有管理镜像的权限" : "";

  useEffect(() => {
    if (form.pool || !activePools.length) return;
    const preferred = activePools.find((pool) => pool.name === "default") ?? activePools[0];
    setForm((current) => ({ ...current, pool: preferred.name }));
  }, [activePools, form.pool]);

  const registerMutation = useMutation({
    mutationFn: (input: VMImageInput) => api.registerImage(input),
    onSuccess: (image) => {
      setNotice({ tone: "success", text: `镜像 ${image.name} 已登记。请刷新以计算 SHA-256 校验值。` });
      setForm(initialForm);
      setFormOpen(false);
      void queryClient.invalidateQueries({ queryKey: imagesQueryKey });
    },
    onError: (cause) => setNotice({ tone: "error", text: imageError(cause, "登记镜像失败，请稍后重试。") }),
  });

  const uploadMutation = useMutation({
    mutationFn: async () => {
      if (!uploadFile) throw new Error("请选择要上传的镜像文件。");
      if (!/\.(qcow2|raw)$/i.test(uploadFile.name)) throw new Error("仅支持 .qcow2 和 .raw 镜像文件。");
      if (uploadFile.size < minImageBytes || uploadFile.size > maxImageBytes) {
        throw new Error(`镜像大小必须在 ${formatBytes(minImageBytes)} 到 ${formatBytes(maxImageBytes)} 之间。`);
      }
      const uploaded = await api.uploadStorageVolume(form.pool, uploadFile);
      try {
        return await api.registerImage({
          name: form.name.trim(), description: form.description.trim(), pool: form.pool,
          volume: uploaded.name, osFamily: form.osFamily.trim(), osName: form.osName.trim(), architecture: form.architecture.trim(),
        });
      } catch (cause) {
        try {
          await api.deleteStorageVolume(form.pool, uploaded.name);
          throw cause;
        } catch (cleanupCause) {
          if (cleanupCause === cause) throw cause;
          throw new Error(`${imageError(cause, "镜像登记失败。")}；上传卷 ${uploaded.name} 自动清理失败，请到存储页手动处理。`);
        }
      }
    },
    onSuccess: (image) => {
      setNotice({ tone: "success", text: `镜像 ${image.name} 已上传并登记。可点击刷新计算 SHA-256。` });
      setForm(initialForm);
      setUploadFile(null);
      setFormOpen(false);
      void queryClient.invalidateQueries({ queryKey: imagesQueryKey });
      void queryClient.invalidateQueries({ queryKey: queryKeys.storage.root });
    },
    onError: (cause) => setNotice({ tone: "error", text: imageError(cause, "上传镜像失败，请稍后重试。") }),
  });

  const refreshMutation = useMutation({
    mutationFn: (image: VMImage) => api.refreshImage(image.id),
    onSuccess: (image) => {
      setNotice({ tone: "success", text: `镜像 ${image.name} 的容量与 SHA-256 已刷新。` });
      void queryClient.invalidateQueries({ queryKey: imagesQueryKey });
    },
    onError: (cause) => setNotice({ tone: "error", text: imageError(cause, "刷新镜像失败，请稍后重试。") }),
  });

  const deleteMutation = useMutation({
    mutationFn: (image: VMImage) => api.deleteImage(image.id),
    onSuccess: (_, image) => {
      setDeleteTarget(null);
      setNotice({ tone: "success", text: `镜像条目 ${image.name} 已移除；底层磁盘卷未被删除。` });
      void queryClient.invalidateQueries({ queryKey: imagesQueryKey });
    },
    onError: (cause) => setNotice({ tone: "error", text: imageError(cause, "移除镜像失败，请稍后重试。") }),
  });

  function update<K extends keyof ImageForm>(key: K, value: ImageForm[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  function selectVolume(name: string) {
    const volume = volumes.find((item) => item.name === name);
    setForm((current) => ({
      ...current,
      volume: name,
      name: current.name || name.replace(/\.(qcow2|raw|img)$/i, ""),
      osName: current.osName,
      description: current.description,
      ...(volume?.format ? { description: current.description } : {}),
    }));
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setNotice(null);
    if (formMode === "upload") {
      uploadMutation.mutate();
      return;
    }
    registerMutation.mutate({
      name: form.name.trim(), description: form.description.trim(), pool: form.pool,
      volume: form.volume, osFamily: form.osFamily.trim(), osName: form.osName.trim(), architecture: form.architecture.trim(),
    });
  }

  function openCreate(mode: ImageFormMode) {
    setFormMode(mode);
    setForm({ ...initialForm, pool: activePools[0]?.name ?? "" });
    setUploadFile(null);
    setFormOpen(true);
    setNotice(null);
  }

  return (
    <div data-testid="images-page">
      <PageHeader
        title="镜像库"
        breadcrumb="总览 / 镜像库"
        description="上传或登记存储池中的 qcow2/raw 基础磁盘，作为新建虚拟机和模板的系统盘来源。创建虚拟机时使用全量克隆。"
        actions={<div className="flex flex-wrap gap-2"><Button variant="outline" data-testid="image-upload-toggle" onClick={() => openCreate("upload")} disabled={!canManage} title={canManage ? undefined : disabledReason}><Upload className="h-4 w-4" />上传镜像</Button><Button data-testid="image-create-toggle" onClick={() => openCreate("register")} disabled={!canManage} title={canManage ? undefined : disabledReason}><Plus className="h-4 w-4" />登记已有卷</Button></div>}
      />

      <div className="mb-4 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <Badge variant="outline"><Layers3 className="h-3 w-3" />共 <span data-testid="image-total">{images.length}</span> 个镜像</Badge>
        <Badge variant="outline">全量克隆</Badge>
        <Badge variant="outline">SHA-256 按需计算</Badge>
        {readOnly || viewer ? <span className="text-warning" data-testid="image-read-only-notice">{disabledReason}，镜像写操作已禁用。</span> : null}
      </div>

      {notice ? <p role="status" data-testid="image-notice" className={notice.tone === "success" ? "mb-4 flex items-center gap-2 rounded-md border border-success/30 bg-success/5 px-3 py-2 text-sm text-success" : "mb-4 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive"}>{notice.tone === "success" ? <CheckCircle2 className="h-4 w-4" /> : null}{notice.text}</p> : null}

      {formOpen ? (
        <Card className="mb-4" data-testid="image-form-card">
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><ImageIcon className="h-4 w-4 text-primary" />{formMode === "upload" ? "上传并登记新镜像" : "登记已有磁盘卷"}</CardTitle>
            <CardDescription>{formMode === "upload" ? "上传 qcow2/raw 文件到目标存储池，完成后自动登记为镜像。只支持 1 MiB 至 256 GiB 的文件。" : "只接受活跃存储池中的 qcow2/raw 卷。登记不会修改磁盘内容，也不会上传文件。"}</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={submit} data-testid="image-form">
              <div className="grid gap-4 md:grid-cols-2">
                <div className="space-y-2"><Label htmlFor="image-name">镜像名称</Label><Input id="image-name" data-testid="image-name" value={form.name} onChange={(event) => update("name", event.target.value)} placeholder="ubuntu-24.04-cloud" required maxLength={64} /></div>
                <div className="space-y-2"><Label htmlFor="image-pool">存储池</Label><select id="image-pool" data-testid="image-pool" value={form.pool} onChange={(event) => { update("pool", event.target.value); update("volume", ""); }} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">{!activePools.length ? <option value="">{poolsQuery.isLoading ? "正在读取存储池..." : "没有活跃存储池"}</option> : null}{activePools.map((pool) => <option key={pool.uuid || pool.name} value={pool.name}>{pool.name} · {pool.targetPath || "无目标路径"}</option>)}</select></div>
                {formMode === "upload" ? (
                  <div className="space-y-2">
                    <Label htmlFor="image-upload-file">镜像文件</Label>
                    <Input id="image-upload-file" data-testid="image-upload-file" type="file" accept=".qcow2,.raw" required onChange={(event) => {
                      const file = event.target.files?.[0] ?? null;
                      setUploadFile(file);
                      if (file) {
                        setForm((current) => ({
                          ...current,
                          volume: file.name,
                          name: current.name.trim() || file.name.replace(/\.(qcow2|raw)$/i, ""),
                        }));
                      }
                    }} className="h-9 py-1 text-xs" />
                    <p className="text-xs text-muted-foreground">{uploadFile ? `${uploadFile.name} · ${formatBytes(uploadFile.size)}` : "支持 .qcow2 / .raw"}</p>
                  </div>
                ) : (
                  <div className="space-y-2"><Label htmlFor="image-volume">磁盘卷</Label><select id="image-volume" data-testid="image-volume" value={form.volume} onChange={(event) => selectVolume(event.target.value)} required className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm"><option value="">{volumesQuery.isLoading ? "正在读取存储卷..." : "选择 qcow2/raw 卷"}</option>{volumes.map((volume) => <option key={volume.path || volume.name} value={volume.name}>{volume.name} · {formatBytes(volume.capacityBytes)}</option>)}</select></div>
                )}
                <div className="space-y-2"><Label htmlFor="image-description">描述</Label><Input id="image-description" value={form.description} onChange={(event) => update("description", event.target.value)} placeholder="Ubuntu 24.04 LTS Cloud Image" maxLength={512} /></div>
                <div className="space-y-2"><Label htmlFor="image-os-family">OS 家族</Label><Input id="image-os-family" value={form.osFamily} onChange={(event) => update("osFamily", event.target.value)} placeholder="linux" maxLength={32} /></div>
                <div className="space-y-2"><Label htmlFor="image-os-name">OS 名称</Label><Input id="image-os-name" value={form.osName} onChange={(event) => update("osName", event.target.value)} placeholder="Ubuntu 24.04 LTS" maxLength={64} /></div>
                <div className="space-y-2"><Label htmlFor="image-architecture">架构</Label><Input id="image-architecture" value={form.architecture} onChange={(event) => update("architecture", event.target.value)} placeholder="x86_64" maxLength={32} /></div>
              </div>
              <div className="flex flex-wrap gap-2"><Button type="submit" data-testid="image-submit" disabled={(formMode === "upload" ? uploadMutation.isPending : registerMutation.isPending) || !form.name.trim() || !form.pool || (formMode === "upload" ? !uploadFile : !form.volume)}>{formMode === "upload" ? (uploadMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />) : (registerMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />)}{formMode === "upload" ? (uploadMutation.isPending ? "正在上传..." : "上传并登记") : "登记镜像"}</Button><Button type="button" variant="outline" onClick={() => setFormOpen(false)} disabled={uploadMutation.isPending || registerMutation.isPending}>取消</Button></div>
            </form>
          </CardContent>
        </Card>
      ) : null}

      <Card>
        <CardHeader><CardTitle className="flex items-center gap-2"><Database className="h-4 w-4 text-primary" />镜像清单</CardTitle><CardDescription>移除镜像只删除 VirtMgr 元数据；被模板引用时后端会拒绝移除。底层存储卷始终保留。</CardDescription></CardHeader>
        <CardContent className="p-0">
          {query.isLoading ? <LoadingState label="正在读取镜像库" /> : null}
          {query.isError ? <ErrorState message={query.error instanceof Error ? query.error.message : "无法读取镜像库"} onRetry={() => void query.refetch()} className="m-4" /> : null}
          {!query.isLoading && !query.isError && images.length === 0 ? <div className="px-5 py-16 text-center" data-testid="image-empty"><ImageIcon className="mx-auto h-7 w-7 text-muted-foreground" /><p className="mt-3 text-sm font-medium">镜像库为空</p><p className="mt-1 text-xs text-muted-foreground">先把已有 qcow2/raw 卷登记为镜像，再在新建虚拟机或模板中选择。</p></div> : null}
          {images.length ? <Table className="min-w-[1080px]">
            <TableHeader><TableRow><TableHead>镜像</TableHead><TableHead>来源</TableHead><TableHead>格式</TableHead><TableHead>容量</TableHead><TableHead>操作系统</TableHead><TableHead>SHA-256</TableHead><TableHead>更新时间</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
            <TableBody>{images.map((image) => (
              <TableRow key={image.id} data-testid={`image-row-${image.id}`}>
                <TableCell><p className="font-medium">{image.name}</p><p className="max-w-64 truncate text-xs text-muted-foreground">{image.description || "无描述"}</p></TableCell>
                <TableCell className="text-xs"><p className="flex items-center gap-1"><HardDrive className="h-3 w-3" />{image.pool}</p><p className="max-w-48 truncate font-mono text-[10px] text-muted-foreground" title={image.volume}>{image.volume}</p></TableCell>
                <TableCell><Badge variant="outline">{image.format}</Badge></TableCell>
                <TableCell className="whitespace-nowrap text-xs"><p>{formatBytes(image.capacityBytes)}</p><p className="text-muted-foreground">占用 {formatBytes(image.allocationBytes)}</p></TableCell>
                <TableCell className="text-xs"><p>{image.osName || "-"}</p><p className="text-muted-foreground">{image.osFamily || "unknown"} · {image.architecture || "-"}</p></TableCell>
                <TableCell><p className="max-w-40 truncate font-mono text-[10px]" title={image.sha256 || "尚未计算"}>{image.sha256 ? `${image.sha256.slice(0, 16)}…` : "未计算"}</p><p className="text-[10px] text-muted-foreground">{image.sha256 ? <span className="inline-flex items-center gap-1"><ShieldCheck className="h-3 w-3" />已计算</span> : "点击刷新计算"}</p></TableCell>
                <TableCell className="whitespace-nowrap text-xs text-muted-foreground">{formatTime(image.updatedAt)}</TableCell>
                <TableCell><div className="flex justify-end gap-1"><Button variant="outline" size="sm" data-testid={`image-refresh-${image.id}`} disabled={!canManage || refreshMutation.isPending} title={canManage ? "计算 SHA-256 并刷新容量" : disabledReason} onClick={() => refreshMutation.mutate(image)}>{refreshMutation.isPending && refreshMutation.variables?.id === image.id ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}刷新</Button><Button variant="outline" size="sm" data-testid={`image-delete-${image.id}`} disabled={!canManage || deleteMutation.isPending} title={canManage ? "仅移除镜像条目" : disabledReason} onClick={() => setDeleteTarget(image)}><Trash2 className="h-3.5 w-3.5" />移除</Button></div></TableCell>
              </TableRow>
            ))}</TableBody>
          </Table> : null}
        </CardContent>
      </Card>

      <div className="mt-4 flex items-start gap-2 rounded-md border bg-muted/25 px-3 py-2 text-xs text-muted-foreground"><AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" /><p>创建虚拟机时可选择全量克隆，或为 qcow2 镜像选择同存储池链接克隆。链接克隆的基础卷在子卷存在时禁止删除；跨池自动搬移和镜像格式转换暂未提供。</p></div>

      <AlertDialog open={Boolean(deleteTarget)} onOpenChange={(open) => { if (!open) setDeleteTarget(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader><AlertDialogTitle>移除镜像条目？</AlertDialogTitle><AlertDialogDescription>将从 VirtMgr 移除 {deleteTarget?.name} 的登记信息，但不会删除存储池中的卷 {deleteTarget?.volume}。被模板引用时操作会失败。</AlertDialogDescription></AlertDialogHeader>
          <AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><Button variant="destructive" data-testid="confirm-delete-image" disabled={!deleteTarget || deleteMutation.isPending} onClick={() => { if (deleteTarget) deleteMutation.mutate(deleteTarget); }}>{deleteMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}确认移除</Button></AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

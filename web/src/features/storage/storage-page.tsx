import { useEffect, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, Database, FolderOpen, HardDrive, Layers3, LoaderCircle, Plus, Trash2, Upload } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { StoragePoolState, StorageVolume, StorageVolumeCreateInput, StorageVolumeFormat } from "@/api/types";
import { MetricCard } from "@/components/data/metric-card";
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
import { StoragePoolsCard } from "@/features/storage/storage-pools-card";
import { storagePoolsQueryKey, useStoragePoolsQuery, useStorageVolumesQuery } from "@/features/storage/use-storage-query";
import { formatBytes, percent } from "@/lib/utils";

type CreateFormState = {
  name: string;
  format: StorageVolumeFormat;
  capacityGiB: string;
};

const initialCreateForm: CreateFormState = { name: "", format: "qcow2", capacityGiB: "20" };

const poolStateLabel: Record<StoragePoolState, string> = {
  active: "活跃",
  inactive: "未激活",
  degraded: "降级",
  inaccessible: "不可访问",
  unknown: "未知",
};

function poolStateVariant(state: StoragePoolState): "success" | "secondary" | "warning" | "destructive" | "default" {
  switch (state) {
    case "active":
      return "success";
    case "inactive":
      return "secondary";
    case "degraded":
      return "warning";
    case "inaccessible":
      return "destructive";
    default:
      return "default";
  }
}

export function StoragePage() {
  const session = useSessionQuery();
  const queryClient = useQueryClient();
  const poolsQuery = useStoragePoolsQuery();
  const [selectedPool, setSelectedPool] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState<CreateFormState>(initialCreateForm);
  const [createError, setCreateError] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [successTestId, setSuccessTestId] = useState("storage-create-success");
  const [uploadOpen, setUploadOpen] = useState(false);
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [uploadError, setUploadError] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<StorageVolume | null>(null);
  const [deleteName, setDeleteName] = useState("");
  const [deleteError, setDeleteError] = useState("");
  const pools = poolsQuery.data?.items ?? [];
  const volumesQuery = useStorageVolumesQuery(selectedPool);
  const selected = pools.find((pool) => pool.name === selectedPool);

  const createMutation = useMutation({
    mutationFn: ({ pool, input }: { pool: string; input: StorageVolumeCreateInput }) => api.createStorageVolume(pool, input),
    onSuccess: (volume, variables) => {
      setSuccessMessage(`存储卷 ${volume.name} 已创建。`);
      setSuccessTestId("storage-create-success");
      setCreateError("");
      setCreateOpen(false);
      setCreateForm(initialCreateForm);
      void queryClient.invalidateQueries({ queryKey: ["storage", "volumes", variables.pool] });
      void queryClient.invalidateQueries({ queryKey: storagePoolsQueryKey });
    },
    onError: (cause) => {
      setCreateError(cause instanceof ApiError ? cause.message : "创建存储卷失败，请稍后重试。");
    },
  });

  const uploadMutation = useMutation({
    mutationFn: ({ pool, file }: { pool: string; file: File }) => api.uploadStorageVolume(pool, file),
    onSuccess: (volume, variables) => {
      setSuccessMessage(`ISO ${volume.name} 已上传。`);
      setSuccessTestId("storage-upload-success");
      setUploadError("");
      setUploadOpen(false);
      setUploadFile(null);
      void queryClient.invalidateQueries({ queryKey: ["storage", "volumes", variables.pool] });
      void queryClient.invalidateQueries({ queryKey: storagePoolsQueryKey });
    },
    onError: (cause) => {
      setUploadError(cause instanceof ApiError ? cause.message : "上传 ISO 失败，请稍后重试。");
    },
  });

  const deleteMutation = useMutation({
    mutationFn: ({ pool, name }: { pool: string; name: string }) => api.deleteStorageVolume(pool, name),
    onSuccess: (_, variables) => {
      setSuccessMessage(`存储卷 ${variables.name} 已删除。`);
      setSuccessTestId("storage-create-success");
      setDeleteTarget(null);
      setDeleteName("");
      setDeleteError("");
      void queryClient.invalidateQueries({ queryKey: ["storage", "volumes", variables.pool] });
      void queryClient.invalidateQueries({ queryKey: storagePoolsQueryKey });
    },
    onError: (cause) => {
      setDeleteError(cause instanceof ApiError ? cause.message : "删除存储卷失败，请稍后重试。");
    },
  });

  useEffect(() => {
    if (!pools.length) return;
    if (selectedPool && pools.some((pool) => pool.name === selectedPool)) return;
    const preferred = pools.find((pool) => pool.state === "active") ?? pools[0];
    setSelectedPool(preferred.name);
  }, [pools, selectedPool]);

  const activePools = pools.filter((pool) => pool.state === "active").length;
  const totalCapacity = pools.reduce((sum, pool) => sum + pool.capacityBytes, 0);
  const totalAllocation = pools.reduce((sum, pool) => sum + pool.allocationBytes, 0);
  const readOnly = Boolean(session.data?.readOnly);
  const viewer = session.data?.user.role === "viewer";
  const canManage = Boolean(session.data) && !readOnly && !viewer;
  const selectedActive = selected?.state === "active";
  const manageDisabledReason = !canManage
    ? readOnly ? "系统当前处于只读模式" : "当前账号没有管理存储的权限"
    : "";
  const createDisabledReason = !canManage
    ? manageDisabledReason
    : !selectedActive ? "只有活跃存储池可以创建卷" : "";
  const uploadDisabledReason = !canManage
    ? manageDisabledReason
    : !selectedActive ? "只有活跃存储池可以上传 ISO" : "";

  function selectPool(name: string) {
    setSelectedPool(name);
    setCreateOpen(false);
    setUploadOpen(false);
    setCreateError("");
    setUploadError("");
    setUploadFile(null);
    setSuccessMessage("");
    setDeleteTarget(null);
    setDeleteError("");
  }

  function openDelete(volume: StorageVolume) {
    setDeleteTarget(volume);
    setDeleteName("");
    setDeleteError("");
  }

  function handleCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selected || !selectedActive || !canManage) return;
    const capacityGiB = Number(createForm.capacityGiB);
    if (!Number.isFinite(capacityGiB) || capacityGiB <= 0) {
      setCreateError("请输入有效的卷容量。");
      return;
    }
    setCreateError("");
    createMutation.mutate({
      pool: selected.name,
      input: {
        name: createForm.name.trim(),
        format: createForm.format,
        capacityBytes: Math.round(capacityGiB * 1024 ** 3),
      },
    });
  }

  function handleUpload(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selected || !selectedActive || !canManage || !uploadFile) return;
    if (!uploadFile.name.toLowerCase().endsWith(".iso")) {
      setUploadError("请选择 .iso 文件。");
      return;
    }
    if (uploadFile.size < 1024 ** 2 || uploadFile.size > 16 * 1024 ** 4) {
      setUploadError("ISO 文件大小必须在 1 MiB 到 16 GiB 之间。");
      return;
    }
    setUploadError("");
    uploadMutation.mutate({ pool: selected.name, file: uploadFile });
  }

  return (
    <>
      <PageHeader title="存储" breadcrumb="总览 / 存储" description="读取 libvirt 存储池、容量和卷元数据。所有存储操作仍以 libvirt 为权威来源。" />

      {successMessage ? (
        <div className="mb-4 flex items-center gap-2 rounded-lg border border-success/30 bg-success/10 px-4 py-3 text-sm text-success" role="status" data-testid={successTestId}>
          <CheckCircle2 className="h-4 w-4" />{successMessage}
        </div>
      ) : null}

      {poolsQuery.isLoading ? <LoadingState label="正在读取存储池" /> : null}
      {poolsQuery.isError ? <ErrorState message={poolsQuery.error instanceof Error ? poolsQuery.error.message : "无法读取存储池"} onRetry={() => void poolsQuery.refetch()} /> : null}

      {poolsQuery.data ? (
        <>
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <MetricCard label="存储池" value={poolsQuery.data.total} hint="已定义的全部存储池" icon={Database} tone="primary" testId="storage-pool-total" />
            <MetricCard label="活跃存储池" value={activePools} hint={`${pools.length - activePools} 个未激活或异常`} icon={Layers3} tone={activePools ? "success" : "neutral"} testId="storage-pool-active" />
            <MetricCard label="总容量" value={formatBytes(totalCapacity)} hint="libvirt 报告容量" icon={HardDrive} tone="neutral" testId="storage-capacity-total" />
            <MetricCard label="已分配" value={formatBytes(totalAllocation)} hint={`${percent(totalAllocation, totalCapacity)}% 使用率`} icon={FolderOpen} tone="warning" testId="storage-allocation-total" />
          </div>

          <div className="mt-4 space-y-4">
            <StoragePoolsCard
              pools={pools}
              selectedPool={selectedPool}
              canManage={canManage}
              manageDisabledReason={manageDisabledReason}
              onSelect={selectPool}
              onNotice={(message, testId) => { setSuccessMessage(message); setSuccessTestId(testId); }}
            />

            <Card id="storage-volumes" data-testid="storage-volumes-card" className="scroll-mt-20">
              <CardHeader>
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <CardTitle>存储卷 {selected ? `/ ${selected.name}` : ""}</CardTitle>
                    <CardDescription>{selected?.targetPath || "请先在上方存储池列表中选择一个存储池"}</CardDescription>
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    {selected ? <Badge variant={poolStateVariant(selected.state)}>{poolStateLabel[selected.state]}</Badge> : null}
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      data-testid="storage-upload-iso"
                      onClick={() => { setUploadOpen((open) => !open); setUploadError(""); }}
                      disabled={!canManage || !selectedActive || uploadMutation.isPending}
                      title={uploadDisabledReason}
                    >
                      <Upload className="h-4 w-4" />上传 ISO
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      data-testid="storage-create-volume"
                      onClick={() => { setCreateOpen((open) => !open); setCreateError(""); }}
                      disabled={!canManage || !selectedActive || createMutation.isPending}
                      title={createDisabledReason}
                    >
                      <Plus className="h-4 w-4" />新建存储卷
                    </Button>
                  </div>
                </div>
              </CardHeader>
              {uploadOpen && selected ? (
                <form onSubmit={handleUpload} className="border-y bg-muted/20 p-4" data-testid="storage-upload-form">
                  <div className="grid gap-3 sm:grid-cols-[1fr_auto] sm:items-end">
                    <div className="space-y-2">
                      <Label htmlFor="storage-upload-file">ISO 文件</Label>
                      <input
                        id="storage-upload-file"
                        data-testid="storage-upload-input"
                        type="file"
                        accept=".iso,application/x-iso9660-image"
                        onChange={(event) => { setUploadFile(event.target.files?.[0] ?? null); setUploadError(""); }}
                        className="flex h-9 w-full rounded-md border border-input bg-card px-3 py-1.5 text-sm shadow-sm file:mr-3 file:border-0 file:bg-transparent file:text-sm file:font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1"
                      />
                    </div>
                    <div className="flex items-center gap-2">
                      <Button type="button" variant="ghost" size="sm" onClick={() => { setUploadOpen(false); setUploadFile(null); setUploadError(""); }}>取消</Button>
                      <Button type="submit" size="sm" data-testid="storage-upload-submit" disabled={!uploadFile || uploadMutation.isPending}>{uploadMutation.isPending ? "上传中..." : "开始上传"}</Button>
                    </div>
                  </div>
                  <p className="mt-3 text-xs text-muted-foreground">仅支持 .iso，文件大小 1 MiB - 16 GiB。文件将以原始字节流写入 {selected.targetPath || "存储池"}，不会先落到临时文件。</p>
                  {uploadError ? (
                    <p role="alert" className="mt-3 flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">
                      <AlertTriangle className="h-3.5 w-3.5" />{uploadError}
                    </p>
                  ) : null}
                </form>
              ) : null}
              {createOpen && selected ? (
                <form onSubmit={handleCreate} className="border-y bg-muted/20 p-4" data-testid="storage-volume-create-form">
                  <div className="grid gap-3 sm:grid-cols-3">
                    <div className="space-y-2 sm:col-span-1">
                      <Label htmlFor="storage-volume-name">卷名称</Label>
                      <Input id="storage-volume-name" value={createForm.name} onChange={(event) => setCreateForm((form) => ({ ...form, name: event.target.value }))} placeholder="例如 app-data-01.qcow2" required autoComplete="off" />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="storage-volume-format">卷格式</Label>
                      <select
                        id="storage-volume-format"
                        value={createForm.format}
                        onChange={(event) => setCreateForm((form) => ({ ...form, format: event.target.value as StorageVolumeFormat }))}
                        className="flex h-9 w-full rounded-md border border-input bg-card px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1"
                      >
                        <option value="qcow2">qcow2</option>
                        <option value="raw">raw</option>
                      </select>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="storage-volume-capacity">容量 (GiB)</Label>
                      <Input id="storage-volume-capacity" type="number" min={0.001} max={16384} step="any" value={createForm.capacityGiB} onChange={(event) => setCreateForm((form) => ({ ...form, capacityGiB: event.target.value }))} required />
                    </div>
                  </div>
                  <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
                    <p className="text-xs text-muted-foreground">名称需以 .qcow2 或 .raw 结尾，并与卷格式一致。卷由 libvirt 在 {selected.targetPath || "存储池"} 中创建。</p>
                    <div className="flex items-center gap-2">
                      <Button type="button" variant="ghost" size="sm" onClick={() => { setCreateOpen(false); setCreateError(""); }}>取消</Button>
                      <Button type="submit" size="sm" disabled={createMutation.isPending}>{createMutation.isPending ? "创建中..." : "创建存储卷"}</Button>
                    </div>
                  </div>
                  {createError ? (
                    <p role="alert" className="mt-3 flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">
                      <AlertTriangle className="h-3.5 w-3.5" />{createError}
                    </p>
                  ) : null}
                </form>
              ) : null}
              <CardContent className="p-0">
                {!selected ? <p className="p-5 text-sm text-muted-foreground">没有可用的存储池。</p> : null}
                {selected && volumesQuery.isLoading ? <LoadingState label="正在读取存储卷" /> : null}
                {selected && volumesQuery.isError ? <ErrorState message={volumesQuery.error instanceof Error ? volumesQuery.error.message : "无法读取存储卷"} onRetry={() => void volumesQuery.refetch()} className="m-4" /> : null}
                {selected && volumesQuery.data ? (
                  <>
                    <div className="flex items-center justify-between border-b px-4 py-2 text-xs text-muted-foreground">
                      <span data-testid="storage-volume-total">共 {volumesQuery.data.total} 个卷</span>
                      <span>{formatBytes(selected.allocationBytes)} / {formatBytes(selected.capacityBytes)}</span>
                    </div>
                    {volumesQuery.data.items.length ? (
                      <Table>
                        <TableHeader><TableRow><TableHead>名称</TableHead><TableHead>格式</TableHead><TableHead>容量</TableHead><TableHead>已分配</TableHead><TableHead>路径</TableHead><TableHead>操作</TableHead></TableRow></TableHeader>
                        <TableBody>
                          {volumesQuery.data.items.map((volume) => (
                            <TableRow key={volume.path || volume.name} data-testid={`storage-volume-row-${volume.name}`}>
                              <TableCell><p className="font-medium">{volume.name}</p><p className="text-[10px] text-muted-foreground">{volume.type}</p></TableCell>
                              <TableCell><Badge variant="outline">{volume.format || "-"}</Badge></TableCell>
                              <TableCell className="tabular-nums">{formatBytes(volume.capacityBytes)}</TableCell>
                              <TableCell className="tabular-nums">{formatBytes(volume.allocationBytes)}</TableCell>
                              <TableCell><span className="block max-w-72 truncate font-mono text-xs text-muted-foreground" title={volume.path}>{volume.path || "-"}</span></TableCell>
                              <TableCell>
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="icon-sm"
                                  data-testid={`delete-volume-${volume.name}`}
                                  aria-label={`删除存储卷 ${volume.name}`}
                                  title={canManage ? "删除未被虚拟机占用的存储卷" : createDisabledReason}
                                  disabled={!canManage || !selectedActive || deleteMutation.isPending}
                                  onClick={() => openDelete(volume)}
                                >
                                  <Trash2 className="h-4 w-4" />
                                </Button>
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    ) : <p className="p-5 text-sm text-muted-foreground">该存储池中没有卷。</p>}
                  </>
                ) : null}
              </CardContent>
            </Card>
          </div>
        </>
      ) : null}

      <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => { if (!open) { setDeleteTarget(null); setDeleteError(""); } }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除存储卷？</AlertDialogTitle>
            <AlertDialogDescription>
              将从 <span className="font-medium text-foreground">{selected?.name}</span> 存储池永久删除 <span className="font-medium text-foreground">{deleteTarget?.name}</span>。被虚拟机引用的卷不能删除。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="space-y-2">
            <Label htmlFor="delete-volume-name">输入存储卷名称确认</Label>
            <Input id="delete-volume-name" value={deleteName} onChange={(event) => setDeleteName(event.target.value)} placeholder={deleteTarget?.name} autoComplete="off" />
          </div>
          {deleteError ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert">{deleteError}</p> : null}
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <Button
              variant="destructive"
              data-testid="confirm-delete-volume"
              disabled={!deleteTarget || deleteName !== deleteTarget.name || deleteMutation.isPending}
              onClick={() => {
                if (!deleteTarget || !selected) return;
                setDeleteError("");
                deleteMutation.mutate({ pool: selected.name, name: deleteTarget.name });
              }}
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

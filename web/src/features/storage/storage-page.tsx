import { useEffect, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, Database, FolderOpen, HardDrive, Layers3, LoaderCircle, Plus, Repeat2, Trash2, Upload } from "lucide-react";
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
import { Modal } from "@/components/ui/modal";
import { Progress } from "@/components/ui/progress";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useSessionQuery } from "@/features/auth/use-session";
import { StoragePoolsCard } from "@/features/storage/storage-pools-card";
import { storagePoolsQueryKey, useStoragePoolsQuery, useStorageVolumesQuery } from "@/features/storage/use-storage-query";
import { useTaskQuery } from "@/features/tasks/use-tasks-query";
import { queryKeys } from "@/lib/query-keys";
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
  const [convertTarget, setConvertTarget] = useState<StorageVolume | null>(null);
  const [convertPool, setConvertPool] = useState("");
  const [convertName, setConvertName] = useState("");
  const [convertFormat, setConvertFormat] = useState<StorageVolumeFormat>("qcow2");
  const [convertError, setConvertError] = useState("");
  const [convertTaskID, setConvertTaskID] = useState<string | null>(null);
  const [flattenTarget, setFlattenTarget] = useState<StorageVolume | null>(null);
  const [flattenPool, setFlattenPool] = useState("");
  const [flattenName, setFlattenName] = useState("");
  const [flattenError, setFlattenError] = useState("");
  const [flattenTaskID, setFlattenTaskID] = useState<string | null>(null);
  const pools = poolsQuery.data?.items ?? [];
  const volumesQuery = useStorageVolumesQuery(selectedPool);
  const selected = pools.find((pool) => pool.name === selectedPool);
  const convertTaskQuery = useTaskQuery(convertTaskID);
  const flattenTaskQuery = useTaskQuery(flattenTaskID);

  const createMutation = useMutation({
    mutationFn: ({ pool, input }: { pool: string; input: StorageVolumeCreateInput }) => api.createStorageVolume(pool, input),
    onSuccess: (volume, variables) => {
      setSuccessMessage(`存储卷 ${volume.name} 已创建。`);
      setSuccessTestId("storage-create-success");
      setCreateError("");
      setCreateOpen(false);
      setCreateForm(initialCreateForm);
      void queryClient.invalidateQueries({ queryKey: queryKeys.storage.volumes(variables.pool) });
      void queryClient.invalidateQueries({ queryKey: storagePoolsQueryKey });
    },
    onError: (cause) => {
      setCreateError(cause instanceof ApiError ? cause.message : "创建存储卷失败，请稍后重试。");
    },
  });

  const uploadMutation = useMutation({
    mutationFn: ({ pool, file }: { pool: string; file: File }) => api.uploadStorageVolume(pool, file),
    onSuccess: (volume, variables) => {
      setSuccessMessage(`存储卷 ${volume.name} 已上传。`);
      setSuccessTestId("storage-upload-success");
      setUploadError("");
      setUploadOpen(false);
      setUploadFile(null);
      void queryClient.invalidateQueries({ queryKey: queryKeys.storage.volumes(variables.pool) });
      void queryClient.invalidateQueries({ queryKey: storagePoolsQueryKey });
    },
    onError: (cause) => {
      setUploadError(cause instanceof ApiError ? cause.message : "上传存储卷失败，请稍后重试。");
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
      void queryClient.invalidateQueries({ queryKey: queryKeys.storage.volumes(variables.pool) });
      void queryClient.invalidateQueries({ queryKey: storagePoolsQueryKey });
    },
    onError: (cause) => {
      setDeleteError(cause instanceof ApiError ? cause.message : "删除存储卷失败，请稍后重试。");
    },
  });

  const convertMutation = useMutation({
    mutationFn: ({ pool, volume, targetPool, targetName, targetFormat }: { pool: string; volume: string; targetPool: string; targetName: string; targetFormat: StorageVolumeFormat }) => api.convertStorageVolume(pool, volume, { targetPool, targetName, targetFormat }),
    onSuccess: (task) => {
      setConvertError("");
      setConvertTaskID(task.id);
      setSuccessMessage(`存储卷转换任务已提交：${task.step}。`);
      setSuccessTestId("storage-convert-success");
    },
    onError: (cause) => setConvertError(cause instanceof ApiError ? cause.message : "提交存储卷转换任务失败，请稍后重试。"),
  });

  const flattenMutation = useMutation({
    mutationFn: ({ pool, volume, targetPool, targetName }: { pool: string; volume: string; targetPool: string; targetName: string }) => api.flattenStorageVolume(pool, volume, { targetPool, targetName }),
    onSuccess: (task) => {
      setFlattenError("");
      setFlattenTaskID(task.id);
      setSuccessMessage(`链接克隆展平任务已提交：${task.step}。`);
      setSuccessTestId("storage-flatten-success");
    },
    onError: (cause) => setFlattenError(cause instanceof ApiError ? cause.message : "提交链接克隆展平任务失败，请稍后重试。"),
  });

  useEffect(() => {
    if (!pools.length) return;
    if (selectedPool && pools.some((pool) => pool.name === selectedPool)) return;
    const preferred = pools.find((pool) => pool.state === "active") ?? pools[0];
    setSelectedPool(preferred.name);
  }, [pools, selectedPool]);

  useEffect(() => {
    if (!convertTaskQuery.data || !convertTarget || !selected) return;
    if (convertTaskQuery.data.status === "succeeded") {
      setSuccessMessage(`存储卷 ${convertTarget.name} 已转换为 ${convertTaskQuery.data.step.replace(/^存储卷已转换为\s*/, "")}。`);
      setSuccessTestId("storage-convert-success");
      void queryClient.invalidateQueries({ queryKey: queryKeys.storage.volumes(selected.name) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.storage.volumes(convertPool) });
      void queryClient.invalidateQueries({ queryKey: storagePoolsQueryKey });
    } else if (convertTaskQuery.data.status === "failed") {
      setConvertError(convertTaskQuery.data.error || "存储卷转换任务失败。");
    }
  }, [convertTaskQuery.data, convertPool, convertTarget, queryClient, selected]);

  useEffect(() => {
    if (!flattenTaskQuery.data || !flattenTarget || !selected) return;
    if (flattenTaskQuery.data.status === "succeeded") {
      setSuccessMessage(`链接克隆 ${flattenTarget.name} 已展平为独立卷。`);
      setSuccessTestId("storage-flatten-success");
      void queryClient.invalidateQueries({ queryKey: queryKeys.storage.volumes(selected.name) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.storage.volumes(flattenPool) });
      void queryClient.invalidateQueries({ queryKey: storagePoolsQueryKey });
    } else if (flattenTaskQuery.data.status === "failed") {
      setFlattenError(flattenTaskQuery.data.error || "链接克隆展平任务失败。");
    }
  }, [flattenPool, flattenTarget, flattenTaskQuery.data, queryClient, selected]);

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
    : !selectedActive ? "只有活跃存储池可以上传文件" : selected?.type === "logical" ? "LVM logical 池不支持文件上传，请直接创建 raw 卷" : "";
  const selectedLogical = selected?.type === "logical";
  const volumeFormat: StorageVolumeFormat = selectedLogical ? "raw" : createForm.format;

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

  function openConvert(volume: StorageVolume) {
    const format: StorageVolumeFormat = volume.format === "qcow2" ? "raw" : "qcow2";
    setConvertTarget(volume);
    setConvertPool(selected?.name || selectedPool);
    setConvertFormat(format);
    setConvertName(replaceVolumeExtension(volume.name, format));
    setConvertError("");
    setConvertTaskID(null);
  }

  function openFlatten(volume: StorageVolume) {
    const stem = volume.name.replace(/\.qcow2$/i, "");
    setFlattenTarget(volume);
    setFlattenPool(selected?.name || selectedPool);
    setFlattenName(`${stem || "volume"}-flat.qcow2`);
    setFlattenError("");
    setFlattenTaskID(null);
  }

  function submitFlatten() {
    if (!selected || !flattenTarget) return;
    if (!flattenPool || !flattenName.trim()) {
      setFlattenError("请选择目标存储池并填写目标卷名称。");
      return;
    }
    if (!flattenName.trim().toLowerCase().endsWith(".qcow2")) {
      setFlattenError("展平后的目标卷名称必须以 .qcow2 结尾。");
      return;
    }
    setFlattenError("");
    setFlattenTaskID(null);
    flattenMutation.mutate({ pool: selected.name, volume: flattenTarget.name, targetPool: flattenPool, targetName: flattenName.trim() });
  }

  function submitConvert() {
    if (!selected || !convertTarget) return;
    if (!convertPool || !convertName.trim()) {
      setConvertError("请选择目标存储池并填写目标卷名称。");
      return;
    }
    const extension = convertFormat === "qcow2" ? ".qcow2" : ".raw";
    if (!convertName.trim().toLowerCase().endsWith(extension)) {
      setConvertError(`目标卷名称必须以 ${extension} 结尾。`);
      return;
    }
    setConvertError("");
    setConvertTaskID(null);
    convertMutation.mutate({ pool: selected.name, volume: convertTarget.name, targetPool: convertPool, targetName: convertName.trim(), targetFormat: convertFormat });
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
        format: volumeFormat,
        capacityBytes: Math.round(capacityGiB * 1024 ** 3),
      },
    });
  }

  function handleUpload(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selected || !selectedActive || !canManage || !uploadFile) return;
    if (selected.type === "logical") {
      setUploadError("LVM logical 池不支持上传文件。");
      return;
    }
    if (!/\.(iso|qcow2|raw)$/i.test(uploadFile.name)) {
      setUploadError("请选择 .iso、.qcow2 或 .raw 文件。");
      return;
    }
    if (uploadFile.size < 1024 ** 2 || uploadFile.size > 256 * 1024 ** 3) {
      setUploadError("文件大小必须在 1 MiB 到 256 GiB 之间。");
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
                      disabled={!canManage || !selectedActive || selectedLogical || uploadMutation.isPending}
                      title={uploadDisabledReason}
                    >
                      <Upload className="h-4 w-4" />上传镜像/ISO
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
                      <Label htmlFor="storage-upload-file">镜像或 ISO 文件</Label>
                      <input
                        id="storage-upload-file"
                        data-testid="storage-upload-input"
                        type="file"
                        accept=".iso,.qcow2,.raw,application/x-iso9660-image,application/octet-stream"
                        onChange={(event) => { setUploadFile(event.target.files?.[0] ?? null); setUploadError(""); }}
                        className="flex h-9 w-full rounded-md border border-input bg-card px-3 py-1.5 text-sm shadow-sm file:mr-3 file:border-0 file:bg-transparent file:text-sm file:font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1"
                      />
                    </div>
                    <div className="flex items-center gap-2">
                      <Button type="button" variant="ghost" size="sm" onClick={() => { setUploadOpen(false); setUploadFile(null); setUploadError(""); }}>取消</Button>
                      <Button type="submit" size="sm" data-testid="storage-upload-submit" disabled={!uploadFile || uploadMutation.isPending}>{uploadMutation.isPending ? "上传中..." : "开始上传"}</Button>
                    </div>
                  </div>
                  <p className="mt-3 text-xs text-muted-foreground">支持 .iso、.qcow2 和 .raw，文件大小 1 MiB - 256 GiB。文件将以原始字节流写入 {selected.targetPath || "存储池"}，不会先落到临时文件。</p>
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
                        value={volumeFormat}
                        onChange={(event) => setCreateForm((form) => ({ ...form, format: event.target.value as StorageVolumeFormat }))}
                        className="flex h-9 w-full rounded-md border border-input bg-card px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1"
                      >
                        <option value="qcow2" disabled={selectedLogical}>qcow2</option>
                        <option value="raw">raw</option>
                      </select>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="storage-volume-capacity">容量 (GiB)</Label>
                      <Input id="storage-volume-capacity" type="number" min={0.001} max={16384} step="any" value={createForm.capacityGiB} onChange={(event) => setCreateForm((form) => ({ ...form, capacityGiB: event.target.value }))} required />
                    </div>
                  </div>
                  <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
                    <p className="text-xs text-muted-foreground">{selectedLogical ? "LVM logical 池仅支持 .raw 块卷，由 libvirt 在卷组中创建逻辑卷。" : `名称需以 .qcow2 或 .raw 结尾，并与卷格式一致。卷由 libvirt 在 ${selected.targetPath || "存储池"} 中创建。`}</p>
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
                    {(volumesQuery.data.items ?? []).length ? (
                      <Table className="min-w-[1080px]">
                        <TableHeader><TableRow><TableHead>名称</TableHead><TableHead>格式</TableHead><TableHead>容量</TableHead><TableHead>已分配</TableHead><TableHead>路径</TableHead><TableHead>使用关系</TableHead><TableHead>操作</TableHead></TableRow></TableHeader>
                        <TableBody>
                          {(volumesQuery.data.items ?? []).map((volume) => {
                            const usedBy = volume.usedBy ?? [];
                            const inUse = usedBy.length > 0;
                            return <TableRow key={volume.path || volume.name} data-testid={`storage-volume-row-${volume.name}`}>
                              <TableCell><p className="font-medium">{volume.name}</p><p className="text-[10px] text-muted-foreground">{volume.type}</p></TableCell>
                              <TableCell><div className="flex flex-wrap gap-1"><Badge variant="outline">{volume.format || "-"}</Badge>{volume.linked ? <Badge variant="warning">链接克隆</Badge> : null}</div></TableCell>
                              <TableCell className="tabular-nums">{formatBytes(volume.capacityBytes)}</TableCell>
                              <TableCell className="tabular-nums">{formatBytes(volume.allocationBytes)}</TableCell>
                              <TableCell><span className="block max-w-72 truncate font-mono text-xs text-muted-foreground" title={volume.path}>{volume.path || "-"}</span></TableCell>
                              <TableCell data-testid={`storage-volume-usage-${volume.name}`}>
                                {inUse ? <div className="space-y-1"><Badge variant="warning">使用中</Badge><p className="max-w-56 text-xs text-muted-foreground">{usedBy.map((usage) => `${usage.vmName} · ${usage.target}${usage.readOnly ? "（只读）" : ""}`).join("，")}</p></div> : <Badge variant="secondary">空闲</Badge>}
                              </TableCell>
                              <TableCell>
                                <div className="flex flex-wrap justify-end gap-1">
                                  {volume.linked && volume.format === "qcow2" ? (
                                    <Button
                                      type="button"
                                      variant="outline"
                                      size="sm"
                                      data-testid={`flatten-volume-${volume.name}`}
                                      aria-label={`展平链接克隆存储卷 ${volume.name}`}
                                      title={inUse ? `被 ${usedBy.map((usage) => usage.vmName).join("、")} 引用，不能展平` : selectedActive ? "生成独立 qcow2 卷，源卷保留" : createDisabledReason}
                                      disabled={!canManage || !selectedActive || flattenMutation.isPending || inUse}
                                      onClick={() => openFlatten(volume)}
                                    >
                                      <Layers3 className="h-3.5 w-3.5" />展平
                                    </Button>
                                  ) : null}
                                  {(volume.format === "qcow2" || volume.format === "raw") && !/\.iso$/i.test(volume.name) ? (
                                    <Button
                                      type="button"
                                      variant="outline"
                                      size="sm"
                                      data-testid={`convert-volume-${volume.name}`}
                                      aria-label={`转换或复制存储卷 ${volume.name}`}
                                      title={inUse ? `被 ${usedBy.map((usage) => usage.vmName).join("、")} 引用，不能转换` : volume.linked ? "链接克隆需要先展平，再执行转换或复制" : selectedActive ? "转换为 qcow2/raw 或复制到其他存储池" : createDisabledReason}
                                      disabled={!canManage || !selectedActive || convertMutation.isPending || inUse || volume.linked}
                                      onClick={() => openConvert(volume)}
                                    >
                                      <Repeat2 className="h-3.5 w-3.5" />转换/复制
                                    </Button>
                                  ) : null}
                                  <Button
                                    type="button"
                                    variant="ghost"
                                    size="sm"
                                    data-testid={`delete-volume-${volume.name}`}
                                    aria-label={`删除存储卷 ${volume.name}`}
                                    title={inUse ? `被 ${usedBy.map((usage) => usage.vmName).join("、")} 引用，不能删除` : canManage ? "删除未被虚拟机占用的存储卷" : createDisabledReason}
                                    disabled={!canManage || !selectedActive || deleteMutation.isPending || inUse}
                                    onClick={() => openDelete(volume)}
                                  >
                                    <Trash2 className="h-3.5 w-3.5" />删除
                                  </Button>
                                </div>
                              </TableCell>
                            </TableRow>
                          })}
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
          {deleteTarget?.usedBy?.length ? <p role="alert" className="rounded-md border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning">该卷正被 {deleteTarget.usedBy.map((usage) => `${usage.vmName}（${usage.target}）`).join("、")} 使用，请先移除对应磁盘设备。</p> : null}
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

      <Modal
        open={convertTarget !== null}
        onClose={() => { if (!convertMutation.isPending) { setConvertTarget(null); setConvertError(""); setConvertTaskID(null); } }}
        title={`转换或复制 ${convertTarget?.name || ""}`}
        description="通过 libvirt Storage Volume API 创建独立目标卷并写入 qcow2/raw 格式。源卷保持不变；链接克隆源和运行中虚拟机使用的卷会被拒绝。"
        testId="storage-convert-modal"
        footer={<><Button variant="ghost" disabled={convertMutation.isPending} onClick={() => { setConvertTarget(null); setConvertError(""); setConvertTaskID(null); }}>关闭</Button><Button data-testid="storage-convert-submit" disabled={convertMutation.isPending || !convertPool || !convertName.trim()} onClick={submitConvert}>{convertMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Repeat2 className="h-4 w-4" />}{convertMutation.isPending ? "正在提交..." : "提交转换任务"}</Button></>}
      >
        <div className="space-y-4">
          <div className="grid gap-2 rounded-md border bg-muted/20 p-3 text-sm sm:grid-cols-3">
            <div><p className="text-xs text-muted-foreground">源存储池</p><p className="mt-1 font-mono">{selected?.name || "-"}</p></div>
            <div><p className="text-xs text-muted-foreground">源卷</p><p className="mt-1 truncate font-mono">{convertTarget?.name || "-"}</p></div>
            <div><p className="text-xs text-muted-foreground">源格式</p><p className="mt-1">{convertTarget?.format || "-"}</p></div>
          </div>
          <div className="space-y-2"><Label htmlFor="storage-convert-pool">目标存储池</Label><select id="storage-convert-pool" data-testid="storage-convert-pool" value={convertPool} onChange={(event) => setConvertPool(event.target.value)} disabled={convertMutation.isPending} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">{(poolsQuery.data?.items ?? []).filter((pool) => pool.state === "active").map((pool) => <option key={pool.name} value={pool.name}>{pool.name} · {pool.type}</option>)}</select></div>
          <div className="grid gap-3 sm:grid-cols-[1fr_10rem]">
            <div className="space-y-2"><Label htmlFor="storage-convert-name">目标卷名称</Label><Input id="storage-convert-name" data-testid="storage-convert-name" value={convertName} onChange={(event) => setConvertName(event.target.value)} disabled={convertMutation.isPending} /></div>
            <div className="space-y-2"><Label htmlFor="storage-convert-format">目标格式</Label><select id="storage-convert-format" data-testid="storage-convert-format" value={convertFormat} onChange={(event) => { const format = event.target.value as StorageVolumeFormat; setConvertFormat(format); setConvertName((name) => replaceVolumeExtension(name, format)); }} disabled={convertMutation.isPending} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm"><option value="qcow2">qcow2</option><option value="raw">raw</option></select></div>
          </div>
          {convertTaskQuery.data ? (
            <div className="space-y-2 rounded-md border bg-card px-3 py-2" data-testid="storage-convert-task" data-status={convertTaskQuery.data.status}>
              <div className="flex flex-wrap items-center gap-2"><Badge variant={convertTaskQuery.data.status === "succeeded" ? "success" : convertTaskQuery.data.status === "failed" ? "destructive" : "warning"}>{convertTaskQuery.data.status === "succeeded" ? "已完成" : convertTaskQuery.data.status === "failed" ? "失败" : "执行中"}</Badge><span className="text-xs">{convertTaskQuery.data.step}</span><span className="ml-auto text-xs tabular-nums text-muted-foreground">{convertTaskQuery.data.progress}%</span></div>
              <Progress value={convertTaskQuery.data.progress} />
              {convertTaskQuery.data.error ? <p className="text-xs text-destructive">{convertTaskQuery.data.error}</p> : null}
            </div>
          ) : null}
          {convertError ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{convertError}</p> : null}
          <p className="text-xs text-muted-foreground">转换会完整复制数据，耗时取决于卷容量和存储池吞吐。任务可在任务中心查看，源卷不会被修改或删除。</p>
        </div>
      </Modal>

      <Modal
        open={flattenTarget !== null}
        onClose={() => { if (!flattenMutation.isPending) { setFlattenTarget(null); setFlattenError(""); setFlattenTaskID(null); } }}
        title={`展平链接克隆 ${flattenTarget?.name || ""}`}
        description="通过 libvirt 复制完整数据并生成独立 qcow2 卷。源链接克隆保持不变，虚拟机引用不会自动切换。"
        testId="storage-flatten-modal"
        footer={<><Button variant="ghost" disabled={flattenMutation.isPending} onClick={() => { setFlattenTarget(null); setFlattenError(""); setFlattenTaskID(null); }}>关闭</Button><Button data-testid="storage-flatten-submit" disabled={flattenMutation.isPending || !flattenPool || !flattenName.trim()} onClick={submitFlatten}>{flattenMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Layers3 className="h-4 w-4" />}{flattenMutation.isPending ? "正在提交..." : "提交展平任务"}</Button></>}
      >
        <div className="space-y-4">
          <div className="grid gap-2 rounded-md border bg-muted/20 p-3 text-sm sm:grid-cols-3">
            <div><p className="text-xs text-muted-foreground">源存储池</p><p className="mt-1 font-mono">{selected?.name || "-"}</p></div>
            <div><p className="text-xs text-muted-foreground">源卷</p><p className="mt-1 truncate font-mono">{flattenTarget?.name || "-"}</p></div>
            <div><p className="text-xs text-muted-foreground">当前分配</p><p className="mt-1">{flattenTarget ? formatBytes(flattenTarget.allocationBytes) : "-"}</p></div>
          </div>
          <div className="space-y-2"><Label htmlFor="storage-flatten-pool">目标存储池</Label><select id="storage-flatten-pool" data-testid="storage-flatten-pool" value={flattenPool} onChange={(event) => setFlattenPool(event.target.value)} disabled={flattenMutation.isPending} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">{(poolsQuery.data?.items ?? []).filter((pool) => pool.state === "active").map((pool) => <option key={pool.name} value={pool.name}>{pool.name} · {pool.type}</option>)}</select></div>
          <div className="space-y-2"><Label htmlFor="storage-flatten-name">目标卷名称</Label><Input id="storage-flatten-name" data-testid="storage-flatten-name" value={flattenName} onChange={(event) => setFlattenName(event.target.value)} disabled={flattenMutation.isPending} /></div>
          {flattenTaskQuery.data ? (
            <div className="space-y-2 rounded-md border bg-card px-3 py-2" data-testid="storage-flatten-task" data-status={flattenTaskQuery.data.status}>
              <div className="flex flex-wrap items-center gap-2"><Badge variant={flattenTaskQuery.data.status === "succeeded" ? "success" : flattenTaskQuery.data.status === "failed" ? "destructive" : "warning"}>{flattenTaskQuery.data.status === "succeeded" ? "已完成" : flattenTaskQuery.data.status === "failed" ? "失败" : "执行中"}</Badge><span className="text-xs">{flattenTaskQuery.data.step}</span><span className="ml-auto text-xs tabular-nums text-muted-foreground">{flattenTaskQuery.data.progress}%</span></div>
              <Progress value={flattenTaskQuery.data.progress} />
              {flattenTaskQuery.data.error ? <p className="text-xs text-destructive">{flattenTaskQuery.data.error}</p> : null}
            </div>
          ) : null}
          {flattenError ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{flattenError}</p> : null}
          <p className="text-xs text-muted-foreground">展平会复制完整数据，耗时取决于底链大小和存储池吞吐。源卷及其虚拟机引用保持不变。</p>
        </div>
      </Modal>
    </>
  );
}

function replaceVolumeExtension(name: string, format: StorageVolumeFormat) {
  const stem = name.replace(/\.(qcow2|raw)$/i, "");
  return `${stem || "volume"}.${format}`;
}

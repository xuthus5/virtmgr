import { useEffect, useMemo, useState, type FormEvent } from "react";
import { Link } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, ArrowLeft, CheckCircle2, HardDrive, LoaderCircle, Server, Upload } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { Task, VMDiskBus, VMFirmware, VMNICModel, StorageVolume } from "@/api/types";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useSessionQuery } from "@/features/auth/use-session";
import { useNetworksQuery } from "@/features/networks/use-networks-query";
import { useStoragePoolsQuery, useStorageVolumesQuery } from "@/features/storage/use-storage-query";
import { buildVMImportInput, importDiskCapacityError, importFilenameError, importVolumeFormat, isSupportedImportVolume, vmImportTargetDiskName, type VMImportFormValues } from "@/features/vms/vm-import-model";
import { formatBytes } from "@/lib/utils";

const initialForm: VMImportFormValues = {
  name: "",
  sourcePool: "",
  sourceVolume: "",
  targetPool: "",
  vcpus: "2",
  memoryMiB: "2048",
  diskCapacityGiB: "20",
  diskBus: "virtio",
  network: "",
  nicModel: "virtio",
  firmware: "bios",
  secureBoot: false,
  guestAgent: true,
  autostart: false,
  start: false,
};

export function VMImportPage() {
  const session = useSessionQuery();
  const poolsQuery = useStoragePoolsQuery();
  const networksQuery = useNetworksQuery();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<VMImportFormValues>(initialForm);
  const [sourceMode, setSourceMode] = useState<"existing" | "upload">("existing");
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [uploadError, setUploadError] = useState("");
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [task, setTask] = useState<Task | null>(null);
  const activePools = useMemo(() => (poolsQuery.data?.items ?? []).filter((pool) => pool.state === "active"), [poolsQuery.data]);
  const activeNetworks = useMemo(() => (networksQuery.data?.items ?? []).filter((network) => network.active), [networksQuery.data]);
  const sourceVolumesQuery = useStorageVolumesQuery(form.sourcePool);
  const sourceVolumes = useMemo(() => (sourceVolumesQuery.data?.items ?? []).filter(isSupportedImportVolume), [sourceVolumesQuery.data]);
  const selectedVolume = sourceVolumes.find((volume) => volume.name === form.sourceVolume);
  const selectedFormat = selectedVolume ? importVolumeFormat(selectedVolume) : null;
  const selectedISO = selectedFormat === "iso";
  const readOnly = Boolean(session.data?.readOnly);
  const canImport = Boolean(session.data) && !readOnly && session.data?.user.role !== "viewer";
  const disabledReason = readOnly ? "系统当前处于只读模式" : "当前账号没有导入虚拟机权限";

  useEffect(() => {
    if (!activePools.length) return;
    const preferred = activePools.find((pool) => pool.name === "default") ?? activePools[0];
    setForm((current) => {
      const sourcePool = activePools.some((pool) => pool.name === current.sourcePool) ? current.sourcePool : preferred.name;
      const targetPool = activePools.some((pool) => pool.name === current.targetPool) ? current.targetPool : sourcePool;
      if (sourcePool === current.sourcePool && targetPool === current.targetPool) return current;
      return { ...current, sourcePool, targetPool };
    });
  }, [activePools]);

  useEffect(() => {
    if (!activeNetworks.length) return;
    setForm((current) => {
      if (activeNetworks.some((network) => network.name === current.network)) return current;
      const preferred = activeNetworks.find((network) => network.name === "default") ?? activeNetworks[0];
      return { ...current, network: preferred.name };
    });
  }, [activeNetworks]);

  const uploadMutation = useMutation({
    mutationFn: async () => {
      if (!uploadFile) throw new Error("请选择 QCOW2、RAW 或 ISO 文件");
      const fileError = importFilenameError(uploadFile.name);
      if (fileError) throw new Error(fileError);
      return api.uploadStorageVolume(form.targetPool, uploadFile);
    },
    onSuccess: (volume) => {
      setUploadError("");
      setNotice(`磁盘 ${volume.name} 已上传，并已选为导入来源。`);
      setForm((current) => ({ ...current, sourcePool: current.targetPool, sourceVolume: volume.name }));
      setSourceMode("existing");
      setUploadFile(null);
      void queryClient.invalidateQueries({ queryKey: ["storage", "volumes", form.targetPool] });
    },
    onError: (cause) => {
      setNotice("");
      setUploadError(cause instanceof ApiError ? cause.message : cause instanceof Error ? cause.message : "磁盘上传失败");
    },
  });

  const importMutation = useMutation({
    mutationFn: api.importVM,
    onSuccess: (result) => {
      setError("");
      setTask(result);
      void queryClient.invalidateQueries({ queryKey: ["tasks"] });
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
    },
    onError: (cause) => {
      setTask(null);
      setError(cause instanceof ApiError ? cause.message : "虚拟机导入请求失败，请稍后重试。");
    },
  });

  function update<K extends keyof VMImportFormValues>(key: K, value: VMImportFormValues[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setTask(null);
    if (!canImport) {
      setError(disabledReason);
      return;
    }
    if (!selectedVolume || !selectedFormat) {
      setError("请选择可用的 QCOW2、RAW 或 ISO 来源文件");
      return;
    }
    if (selectedISO) {
      const capacityError = importDiskCapacityError(form.diskCapacityGiB);
      if (capacityError) {
        setError(capacityError);
        return;
      }
    }
    importMutation.mutate(buildVMImportInput(form));
  }

  return (
    <div className="min-w-0" data-testid="vm-import-page">
      <PageHeader
        title="导入虚拟机"
        breadcrumb="总览 / 虚拟机 / 导入"
        description="从已有 RAW/QCOW2 磁盘或 ISO 安装介质创建虚拟机；来源卷不会被修改或删除。"
        actions={<Button asChild variant="outline"><Link to="/vms"><ArrowLeft className="h-4 w-4" />返回虚拟机</Link></Button>}
      />

      {!canImport ? (
        <div className="mb-4 flex items-start gap-3 rounded-lg border border-warning/35 bg-warning/10 px-4 py-3 text-sm text-warning" role="status" data-testid="vm-import-disabled">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <p>{disabledReason}。可查看导入说明，但无法上传或提交导入任务。</p>
        </div>
      ) : null}

      <form onSubmit={submit} className="grid gap-4 xl:grid-cols-[1.15fr_0.85fr]">
        <div className="min-w-0 space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2"><HardDrive className="h-4 w-4 text-primary" />磁盘或 ISO 来源</CardTitle>
              <CardDescription>支持 RAW/QCOW2 系统盘和 ISO 安装介质；OVA、OVF、VMDK、VHDX 及格式转换不在当前范围内。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid gap-2 sm:grid-cols-2" role="radiogroup" aria-label="导入方式">
                <label className={`flex cursor-pointer items-start gap-3 rounded-lg border p-3 text-sm ${sourceMode === "existing" ? "border-primary bg-primary/5" : ""}`}>
                  <input type="radio" name="vm-import-mode" value="existing" checked={sourceMode === "existing"} onChange={() => setSourceMode("existing")} disabled={!canImport} className="mt-0.5 h-4 w-4" />
                  <span><span className="block font-medium">选择已有存储卷</span><span className="mt-0.5 block text-xs text-muted-foreground">来源卷必须位于活跃存储池。</span></span>
                </label>
                <label className={`flex cursor-pointer items-start gap-3 rounded-lg border p-3 text-sm ${sourceMode === "upload" ? "border-primary bg-primary/5" : ""}`}>
                  <input type="radio" name="vm-import-mode" value="upload" data-testid="vm-import-source-upload" checked={sourceMode === "upload"} onChange={() => setSourceMode("upload")} disabled={!canImport} className="mt-0.5 h-4 w-4" />
                  <span><span className="block font-medium">上传磁盘或 ISO 后导入</span><span className="mt-0.5 block text-xs text-muted-foreground">上传到目标池；ISO 会作为只读安装介质使用。</span></span>
                </label>
              </div>

              {sourceMode === "existing" ? (
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="min-w-0 space-y-2">
                    <Label htmlFor="vm-import-source-pool">来源存储池</Label>
                    <select id="vm-import-source-pool" value={form.sourcePool} onChange={(event) => setForm((current) => ({ ...current, sourcePool: event.target.value, sourceVolume: "" }))} disabled={!canImport || !activePools.length} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                      {!activePools.length ? <option value="">没有活跃存储池</option> : null}
                      {activePools.map((pool) => <option key={pool.name} value={pool.name}>{pool.name} · {formatBytes(pool.availableBytes)} 可用</option>)}
                    </select>
                  </div>
                  <div className="min-w-0 space-y-2">
                    <Label htmlFor="vm-import-source-volume">来源磁盘或 ISO</Label>
                    <select id="vm-import-source-volume" value={form.sourceVolume} onChange={(event) => update("sourceVolume", event.target.value)} disabled={!canImport || !form.sourcePool || sourceVolumesQuery.isLoading} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                      <option value="">{sourceVolumesQuery.isLoading ? "正在读取存储卷..." : sourceVolumes.length ? "请选择磁盘或 ISO" : "没有可导入的 RAW/QCOW2/ISO 卷"}</option>
                      {sourceVolumes.map((volume: StorageVolume) => <option key={volume.name} value={volume.name}>{volume.name} · {volume.format} · {formatBytes(volume.capacityBytes)}</option>)}
                    </select>
                    {sourceVolumesQuery.isError ? <p className="text-xs text-destructive">无法读取该存储池的卷，请确认存储池处于活跃状态。</p> : null}
                  </div>
                </div>
              ) : (
                <div className="space-y-3 rounded-lg border bg-muted/20 p-4">
                  <div className="space-y-2">
                    <Label htmlFor="vm-import-file">磁盘或 ISO 文件</Label>
                    <Input id="vm-import-file" type="file" accept=".qcow2,.raw,.iso" disabled={!canImport || uploadMutation.isPending} onChange={(event) => { setUploadFile(event.target.files?.[0] ?? null); setUploadError(event.target.files?.[0] ? importFilenameError(event.target.files[0].name) : ""); setNotice(""); }} />
                    <p className="text-xs text-muted-foreground">文件将上传到下方目标存储池，完成后会自动选为来源卷。上传本身也是可取消任务。</p>
                  </div>
                  {uploadFile ? <p className="break-all text-xs text-muted-foreground">{uploadFile.name} · {formatBytes(uploadFile.size)}</p> : null}
                  {uploadError ? <p className="text-xs text-destructive" role="alert">{uploadError}</p> : null}
                  {notice ? <p className="text-xs text-success" role="status">{notice}</p> : null}
                  <Button type="button" size="sm" variant="outline" data-testid="vm-import-upload" disabled={!canImport || !uploadFile || Boolean(importFilenameError(uploadFile.name)) || uploadMutation.isPending || !form.targetPool} onClick={() => { setUploadError(""); setNotice(""); uploadMutation.mutate(); }}>
                    {uploadMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}
                    {uploadMutation.isPending ? "正在上传" : "上传文件"}
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2"><Server className="h-4 w-4 text-primary" />虚拟机配置</CardTitle>
              <CardDescription>CPU、内存、固件、网络和 Guest Agent 均按本次配置重新定义；ISO 模式会额外创建空白系统盘。</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4 sm:grid-cols-2">
              <div className="min-w-0 space-y-2 sm:col-span-2">
                <Label htmlFor="vm-import-name">名称</Label>
                <Input id="vm-import-name" value={form.name} onChange={(event) => update("name", event.target.value)} placeholder="例如 imported-web-01" required disabled={!canImport} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="vm-import-vcpus">vCPU</Label>
                <Input id="vm-import-vcpus" type="number" min={1} max={255} value={form.vcpus} onChange={(event) => update("vcpus", event.target.value)} required disabled={!canImport} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="vm-import-memory">内存 (MiB)</Label>
                <Input id="vm-import-memory" type="number" min={128} value={form.memoryMiB} onChange={(event) => update("memoryMiB", event.target.value)} required disabled={!canImport} />
              </div>
              {selectedISO ? (
                <div className="min-w-0 space-y-2 sm:col-span-2" data-testid="vm-import-iso-disk-capacity">
                  <Label htmlFor="vm-import-disk-capacity">系统盘容量 (GiB)</Label>
                  <Input id="vm-import-disk-capacity" type="number" min={1} max={16384} value={form.diskCapacityGiB} onChange={(event) => update("diskCapacityGiB", event.target.value)} required disabled={!canImport} />
                  <p className="text-xs text-muted-foreground">ISO 只作为只读光驱挂载；系统盘将创建为空白 QCOW2，并从光盘优先启动。</p>
                </div>
              ) : null}
              <div className="space-y-2">
                <Label htmlFor="vm-import-disk-bus">磁盘总线</Label>
                <select id="vm-import-disk-bus" value={form.diskBus} onChange={(event) => update("diskBus", event.target.value as VMDiskBus)} disabled={!canImport} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                  <option value="virtio">virtio（默认）</option>
                  <option value="sata">SATA</option>
                  <option value="scsi">SCSI</option>
                </select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="vm-import-nic-model">网卡型号</Label>
                <select id="vm-import-nic-model" value={form.nicModel} onChange={(event) => update("nicModel", event.target.value as VMNICModel)} disabled={!canImport} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                  <option value="virtio">virtio（默认）</option>
                  <option value="e1000">e1000</option>
                </select>
              </div>
              <div className="min-w-0 space-y-2">
                <Label htmlFor="vm-import-network">虚拟网络</Label>
                <select id="vm-import-network" value={form.network} onChange={(event) => update("network", event.target.value)} required disabled={!canImport || !activeNetworks.length} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                  {!activeNetworks.length ? <option value="">没有活跃虚拟网络</option> : null}
                  {activeNetworks.map((network) => <option key={network.uuid || network.name} value={network.name}>{network.name}{network.bridge ? ` · ${network.bridge}` : ""}</option>)}
                </select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="vm-import-firmware">固件</Label>
                <select id="vm-import-firmware" value={form.firmware} onChange={(event) => { const firmware = event.target.value as VMFirmware; setForm((current) => ({ ...current, firmware, secureBoot: firmware === "uefi" ? current.secureBoot : false })); }} disabled={!canImport} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                  <option value="bios">BIOS</option>
                  <option value="uefi">UEFI</option>
                </select>
              </div>
              <div className="space-y-3 border-t pt-4 sm:col-span-2">
                <label className="flex items-start gap-3 text-sm">
                  <input aria-label="启用 Secure Boot" type="checkbox" className="mt-0.5 h-4 w-4" checked={form.secureBoot} onChange={(event) => update("secureBoot", event.target.checked)} disabled={!canImport || form.firmware !== "uefi"} />
                  <span><span className="block font-medium">Secure Boot</span><span className="mt-0.5 block text-xs text-muted-foreground">仅 UEFI 固件支持。</span></span>
                </label>
                <label className="flex items-start gap-3 text-sm">
                  <input aria-label="启用 Guest Agent" type="checkbox" className="mt-0.5 h-4 w-4" checked={form.guestAgent} onChange={(event) => update("guestAgent", event.target.checked)} disabled={!canImport} />
                  <span><span className="block font-medium">Guest Agent 通道</span><span className="mt-0.5 block text-xs text-muted-foreground">要求客户机内已安装并启用 qemu-guest-agent。</span></span>
                </label>
                <label className="flex items-start gap-3 text-sm">
                  <input aria-label="开机自动启动" type="checkbox" className="mt-0.5 h-4 w-4" checked={form.autostart} onChange={(event) => update("autostart", event.target.checked)} disabled={!canImport} />
                  <span className="font-medium">开机自动启动</span>
                </label>
                <label className="flex items-start gap-3 text-sm">
                  <input aria-label="导入后立即启动" type="checkbox" className="mt-0.5 h-4 w-4" checked={form.start} onChange={(event) => update("start", event.target.checked)} disabled={!canImport} />
                  <span className="font-medium">导入后立即启动</span>
                </label>
              </div>
            </CardContent>
          </Card>
        </div>

        <div className="min-w-0 space-y-4">
          <Card className="xl:sticky xl:top-4">
            <CardHeader>
              <CardTitle>导入确认</CardTitle>
              <CardDescription>{selectedISO ? "ISO 将作为只读安装介质挂载，并新建空白系统盘；来源 ISO 保持原样。" : "导入会在目标池复制一份新的系统盘，来源卷保持原样。"}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="vm-import-target-pool">目标存储池</Label>
                <select id="vm-import-target-pool" value={form.targetPool} onChange={(event) => update("targetPool", event.target.value)} disabled={!canImport || !activePools.length} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                  {!activePools.length ? <option value="">没有活跃存储池</option> : null}
                  {activePools.map((pool) => <option key={pool.name} value={pool.name}>{pool.name}</option>)}
                </select>
              </div>
              <dl className="space-y-3 text-sm">
                <div className="flex items-start justify-between gap-3"><dt className="text-muted-foreground">来源卷</dt><dd className="min-w-0 break-all text-right font-mono text-xs">{form.sourceVolume || "-"}</dd></div>
                <div className="flex items-start justify-between gap-3"><dt className="text-muted-foreground">格式</dt><dd>{selectedFormat ? <Badge variant="outline">{selectedFormat}</Badge> : "-"}</dd></div>
                <div className="flex items-start justify-between gap-3"><dt className="text-muted-foreground">容量</dt><dd>{selectedVolume ? formatBytes(selectedVolume.capacityBytes) : "-"}</dd></div>
                {selectedISO ? <div className="flex items-start justify-between gap-3"><dt className="text-muted-foreground">系统盘</dt><dd>{form.diskCapacityGiB || "0"} GiB · QCOW2</dd></div> : null}
                <div className="flex items-start justify-between gap-3"><dt className="text-muted-foreground">目标磁盘</dt><dd className="min-w-0 break-all text-right font-mono text-xs">{selectedFormat ? `${form.targetPool}/${vmImportTargetDiskName(form.name, selectedFormat)}` : "-"}</dd></div>
              </dl>
              <div className="rounded-md border border-primary/25 bg-primary/5 px-3 py-2 text-xs text-muted-foreground" data-testid="vm-import-copy-notice">
                {selectedISO ? "来源 ISO 不会被修改或删除。导入会新建空白系统盘；任务失败或取消时会清理该系统盘。" : "来源卷不会被修改或删除。任务失败或取消时会尽量清理新复制的目标卷；请勿在导入期间删除来源卷。"}
              </div>
              {error ? <p className="text-sm text-destructive" role="alert" data-testid="vm-import-error">{error}</p> : null}
              <Button type="submit" className="w-full" data-testid="vm-import-submit" disabled={!canImport || importMutation.isPending || !selectedVolume || !form.name.trim()}>
                {importMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}
                {importMutation.isPending ? "正在提交导入" : "开始导入"}
              </Button>
            </CardContent>
          </Card>

          {task ? (
            <Card className="border-success/35" data-testid="vm-import-result">
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-success"><CheckCircle2 className="h-4 w-4" />导入任务已创建</CardTitle>
                <CardDescription>{selectedISO ? "任务已进入后台执行，空白系统盘创建完成后会定义虚拟机并挂载 ISO。" : "任务已进入后台执行，磁盘复制完成后会定义并可选启动虚拟机。"}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-3 text-sm">
                <p className="break-all font-mono text-xs">{task.id}</p>
                <p className="text-muted-foreground">{task.step}</p>
                <div className="flex flex-col gap-2 sm:flex-row">
                  <Button asChild size="sm"><Link to="/tasks">查看任务中心</Link></Button>
                  <Button asChild size="sm" variant="outline"><Link to="/vms">返回虚拟机列表</Link></Button>
                </div>
              </CardContent>
            </Card>
          ) : null}
        </div>
      </form>
    </div>
  );
}

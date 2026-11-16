import { useEffect, useRef, useState, type FormEvent } from "react";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, ArrowLeft, Image as ImageIcon, LayoutTemplate, LoaderCircle, Plus, Server } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { VMBootOrder, VMCreateInput, VMDiskBus, VMFirmware, VMImageCloneMode, VMNICModel, VMTemplate } from "@/api/types";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useSessionQuery } from "@/features/auth/use-session";
import { CloudInitEditor } from "@/features/cloud-init/cloud-init-editor";
import { cloudInitBundleValidationError, defaultCloudInitUserData } from "@/features/cloud-init/cloud-init-model";
import { useImagesQuery } from "@/features/images/use-images-query";
import { useNetworksQuery } from "@/features/networks/use-networks-query";
import { overviewQueryKey } from "@/features/overview/use-overview-query";
import { useStoragePoolsQuery, useStorageVolumesQuery } from "@/features/storage/use-storage-query";
import { templateVMCreateDefaults } from "@/features/templates/template-form";
import { useTemplatesQuery } from "@/features/templates/use-templates-query";
import { vmQueryKey } from "@/features/vms/use-vm-query";
import { formatBytes } from "@/lib/utils";

type FormState = {
  name: string;
  vcpus: string;
  memoryMiB: string;
  diskPath: string;
  diskFormat: string;
  diskBus: VMDiskBus;
  network: string;
  nicModel: VMNICModel;
  isoPath: string;
  bootOrder: VMBootOrder;
  firmware: VMFirmware;
  secureBoot: boolean;
  cloudInit: boolean;
  cloudInitUserData: string;
  cloudInitNetworkConfig: string;
  cloudInitVendorData: string;
  guestAgent: boolean;
  autostart: boolean;
  start: boolean;
};

const initialForm: FormState = {
  name: "",
  vcpus: "2",
  memoryMiB: "2048",
  diskPath: "",
  diskFormat: "qcow2",
  diskBus: "virtio",
  network: "",
  nicModel: "virtio",
  isoPath: "",
  bootOrder: "auto",
  firmware: "bios",
  secureBoot: false,
  cloudInit: false,
  cloudInitUserData: "",
  cloudInitNetworkConfig: "",
  cloudInitVendorData: "",
  guestAgent: true,
  autostart: false,
  start: false,
};

export function VMCreatePage() {
  const session = useSessionQuery();
  const networksQuery = useNetworksQuery();
  const poolsQuery = useStoragePoolsQuery();
  const templatesQuery = useTemplatesQuery();
  const imagesQuery = useImagesQuery();
  const search = useSearch({ strict: false }) as { template?: number };
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<FormState>(initialForm);
  const [diskPool, setDiskPool] = useState("");
  const [isoPool, setIsoPool] = useState("");
  const [sourceMode, setSourceMode] = useState<"disk" | "image">("disk");
  const [selectedImageID, setSelectedImageID] = useState("");
  const [imageCloneMode, setImageCloneMode] = useState<VMImageCloneMode>("full");
  const [error, setError] = useState("");
  const [selectedTemplateID, setSelectedTemplateID] = useState("");
  const [guestAgentDistro, setGuestAgentDistro] = useState<"debian" | "rhel" | "suse">("debian");
  const presetApplied = useRef(false);
  const activeNetworks = (networksQuery.data?.items ?? []).filter((network) => network.active);
  const activePools = (poolsQuery.data?.items ?? []).filter((pool) => pool.state === "active");
  const volumesQuery = useStorageVolumesQuery(diskPool);
  const isoVolumesQuery = useStorageVolumesQuery(isoPool);
  const diskVolumes = (volumesQuery.data?.items ?? []).filter((volume) => {
    const format = (volume.format || "").toLowerCase();
    return (format === "qcow2" || format === "raw") && /\.(qcow2|raw)$/i.test(volume.name);
  });
  const isoVolumes = (isoVolumesQuery.data?.items ?? []).filter((volume) => {
    const format = (volume.format || "").toLowerCase();
    return /\.iso$/i.test(volume.name) && (format === "raw" || format === "iso" || format === "");
  });
  const selectedISOVolume = isoVolumes.find((volume) => volume.path === form.isoPath);
  const selectedVolume = diskVolumes.find((volume) => volume.path === form.diskPath);
  const templates = templatesQuery.data?.items ?? [];
  const images = imagesQuery.data?.items ?? [];
  const selectedImage = images.find((image) => String(image.id) === selectedImageID);
  const linkedCloneAvailable = selectedImage?.format === "qcow2" && diskPool === selectedImage.pool;
  const selectedTemplate = templates.find((template) => String(template.id) === selectedTemplateID);

  useEffect(() => {
    if (!search.template || presetApplied.current) return;
    const template = templates.find((candidate) => candidate.id === search.template);
    if (!template) return;
    presetApplied.current = true;
    setSelectedTemplateID(String(template.id));
    applyTemplate(template);
  }, [search.template, templates]);

  useEffect(() => {
    if (!activeNetworks.length) return;
    if (activeNetworks.some((network) => network.name === form.network)) return;
    const preferred = activeNetworks.find((network) => network.name === "default") ?? activeNetworks[0];
    setForm((current) => ({ ...current, network: preferred.name }));
  }, [activeNetworks, form.network]);

  useEffect(() => {
    if (!activePools.length) return;
    if (activePools.some((pool) => pool.name === diskPool)) return;
    const preferred = activePools.find((pool) => pool.name === "default") ?? activePools[0];
    setDiskPool(preferred.name);
  }, [activePools, diskPool]);

  useEffect(() => {
    if (!activePools.length) return;
    if (activePools.some((pool) => pool.name === isoPool)) return;
    const preferred = activePools.find((pool) => pool.name === "iso-store")
      ?? activePools.find((pool) => pool.name === "default")
      ?? activePools[0];
    setIsoPool(preferred.name);
  }, [activePools, isoPool]);

  useEffect(() => {
    if (imageCloneMode === "linked" && !linkedCloneAvailable) setImageCloneMode("full");
  }, [imageCloneMode, linkedCloneAvailable]);

  const mutation = useMutation({
    mutationFn: (input: VMCreateInput) => api.createVM(input),
    onSuccess: (vm) => {
      queryClient.setQueryData(vmQueryKey(vm.uuid), vm);
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
      void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
      void navigate({ to: "/vms/$uuid", params: { uuid: vm.uuid } });
    },
    onError: (cause) => {
      setError(cause instanceof ApiError ? cause.message : "创建虚拟机失败，请稍后重试。");
    },
  });

  const readOnly = Boolean(session.data?.readOnly);
  const viewer = session.data?.user.role === "viewer";
  const canCreate = !readOnly && !viewer;
  const disabledReason = readOnly ? "系统当前处于只读模式" : viewer ? "当前账号没有创建虚拟机权限" : "";

  function update<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  function applyTemplate(template: VMTemplate) {
    const defaults = templateVMCreateDefaults(template);
    if (defaults.imageId > 0) {
      setSourceMode("image");
      setSelectedImageID(String(defaults.imageId));
    } else {
      setSourceMode("disk");
      setSelectedImageID("");
    }
    setForm((current) => ({
      ...current,
      vcpus: String(defaults.vcpus),
      memoryMiB: String(defaults.memoryMiB),
      diskFormat: defaults.diskFormat,
      network: defaults.network,
      isoPath: defaults.isoPath ?? "",
      firmware: defaults.firmware,
      secureBoot: defaults.secureBoot,
      cloudInit: defaults.cloudInit,
      cloudInitUserData: defaults.cloudInitUserData,
      cloudInitNetworkConfig: defaults.cloudInitNetworkConfig,
      cloudInitVendorData: defaults.cloudInitVendorData,
      guestAgent: defaults.guestAgent,
      autostart: defaults.autostart,
      start: defaults.start,
    }));
  }

  function selectTemplate(id: string) {
    setSelectedTemplateID(id);
    const template = templates.find((candidate) => String(candidate.id) === id);
    if (template) applyTemplate(template);
  }

  function selectDiskVolume(name: string) {
    const volume = diskVolumes.find((item) => item.name === name);
    if (!volume) return;
    setForm((current) => ({
      ...current,
      diskPath: volume.path || "",
      diskFormat: volume.format === "raw" ? "raw" : "qcow2",
    }));
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canCreate) return;
    setError("");
    if (sourceMode === "image" && (!selectedImageID || !diskPool)) {
      setError("请选择基础镜像和目标存储池。");
      return;
    }
    const cloudInitError = form.cloudInit ? cloudInitBundleValidationError(form.cloudInitUserData, form.cloudInitNetworkConfig, form.cloudInitVendorData) : "";
    if (cloudInitError) {
      setError(cloudInitError);
      return;
    }
    mutation.mutate({
      name: form.name.trim(),
      vcpus: Number(form.vcpus),
      memoryMiB: Number(form.memoryMiB),
      ...(sourceMode === "image" ? { diskPath: "", diskFormat: "", imageId: Number(selectedImageID), imageCloneMode, targetPool: diskPool } : { diskPath: form.diskPath.trim(), diskFormat: form.diskFormat }),
      diskBus: form.diskBus,
      network: form.network.trim(),
      nicModel: form.nicModel,
      isoPath: form.isoPath.trim(),
      bootOrder: form.bootOrder,
      firmware: form.firmware,
      secureBoot: form.secureBoot,
      cloudInit: form.cloudInit ? {
        userData: form.cloudInitUserData.trim(),
        networkConfig: form.cloudInitNetworkConfig.trim() || undefined,
        vendorData: form.cloudInitVendorData.trim() || undefined,
        pool: diskPool,
      } : undefined,
      guestAgent: form.guestAgent,
      autostart: form.autostart,
      start: form.start,
    });
  }

  return (
    <>
      <PageHeader
        title="新建虚拟机"
        breadcrumb={<span className="inline-flex items-center gap-1.5"><Link to="/vms" className="inline-flex items-center gap-1 hover:text-foreground"><ArrowLeft className="h-3 w-3" />虚拟机</Link><span>/</span><span>新建</span></span>}
        description="定义一个持久化 libvirt 域。系统盘可直接使用已有磁盘，也可从镜像库全量克隆。"
      />

      {!canCreate ? (
        <div className="mb-4 flex items-start gap-3 rounded-lg border border-warning/35 bg-warning/10 px-4 py-3 text-sm text-warning" role="status">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <p>{disabledReason}，创建操作已禁用。</p>
        </div>
      ) : null}

      <form onSubmit={handleSubmit}>
        <Card className="mb-4" data-testid="vm-template-picker">
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-base"><LayoutTemplate className="h-4 w-4 text-primary" />从模板预填</CardTitle>
            <CardDescription>模板提供计算规格、固件、网络和 Cloud-init 默认值；名称与磁盘路径仍由你填写。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            <Label htmlFor="vm-template">虚拟机模板</Label>
            <select
              id="vm-template"
              aria-label="虚拟机模板"
              value={selectedTemplateID}
              onChange={(event) => selectTemplate(event.target.value)}
              disabled={templatesQuery.isLoading}
              className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
            >
              <option value="">{templatesQuery.isLoading ? "正在读取模板..." : "不使用模板"}</option>
              {templates.map((template) => <option key={template.id} value={template.id}>{template.name} · {template.vcpus} vCPU · {template.memoryMiB} MiB</option>)}
            </select>
            <p className="text-xs text-muted-foreground">{selectedTemplate ? selectedTemplate.description || "已应用模板默认值，可继续按需调整。" : templates.length ? `${templates.length} 个模板可用于预填。` : "暂无可用模板，可在“虚拟机模板”页面创建。"}</p>
          </CardContent>
        </Card>
        <div className="grid gap-4 xl:grid-cols-[1.15fr_0.85fr]">
          <Card>
            <CardHeader>
              <CardTitle>基础配置</CardTitle>
              <CardDescription>名称在 libvirt 中必须唯一；内存以 MiB 为单位。</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="vm-name">名称</Label>
                <Input id="vm-name" value={form.name} onChange={(event) => update("name", event.target.value)} placeholder="例如 web-01" required disabled={!canCreate} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="vm-vcpus">vCPU</Label>
                <Input id="vm-vcpus" type="number" min={1} max={255} value={form.vcpus} onChange={(event) => update("vcpus", event.target.value)} required disabled={!canCreate} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="vm-memory">内存 (MiB)</Label>
                <Input id="vm-memory" type="number" min={128} value={form.memoryMiB} onChange={(event) => update("memoryMiB", event.target.value)} required disabled={!canCreate} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="vm-firmware">固件</Label>
                <select
                  id="vm-firmware"
                  value={form.firmware}
                  onChange={(event) => {
                    const firmware = event.target.value as VMFirmware;
                    setForm((current) => ({ ...current, firmware, secureBoot: firmware === "uefi" ? current.secureBoot : false }));
                  }}
                  disabled={!canCreate}
                  className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
                >
                  <option value="bios">BIOS</option>
                  <option value="uefi">UEFI</option>
                </select>
              </div>
              <div className="space-y-2">
                <p className="text-sm font-medium leading-none">安全启动</p>
                <label className="flex h-9 items-center gap-2 rounded-md border border-input bg-card px-3 text-sm shadow-sm">
                  <input
                    aria-label="安全启动 (Secure Boot)"
                    type="checkbox"
                    className="h-4 w-4"
                    checked={form.secureBoot}
                    onChange={(event) => update("secureBoot", event.target.checked)}
                    disabled={!canCreate || form.firmware !== "uefi"}
                  />
                  <span className={form.firmware === "uefi" ? "" : "text-muted-foreground"}>Secure Boot</span>
                </label>
                <p className="text-xs text-muted-foreground">仅 UEFI 固件支持，使用 libvirt 安全固件自动选择。</p>
              </div>
              <div className="space-y-2 sm:col-span-2">
                <Label>系统盘来源</Label>
                <div className="grid gap-2 sm:grid-cols-2" role="radiogroup" aria-label="系统盘来源">
                  <label className={`flex cursor-pointer items-start gap-3 rounded-lg border p-3 text-sm ${sourceMode === "disk" ? "border-primary bg-primary/5" : ""}`}>
                    <input type="radio" name="vm-disk-source" value="disk" data-testid="vm-disk-source-direct" className="mt-0.5 h-4 w-4" checked={sourceMode === "disk"} onChange={() => { setSourceMode("disk"); setSelectedImageID(""); }} disabled={!canCreate} />
                    <span><span className="block font-medium">使用已有磁盘</span><span className="mt-0.5 block text-xs text-muted-foreground">填写宿主机上已存在的绝对路径，平台不创建磁盘。</span></span>
                  </label>
                  <label className={`flex cursor-pointer items-start gap-3 rounded-lg border p-3 text-sm ${sourceMode === "image" ? "border-primary bg-primary/5" : ""}`}>
                    <input type="radio" name="vm-disk-source" value="image" data-testid="vm-disk-source-image" className="mt-0.5 h-4 w-4" checked={sourceMode === "image"} onChange={() => { setSourceMode("image"); if (!selectedImageID && images[0]) setSelectedImageID(String(images[0].id)); }} disabled={!canCreate || !images.length} />
                    <span><span className="flex items-center gap-1 font-medium"><ImageIcon className="h-3.5 w-3.5" />从镜像克隆</span><span className="mt-0.5 block text-xs text-muted-foreground">从镜像库全量复制系统盘到目标存储池。</span></span>
                  </label>
                </div>
              </div>
              {sourceMode === "disk" ? <>
              <div className="space-y-2 sm:col-span-2">
                <Label>从存储选择磁盘（可选）</Label>
                <div className="grid gap-3 sm:grid-cols-2">
                  <select
                    aria-label="存储池"
                    value={diskPool}
                    onChange={(event) => setDiskPool(event.target.value)}
                    disabled={!canCreate || !activePools.length}
                    className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
                  >
                    {!activePools.length ? <option value="">{poolsQuery.isLoading ? "正在读取存储池..." : "没有活跃存储池"}</option> : null}
                    {activePools.map((pool) => <option key={pool.uuid || pool.name} value={pool.name}>{pool.name} · {pool.targetPath || "无目标路径"}</option>)}
                  </select>
                  <select
                    aria-label="存储卷"
                    value={selectedVolume?.name ?? ""}
                    onChange={(event) => selectDiskVolume(event.target.value)}
                    disabled={!canCreate || !diskVolumes.length}
                    className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
                  >
                    <option value="">{volumesQuery.isLoading ? "正在读取存储卷..." : "手动输入路径"}</option>
                    {diskVolumes.map((volume) => <option key={volume.path || volume.name} value={volume.name}>{volume.name} · {formatBytes(volume.capacityBytes)}</option>)}
                  </select>
                </div>
                <p className="text-xs text-muted-foreground">选择已有卷后会自动填充磁盘路径和格式；仍可在下方手工修改。</p>
              </div>
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="vm-disk-path">磁盘路径</Label>
                <Input id="vm-disk-path" value={form.diskPath} onChange={(event) => update("diskPath", event.target.value)} placeholder="/var/lib/libvirt/images/web-01.qcow2" required disabled={!canCreate} />
                <p className="text-xs text-muted-foreground">必须为宿主机绝对路径。平台不会自动创建或删除磁盘文件。</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="vm-disk-format">磁盘格式</Label>
                <select id="vm-disk-format" value={form.diskFormat} onChange={(event) => update("diskFormat", event.target.value)} disabled={!canCreate} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                  <option value="qcow2">qcow2</option>
                  <option value="raw">raw</option>
                </select>
              </div>
              </> : <>
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="vm-image">基础镜像</Label>
                <select id="vm-image" aria-label="基础镜像" data-testid="vm-image" value={selectedImageID} onChange={(event) => setSelectedImageID(event.target.value)} required disabled={!canCreate || imagesQuery.isLoading} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                  {!images.length ? <option value="">{imagesQuery.isLoading ? "正在读取镜像..." : "镜像库为空"}</option> : null}
                  {images.map((image) => <option key={image.id} value={image.id}>{image.name} · {image.format} · {formatBytes(image.capacityBytes)}</option>)}
                </select>
                {selectedImage ? <p className="text-xs text-muted-foreground">{selectedImage.osName || selectedImage.description || "未填写操作系统信息"} · 来源池 {selectedImage.pool}</p> : <p className="text-xs text-warning">镜像库为空，请先前往“镜像库”登记基础磁盘。</p>}
              </div>
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="vm-image-target-pool">目标存储池</Label>
                <select id="vm-image-target-pool" aria-label="目标存储池" value={diskPool} onChange={(event) => setDiskPool(event.target.value)} disabled={!canCreate || !activePools.length} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                  {!activePools.length ? <option value="">没有活跃存储池</option> : null}
                  {activePools.map((pool) => <option key={pool.uuid || pool.name} value={pool.name}>{pool.name} · {pool.targetPath || "无目标路径"}</option>)}
                </select>
                <p className="text-xs text-muted-foreground">克隆卷命名为 <span className="font-mono">{form.name || "<name>"}-disk.{selectedImage?.format ?? "qcow2"}</span>；创建域失败时会清理已克隆磁盘。</p>
              </div>
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="vm-image-clone-mode">克隆方式</Label>
                <select
                  id="vm-image-clone-mode"
                  data-testid="vm-image-clone-mode"
                  value={imageCloneMode}
                  onChange={(event) => {
                    const mode = event.target.value as VMImageCloneMode;
                    setImageCloneMode(mode);
                    if (mode === "linked" && selectedImage) setDiskPool(selectedImage.pool);
                  }}
                  disabled={!canCreate}
                  className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
                >
                  <option value="full">全量克隆（独立磁盘）</option>
                  <option value="linked" disabled={!linkedCloneAvailable}>链接克隆（节省空间，仅同池 qcow2）</option>
                </select>
                <p className="text-xs text-muted-foreground">
                  {linkedCloneAvailable
                    ? "链接克隆创建轻量 qcow2 覆盖层，虚拟机写入不会修改基础镜像。基础卷在其子卷存在时禁止删除。"
                    : "链接克隆要求基础镜像为 qcow2，且目标存储池与镜像来源池一致；当前将使用全量克隆。"}
                </p>
              </div>
              </>}
              <div className="space-y-2">
                <Label htmlFor="vm-disk-bus">磁盘总线</Label>
                <select id="vm-disk-bus" value={form.diskBus} onChange={(event) => update("diskBus", event.target.value as VMDiskBus)} disabled={!canCreate} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                  <option value="virtio">virtio（默认）</option>
                  <option value="sata">SATA</option>
                  <option value="scsi">SCSI</option>
                </select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="vm-network">虚拟网络</Label>
                <select id="vm-network" value={form.network} onChange={(event) => update("network", event.target.value)} required disabled={!canCreate || !activeNetworks.length} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                  {!activeNetworks.length ? <option value="">{networksQuery.isLoading ? "正在读取网络..." : "没有活跃虚拟网络"}</option> : null}
                  {activeNetworks.map((network) => <option key={network.uuid || network.name} value={network.name}>{network.name}{network.bridge ? ` · ${network.bridge}` : ""}</option>)}
                </select>
                <p className="text-xs text-muted-foreground">{networksQuery.isError ? "网络列表加载失败，请刷新页面后重试。" : `${activeNetworks.length} 个活跃虚拟网络可选。`}</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="vm-nic-model">网卡型号</Label>
                <select id="vm-nic-model" value={form.nicModel} onChange={(event) => update("nicModel", event.target.value as VMNICModel)} disabled={!canCreate} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                  <option value="virtio">virtio（默认）</option>
                  <option value="e1000">e1000</option>
                </select>
              </div>
              <div className="space-y-2 sm:col-span-2">
                <Label>从存储池选择 ISO（可选）</Label>
                <div className="grid gap-3 sm:grid-cols-2">
                  <select
                    aria-label="ISO 存储池"
                    data-testid="vm-iso-pool"
                    value={isoPool}
                    onChange={(event) => setIsoPool(event.target.value)}
                    disabled={!canCreate || !activePools.length}
                    className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
                  >
                    {!activePools.length ? <option value="">{poolsQuery.isLoading ? "正在读取存储池..." : "没有活跃存储池"}</option> : null}
                    {activePools.map((pool) => <option key={pool.uuid || pool.name} value={pool.name}>{pool.name} · {pool.targetPath || "无目标路径"}</option>)}
                  </select>
                  <select
                    aria-label="ISO 安装介质"
                    data-testid="vm-iso-volume"
                    value={selectedISOVolume?.name ?? ""}
                    onChange={(event) => {
                      const volume = isoVolumes.find((item) => item.name === event.target.value);
                      update("isoPath", volume?.path ?? "");
                    }}
                    disabled={!canCreate || !isoVolumes.length}
                    className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
                  >
                    <option value="">{isoVolumesQuery.isLoading ? "正在读取 ISO..." : "不挂载安装介质"}</option>
                    {isoVolumes.map((volume) => <option key={volume.path || volume.name} value={volume.name}>{volume.name} · {formatBytes(volume.capacityBytes)}</option>)}
                  </select>
                </div>
                <p className="text-xs text-muted-foreground">{isoVolumesQuery.isError ? "ISO 列表加载失败，可继续使用下方绝对路径。" : `${isoVolumes.length} 个 ISO 可选；也可以手动填写绝对路径。`}</p>
              </div>
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="vm-iso-path">ISO 绝对路径（可选）</Label>
                <Input id="vm-iso-path" value={form.isoPath} onChange={(event) => update("isoPath", event.target.value)} placeholder="/var/lib/libvirt/images/install.iso" disabled={!canCreate} />
                <p className="text-xs text-muted-foreground">选择存储卷会自动写入绝对路径；填写后挂载为只读 CD-ROM，默认光盘优先，可按下方启动顺序调整。</p>
              </div>
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="vm-boot-order">启动顺序</Label>
                <select id="vm-boot-order" value={form.bootOrder} onChange={(event) => update("bootOrder", event.target.value as VMBootOrder)} disabled={!canCreate} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                  <option value="auto">自动（有 ISO 时光盘优先）</option>
                  <option value="disk-first">硬盘优先</option>
                  <option value="cdrom-first" disabled={!form.isoPath.trim()}>光盘优先（需填写 ISO）</option>
                </select>
              </div>
              <div className="space-y-3 border-t pt-4 sm:col-span-2">
                <label className="flex items-start gap-3 text-sm">
                  <input
                    aria-label="启用 Cloud-init"
                    type="checkbox"
                    className="mt-0.5 h-4 w-4"
                    checked={form.cloudInit}
                    onChange={(event) => setForm((current) => ({
                      ...current,
                      cloudInit: event.target.checked,
                      cloudInitUserData: event.target.checked && !current.cloudInitUserData.trim()
                        ? defaultCloudInitUserData()
                        : current.cloudInitUserData,
                    }))}
                    disabled={!canCreate}
                  />
                  <span>
                    <span className="block font-medium">启用 Cloud-init</span>
                    <span className="mt-0.5 block text-xs text-muted-foreground">生成 NoCloud seed ISO，并作为只读 CD-ROM 挂载到虚拟机。</span>
                  </span>
                </label>
                {form.cloudInit ? (
                  <CloudInitEditor
                    prefix="vm-cloud-init"
                    value={form.cloudInitUserData}
                    onChange={(value) => update("cloudInitUserData", value)}
                    networkConfig={form.cloudInitNetworkConfig}
                    onNetworkConfigChange={(value) => update("cloudInitNetworkConfig", value)}
                    vendorData={form.cloudInitVendorData}
                    onVendorDataChange={(value) => update("cloudInitVendorData", value)}
                    disabled={!canCreate}
                  />
                ) : null}
                {form.cloudInit ? <p className="text-xs text-muted-foreground">seed ISO 将写入存储池 <span className="font-medium text-foreground">{diskPool || "-"}</span>；审计只记录启用状态和字节数，不保存用户数据内容。</p> : null}
                <label className="flex items-start gap-3 rounded-lg border p-3 text-sm">
                  <input
                    aria-label="启用 Guest Agent 通道"
                    type="checkbox"
                    className="mt-0.5 h-4 w-4"
                    checked={form.guestAgent}
                    onChange={(event) => update("guestAgent", event.target.checked)}
                    disabled={!canCreate}
                  />
                  <span>
                    <span className="block font-medium">启用 Guest Agent 通道</span>
                    <span className="mt-0.5 block text-xs text-muted-foreground">挂载 org.qemu.guest_agent.0；仍需在客户机内安装并启动 qemu-guest-agent。</span>
                  </span>
                </label>
                {form.guestAgent ? (
                  <details className="rounded-lg border bg-muted/20 p-3 text-xs">
                    <summary className="cursor-pointer font-medium">Cloud-init 安装引导（不会自动改写现有 user-data）</summary>
                    <div className="mt-3 space-y-2">
                      <select aria-label="Guest Agent 客户机发行版" value={guestAgentDistro} onChange={(event) => setGuestAgentDistro(event.target.value as typeof guestAgentDistro)} className="h-8 rounded-md border border-input bg-card px-2 text-xs">
                        <option value="debian">Debian / Ubuntu</option>
                        <option value="rhel">RHEL / Fedora / Rocky</option>
                        <option value="suse">openSUSE / SLES</option>
                      </select>
                      <pre className="overflow-auto rounded-md bg-background p-2 font-mono text-[11px]">{`packages:\n  - qemu-guest-agent\nruncmd:\n  - systemctl enable --now qemu-guest-agent`}</pre>
                      <p className="text-muted-foreground">请将片段合并到现有 Cloud-init user-data。Windows 客户机请从 virtio-win ISO 安装 guest-agent MSI；当前不支持自动写入 Windows 安装介质。</p>
                    </div>
                  </details>
                ) : null}
              </div>
            </CardContent>
          </Card>

          <div className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle>启动策略</CardTitle>
                <CardDescription>默认只定义域，不立即启动，便于先检查配置。</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <label className="flex items-start gap-3 rounded-lg border p-3 text-sm">
                  <input type="checkbox" className="mt-0.5 h-4 w-4" checked={form.autostart} onChange={(event) => update("autostart", event.target.checked)} disabled={!canCreate} />
                  <span><span className="block font-medium">开机自动启动</span><span className="mt-0.5 block text-xs text-muted-foreground">宿主机启动时由 libvirt 自动拉起。</span></span>
                </label>
                <label className="flex items-start gap-3 rounded-lg border p-3 text-sm">
                  <input type="checkbox" className="mt-0.5 h-4 w-4" checked={form.start} onChange={(event) => update("start", event.target.checked)} disabled={!canCreate} />
                  <span><span className="block font-medium">创建后立即启动</span><span className="mt-0.5 block text-xs text-muted-foreground">磁盘或网络不可用时会返回错误并回滚域定义。</span></span>
                </label>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>创建摘要</CardTitle>
                <CardDescription>提交前快速核对资源规格。</CardDescription>
              </CardHeader>
              <CardContent className="space-y-3 text-sm">
                <div className="flex items-center justify-between"><span className="text-muted-foreground">名称</span><span className="max-w-48 truncate font-medium">{form.name || "-"}</span></div>
                <div className="flex items-center justify-between"><span className="text-muted-foreground">计算资源</span><span className="font-medium">{form.vcpus || "0"} vCPU · {form.memoryMiB || "0"} MiB</span></div>
                <div className="flex items-center justify-between"><span className="text-muted-foreground">系统盘</span><span className="max-w-52 truncate font-medium">{sourceMode === "image" ? (selectedImage?.name || "未选择镜像") : (form.diskPath || "未选择磁盘")}</span></div>
                <div className="flex items-center justify-between"><span className="text-muted-foreground">固件</span><span className="font-medium">{form.firmware === "uefi" ? "UEFI" : "BIOS"}{form.secureBoot ? " · Secure Boot" : ""}</span></div>
                <div className="flex items-center justify-between"><span className="text-muted-foreground">Cloud-init</span><span className="font-medium">{form.cloudInit ? "已启用" : "未启用"}</span></div>
                <div className="flex items-center justify-between"><span className="text-muted-foreground">Guest Agent</span><span className="font-medium">{form.guestAgent ? "通道已启用" : "未启用"}</span></div>
                <div className="flex items-center justify-between"><span className="text-muted-foreground">网络</span><Badge variant="outline">{form.network || "-"}</Badge></div>
                <div className="flex items-center justify-between"><span className="text-muted-foreground">启动</span><span className="font-medium">{form.start ? "创建后启动" : "保持关机"}</span></div>
              </CardContent>
            </Card>

            {error ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert">{error}</p> : null}
            <Button type="submit" className="w-full" disabled={!canCreate || mutation.isPending}>
              {mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
              创建虚拟机
            </Button>
            <p className="flex items-center gap-1.5 text-xs text-muted-foreground"><Server className="h-3.5 w-3.5" />虚拟机状态始终以 libvirt 为准。</p>
          </div>
        </div>
      </form>
    </>
  );
}

import { useEffect, useRef, useState, type FormEvent } from "react";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, ArrowLeft, LayoutTemplate, LoaderCircle, Plus, Server } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { VMCreateInput, VMFirmware, VMTemplate } from "@/api/types";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useSessionQuery } from "@/features/auth/use-session";
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
  network: string;
  isoPath: string;
  firmware: VMFirmware;
  secureBoot: boolean;
  cloudInit: boolean;
  cloudInitUserData: string;
  autostart: boolean;
  start: boolean;
};

const initialForm: FormState = {
  name: "",
  vcpus: "2",
  memoryMiB: "2048",
  diskPath: "",
  diskFormat: "qcow2",
  network: "",
  isoPath: "",
  firmware: "bios",
  secureBoot: false,
  cloudInit: false,
  cloudInitUserData: "",
  autostart: false,
  start: false,
};

export function VMCreatePage() {
  const session = useSessionQuery();
  const networksQuery = useNetworksQuery();
  const poolsQuery = useStoragePoolsQuery();
  const templatesQuery = useTemplatesQuery();
  const search = useSearch({ strict: false }) as { template?: number };
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<FormState>(initialForm);
  const [diskPool, setDiskPool] = useState("");
  const [error, setError] = useState("");
  const [selectedTemplateID, setSelectedTemplateID] = useState("");
  const presetApplied = useRef(false);
  const activeNetworks = (networksQuery.data?.items ?? []).filter((network) => network.active);
  const activePools = (poolsQuery.data?.items ?? []).filter((pool) => pool.state === "active");
  const volumesQuery = useStorageVolumesQuery(diskPool);
  const diskVolumes = (volumesQuery.data?.items ?? []).filter((volume) => {
    const format = (volume.format || "").toLowerCase();
    return (format === "qcow2" || format === "raw") && /\.(qcow2|raw)$/i.test(volume.name);
  });
  const selectedVolume = diskVolumes.find((volume) => volume.path === form.diskPath);
  const templates = templatesQuery.data?.items ?? [];
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
    mutation.mutate({
      name: form.name.trim(),
      vcpus: Number(form.vcpus),
      memoryMiB: Number(form.memoryMiB),
      diskPath: form.diskPath.trim(),
      diskFormat: form.diskFormat,
      network: form.network.trim(),
      isoPath: form.isoPath.trim(),
      firmware: form.firmware,
      secureBoot: form.secureBoot,
      cloudInit: form.cloudInit ? { userData: form.cloudInitUserData.trim(), pool: diskPool } : undefined,
      autostart: form.autostart,
      start: form.start,
    });
  }

  return (
    <>
      <PageHeader
        title="新建虚拟机"
        breadcrumb={<span className="inline-flex items-center gap-1.5"><Link to="/vms" className="inline-flex items-center gap-1 hover:text-foreground"><ArrowLeft className="h-3 w-3" />虚拟机</Link><span>/</span><span>新建</span></span>}
        description="定义一个持久化 libvirt 域。首版要求使用宿主机上已存在的磁盘文件。"
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
              <div className="space-y-2">
                <Label htmlFor="vm-network">虚拟网络</Label>
                <select id="vm-network" value={form.network} onChange={(event) => update("network", event.target.value)} required disabled={!canCreate || !activeNetworks.length} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                  {!activeNetworks.length ? <option value="">{networksQuery.isLoading ? "正在读取网络..." : "没有活跃虚拟网络"}</option> : null}
                  {activeNetworks.map((network) => <option key={network.uuid || network.name} value={network.name}>{network.name}{network.bridge ? ` · ${network.bridge}` : ""}</option>)}
                </select>
                <p className="text-xs text-muted-foreground">{networksQuery.isError ? "网络列表加载失败，请刷新页面后重试。" : `${activeNetworks.length} 个活跃虚拟网络可选。`}</p>
              </div>
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="vm-iso-path">ISO 路径（可选）</Label>
                <Input id="vm-iso-path" value={form.isoPath} onChange={(event) => update("isoPath", event.target.value)} placeholder="/var/lib/libvirt/images/install.iso" disabled={!canCreate} />
                <p className="text-xs text-muted-foreground">填写后以 CD-ROM 启动；留空则从硬盘启动。</p>
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
                        ? '#cloud-config\nruncmd:\n  - echo "VirtMgr cloud-init ready" > /var/log/virtmgr-cloud-init.log\n'
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
                  <div className="space-y-2">
                    <Label htmlFor="vm-cloud-init-user-data">Cloud-init 用户数据</Label>
                    <textarea
                      id="vm-cloud-init-user-data"
                      value={form.cloudInitUserData}
                      onChange={(event) => update("cloudInitUserData", event.target.value)}
                      required
                      disabled={!canCreate}
                      rows={8}
                      spellCheck={false}
                      className="w-full rounded-md border border-input bg-card px-3 py-2 font-mono text-xs shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
                    />
                    <p className="text-xs text-muted-foreground">seed ISO 将写入存储池 <span className="font-medium text-foreground">{diskPool || "-"}</span>；审计只记录启用状态和字节数，不保存用户数据内容。</p>
                  </div>
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
                <div className="flex items-center justify-between"><span className="text-muted-foreground">固件</span><span className="font-medium">{form.firmware === "uefi" ? "UEFI" : "BIOS"}{form.secureBoot ? " · Secure Boot" : ""}</span></div>
                <div className="flex items-center justify-between"><span className="text-muted-foreground">Cloud-init</span><span className="font-medium">{form.cloudInit ? "已启用" : "未启用"}</span></div>
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

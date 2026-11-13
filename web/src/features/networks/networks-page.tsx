import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Activity, AlertTriangle, CheckCircle2, Download, GitBranch, Info, ListPlus, LoaderCircle, Network, Pencil, Play, Plus, RefreshCw, Router, ScrollText, Square, Trash2, Upload } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { NetworkAddress, NetworkDHCPHost, NetworkDHCPHostInput, VirtualNetwork, VirtualNetworkAction, VirtualNetworkCreateInput } from "@/api/types";
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
import { networkActionHint, networkEditDisabledReason, networkEditForm, networkUpdateInput, type NetworkEditFormState } from "@/features/networks/network-edit-model";
import { networksQueryKey, useNetworksQuery } from "@/features/networks/use-networks-query";
import { networkLeasesQueryKey, useNetworkLeasesQuery } from "@/features/networks/use-network-leases-query";

type CreateFormState = {
  name: string;
  mode: VirtualNetworkCreateInput["mode"];
  subnet: string;
  dhcpStart: string;
  dhcpEnd: string;
  autostart: boolean;
  start: boolean;
};

type HostFormState = {
  mac: string;
  ip: string;
  name: string;
};

const initialCreateForm: CreateFormState = {
  name: "",
  mode: "nat",
  subnet: "10.0.0.0/24",
  dhcpStart: "",
  dhcpEnd: "",
  autostart: false,
  start: false,
};

const initialHostForm: HostFormState = { mac: "", ip: "", name: "" };

function addressLabel(address: NetworkAddress) {
  return `${address.address}/${address.prefix}`;
}

function actionError(cause: unknown, fallback: string) {
  return cause instanceof ApiError ? cause.message : fallback;
}

function staticHosts(network?: VirtualNetwork): NetworkDHCPHost[] {
  if (!network) return [];
  return network.addresses
    .flatMap((address) => address.hosts ?? [])
    .sort((left, right) => left.ip.localeCompare(right.ip, undefined, { numeric: true }));
}

function staticHostCount(network: VirtualNetwork) {
  return network.addresses.reduce((total, address) => total + (address.hosts?.length ?? 0), 0);
}

const leaseTimeFormatter = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric", month: "2-digit", day: "2-digit",
  hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false,
});

function leaseExpiry(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : leaseTimeFormatter.format(date);
}

export function NetworksPage() {
  const session = useSessionQuery();
  const queryClient = useQueryClient();
  const query = useNetworksQuery();
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState<CreateFormState>(initialCreateForm);
  const [createError, setCreateError] = useState("");
  const [editTarget, setEditTarget] = useState<VirtualNetwork | null>(null);
  const [editForm, setEditForm] = useState<NetworkEditFormState>({ mode: "nat", subnet: "", dhcpStart: "", dhcpEnd: "", autostart: false });
  const [editError, setEditError] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [actionErrorText, setActionErrorText] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<VirtualNetwork | null>(null);
  const [deleteName, setDeleteName] = useState("");
  const [deleteError, setDeleteError] = useState("");
  const [leasesNetwork, setLeasesNetwork] = useState<string | null>(null);
  const [hostsNetwork, setHostsNetwork] = useState<string | null>(null);
  const [hostForm, setHostForm] = useState<HostFormState>(initialHostForm);
  const [hostError, setHostError] = useState("");
  const [hostTransfer, setHostTransfer] = useState<"import" | "export" | "">("");
  const [hostDeleteTarget, setHostDeleteTarget] = useState<{ network: VirtualNetwork; host: NetworkDHCPHost } | null>(null);
  const hostFileInput = useRef<HTMLInputElement>(null);
  const networks = query.data?.items ?? [];
  const leasesQuery = useNetworkLeasesQuery(leasesNetwork);
  const leases = leasesQuery.data?.items ?? [];
  const leasesNetworkRecord = networks.find((network) => network.name === leasesNetwork);
  const hostsNetworkRecord = networks.find((network) => network.name === hostsNetwork);
  const hosts = staticHosts(hostsNetworkRecord);
  const hostsHaveDHCP = Boolean(hostsNetworkRecord?.addresses.some((address) => (address.family === "" || address.family === "ipv4") && (address.dhcpEnabled || address.dhcpStart)));
  const active = networks.filter((network) => network.active).length;
  const bridges = networks.filter((network) => Boolean(network.bridge)).length;
  const nat = networks.filter((network) => network.forwardMode === "nat").length;
  const readOnly = Boolean(session.data?.readOnly);
  const viewer = session.data?.user.role === "viewer";
  const canManage = Boolean(session.data) && !readOnly && !viewer;
  const disabledReason = readOnly ? "系统当前处于只读模式" : viewer ? "当前账号没有管理网络的权限" : "";

  const createMutation = useMutation({
    mutationFn: (input: VirtualNetworkCreateInput) => api.createNetwork(input),
    onSuccess: (network) => {
      setSuccessMessage(`网络 ${network.name} 已创建。`);
      setCreateError("");
      setActionErrorText("");
      setCreateOpen(false);
      setCreateForm(initialCreateForm);
      void queryClient.invalidateQueries({ queryKey: networksQueryKey });
    },
    onError: (cause) => {
      setCreateError(actionError(cause, "创建虚拟网络失败，请稍后重试。"));
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ name, input }: { name: string; input: ReturnType<typeof networkUpdateInput> }) => api.updateNetwork(name, input),
    onSuccess: (network) => {
      setSuccessMessage(`网络 ${network.name} 配置已更新。`);
      setEditError("");
      setActionErrorText("");
      setEditTarget(null);
      void queryClient.invalidateQueries({ queryKey: networksQueryKey });
      void queryClient.invalidateQueries({ queryKey: networkLeasesQueryKey(network.name) });
    },
    onError: (cause) => {
      setEditError(actionError(cause, "更新虚拟网络失败，请稍后重试。"));
    },
  });

  const actionMutation = useMutation({
    mutationFn: ({ name, action }: { name: string; action: VirtualNetworkAction }) => api.networkAction(name, action),
    onSuccess: (_, variables) => {
      setSuccessMessage(`网络 ${variables.name} 已${variables.action === "start" ? "启动" : "停止"}。`);
      setActionErrorText("");
      void queryClient.invalidateQueries({ queryKey: networksQueryKey });
      void queryClient.invalidateQueries({ queryKey: networkLeasesQueryKey(variables.name) });
    },
    onError: (cause) => {
      setActionErrorText(actionError(cause, "更新网络状态失败，请稍后重试。"));
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (name: string) => api.deleteNetwork(name),
    onSuccess: (_, name) => {
      setSuccessMessage(`网络 ${name} 已删除。`);
      setActionErrorText("");
      setDeleteTarget(null);
      setDeleteName("");
      setDeleteError("");
      void queryClient.invalidateQueries({ queryKey: networksQueryKey });
      setLeasesNetwork((current) => current === name ? null : current);
      setHostsNetwork((current) => current === name ? null : current);
      queryClient.removeQueries({ queryKey: networkLeasesQueryKey(name) });
    },
    onError: (cause) => {
      setDeleteError(actionError(cause, "删除虚拟网络失败，请稍后重试。"));
    },
  });

  const addHostMutation = useMutation({
    mutationFn: ({ name, input }: { name: string; input: NetworkDHCPHostInput }) => api.addNetworkDHCPHost(name, input),
    onSuccess: (network) => {
      setSuccessMessage(`网络 ${network.name} 的静态主机条目已添加，${network.active ? "已立即生效。" : "将在网络启动后生效。"}`);
      setHostError("");
      setActionErrorText("");
      setHostForm(initialHostForm);
      void queryClient.invalidateQueries({ queryKey: networksQueryKey });
    },
    onError: (cause) => {
      setHostError(actionError(cause, "添加静态主机失败，请稍后重试。"));
    },
  });

  const deleteHostMutation = useMutation({
    mutationFn: ({ name, host }: { name: string; host: NetworkDHCPHost }) => api.deleteNetworkDHCPHost(name, host.mac),
    onSuccess: (network, variables) => {
      setSuccessMessage(`网络 ${network.name} 的静态主机 ${variables.host.mac} 已删除。`);
      setHostError("");
      setActionErrorText("");
      setHostDeleteTarget(null);
      void queryClient.invalidateQueries({ queryKey: networksQueryKey });
    },
    onError: (cause) => {
      setHostError(actionError(cause, "删除静态主机失败，请稍后重试。"));
    },
  });

  function updateCreateForm<K extends keyof CreateFormState>(key: K, value: CreateFormState[K]) {
    setCreateForm((form) => ({ ...form, [key]: value }));
  }

  function updateEditForm<K extends keyof NetworkEditFormState>(key: K, value: NetworkEditFormState[K]) {
    setEditForm((form) => ({ ...form, [key]: value }));
  }

  function updateHostForm<K extends keyof HostFormState>(key: K, value: HostFormState[K]) {
    setHostForm((form) => ({ ...form, [key]: value }));
  }

  async function exportHosts(network: string) {
    setHostError("");
    setHostTransfer("export");
    try {
      const { blob, filename } = await api.exportNetworkDHCPHosts(network);
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = filename;
      anchor.click();
      URL.revokeObjectURL(url);
      setSuccessMessage(`网络 ${network} 的静态主机已导出：${filename}`);
    } catch (cause) {
      setHostError(actionError(cause, "导出静态主机失败，请稍后重试。"));
    } finally {
      setHostTransfer("");
    }
  }

  async function importHosts(network: string, file: File) {
    setHostError("");
    if (file.size > 1 << 20) {
      setHostError("CSV 文件不能超过 1 MiB。");
      return;
    }
    setHostTransfer("import");
    try {
      const result = await api.importNetworkDHCPHosts(network, await file.text());
      setSuccessMessage(`网络 ${network} 静态主机导入完成：新增 ${result.imported} 条，跳过 ${result.skipped} 条。`);
      void queryClient.invalidateQueries({ queryKey: networksQueryKey });
    } catch (cause) {
      setHostError(actionError(cause, "导入静态主机失败，请检查 CSV 格式后重试。"));
    } finally {
      setHostTransfer("");
      if (hostFileInput.current) hostFileInput.current.value = "";
    }
  }

  function submitCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canManage) return;
    setCreateError("");
    setSuccessMessage("");
    setActionErrorText("");
    createMutation.mutate({
      name: createForm.name.trim(),
      mode: createForm.mode,
      subnet: createForm.subnet.trim(),
      dhcpStart: createForm.dhcpStart.trim() || undefined,
      dhcpEnd: createForm.dhcpEnd.trim() || undefined,
      autostart: createForm.autostart,
      start: createForm.start,
    });
  }

  function openEdit(network: VirtualNetwork) {
    setEditTarget(network);
    setEditForm(networkEditForm(network));
    setEditError("");
    setSuccessMessage("");
    setActionErrorText("");
    setCreateOpen(false);
  }

  function submitEdit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editTarget || !canManage) return;
    setEditError("");
    setSuccessMessage("");
    setActionErrorText("");
    updateMutation.mutate({ name: editTarget.name, input: networkUpdateInput(editForm) });
  }

  function runAction(network: VirtualNetwork, action: VirtualNetworkAction) {
    if (!canManage) return;
    setSuccessMessage("");
    setActionErrorText("");
    actionMutation.mutate({ name: network.name, action });
  }

  function editDisabledReason(network: VirtualNetwork) {
    return networkEditDisabledReason(session.data?.user.role, readOnly, network);
  }

  function openDelete(network: VirtualNetwork) {
    setDeleteTarget(network);
    setDeleteName("");
    setDeleteError("");
  }

  function toggleHosts(network: VirtualNetwork) {
    setHostsNetwork((current) => current === network.name ? null : network.name);
    setHostError("");
    setHostForm(initialHostForm);
    setSuccessMessage("");
    setActionErrorText("");
  }

  function submitHost(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!hostsNetwork || !canManage || !hostsHaveDHCP) return;
    setHostError("");
    setSuccessMessage("");
    addHostMutation.mutate({
      name: hostsNetwork,
      input: {
        mac: hostForm.mac.trim(),
        ip: hostForm.ip.trim(),
        name: hostForm.name.trim() || undefined,
      },
    });
  }

  return (
    <>
      <PageHeader
        title="虚拟网络"
        breadcrumb="总览 / 网络"
        description="管理 libvirt 虚拟网络、NAT、桥接、DHCP 范围和自动启动策略。"
        actions={(
          <Button
            data-testid="network-create-toggle"
            disabled={!canManage || createMutation.isPending}
            title={disabledReason || undefined}
            onClick={() => { setCreateOpen((open) => !open); setCreateError(""); }}
          >
            <Plus className="h-4 w-4" />新建网络
          </Button>
        )}
      />

      {successMessage ? (
        <div className="mb-4 flex items-center gap-2 rounded-lg border border-success/30 bg-success/10 px-4 py-3 text-sm text-success" role="status" data-testid="network-action-success">
          <CheckCircle2 className="h-4 w-4" />{successMessage}
        </div>
      ) : null}
      {actionErrorText ? (
        <p className="mb-4 flex items-center gap-2 rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive" role="alert" data-testid="network-action-error">
          <AlertTriangle className="h-4 w-4" />{actionErrorText}
        </p>
      ) : null}

      {createOpen ? (
        <Card className="mb-4">
          <CardHeader>
            <CardTitle>创建虚拟网络</CardTitle>
            <CardDescription>创建持久化网络定义。子网至少为 /29，网关固定为网络地址加一。</CardDescription>
          </CardHeader>
          <CardContent>
            <form id="network-create-form" data-testid="network-create-form" onSubmit={submitCreate} className="space-y-4">
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                <div className="space-y-2">
                  <Label htmlFor="network-name">网络名称</Label>
                  <Input id="network-name" value={createForm.name} onChange={(event) => updateCreateForm("name", event.target.value)} placeholder="lab-net" required disabled={!canManage} autoComplete="off" />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="network-mode">网络模式</Label>
                  <select id="network-mode" value={createForm.mode} onChange={(event) => updateCreateForm("mode", event.target.value as CreateFormState["mode"])} disabled={!canManage} className="flex h-9 w-full rounded-md border border-input bg-card px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 disabled:opacity-50">
                    <option value="nat">NAT 转发</option>
                    <option value="isolated">隔离网络</option>
                  </select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="network-subnet">IPv4 子网</Label>
                  <Input id="network-subnet" value={createForm.subnet} onChange={(event) => updateCreateForm("subnet", event.target.value)} placeholder="10.20.30.0/24" required disabled={!canManage} />
                </div>
                <div className="flex flex-col justify-end gap-2 pb-1 text-sm">
                  <label className="flex items-center gap-2">
                    <input type="checkbox" className="h-4 w-4" checked={createForm.autostart} onChange={(event) => updateCreateForm("autostart", event.target.checked)} disabled={!canManage} />
                    开机自动启动
                  </label>
                  <label className="flex items-center gap-2">
                    <input type="checkbox" className="h-4 w-4" checked={createForm.start} onChange={(event) => updateCreateForm("start", event.target.checked)} disabled={!canManage} />
                    创建后立即启动
                  </label>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="network-dhcp-start">DHCP 起始地址（可选）</Label>
                  <Input id="network-dhcp-start" value={createForm.dhcpStart} onChange={(event) => updateCreateForm("dhcpStart", event.target.value)} placeholder="10.20.30.10" disabled={!canManage} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="network-dhcp-end">DHCP 结束地址（可选）</Label>
                  <Input id="network-dhcp-end" value={createForm.dhcpEnd} onChange={(event) => updateCreateForm("dhcpEnd", event.target.value)} placeholder="10.20.30.200" disabled={!canManage} />
                </div>
              </div>
              {createError ? (
                <p className="flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert" data-testid="network-create-error">
                  <AlertTriangle className="h-3.5 w-3.5" />{createError}
                </p>
              ) : null}
              <div className="flex flex-wrap items-center justify-between gap-3">
                <p className="text-xs text-muted-foreground">DHCP 起止地址留空时自动使用子网内可用范围；只填写一项将被拒绝。</p>
                <div className="flex items-center gap-2">
                  <Button type="button" variant="ghost" size="sm" onClick={() => { setCreateOpen(false); setCreateError(""); }}>取消</Button>
                  <Button type="submit" size="sm" data-testid="network-create-submit" disabled={!canManage || createMutation.isPending}>
                    {createMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
                    创建网络
                  </Button>
                </div>
              </div>
            </form>
          </CardContent>
        </Card>
      ) : null}

      {editTarget ? (
        <Card className="mb-4">
          <CardHeader>
            <CardTitle>编辑虚拟网络 · {editTarget.name}</CardTitle>
            <CardDescription>网络名称不可修改。保存后将在网络下次启动时使用新配置。</CardDescription>
          </CardHeader>
          <CardContent>
            <form id="network-edit-form" data-testid="network-edit-form" onSubmit={submitEdit} className="space-y-4">
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                <div className="space-y-2">
                  <Label htmlFor="network-edit-name">网络名称</Label>
                  <Input id="network-edit-name" value={editTarget.name} readOnly disabled className="font-mono" />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="network-edit-mode">网络模式</Label>
                  <select id="network-edit-mode" value={editForm.mode} onChange={(event) => updateEditForm("mode", event.target.value as NetworkEditFormState["mode"])} disabled={updateMutation.isPending} className="flex h-9 w-full rounded-md border border-input bg-card px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 disabled:opacity-50">
                    <option value="nat">NAT 转发</option>
                    <option value="isolated">隔离网络</option>
                  </select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="network-edit-subnet">IPv4 子网</Label>
                  <Input id="network-edit-subnet" value={editForm.subnet} onChange={(event) => updateEditForm("subnet", event.target.value)} placeholder="10.20.30.0/24" required disabled={updateMutation.isPending} />
                </div>
                <div className="flex flex-col justify-end gap-2 pb-1 text-sm">
                  <label className="flex items-center gap-2">
                    <input type="checkbox" className="h-4 w-4" checked={editForm.autostart} onChange={(event) => updateEditForm("autostart", event.target.checked)} disabled={updateMutation.isPending} />
                    开机自动启动
                  </label>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="network-edit-dhcp-start">DHCP 起始地址（可选）</Label>
                  <Input id="network-edit-dhcp-start" value={editForm.dhcpStart} onChange={(event) => updateEditForm("dhcpStart", event.target.value)} placeholder="10.20.30.10" disabled={updateMutation.isPending} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="network-edit-dhcp-end">DHCP 结束地址（可选）</Label>
                  <Input id="network-edit-dhcp-end" value={editForm.dhcpEnd} onChange={(event) => updateEditForm("dhcpEnd", event.target.value)} placeholder="10.20.30.200" disabled={updateMutation.isPending} />
                </div>
              </div>
              {editError ? (
                <p className="flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert" data-testid="network-edit-error">
                  <AlertTriangle className="h-3.5 w-3.5" />{editError}
                </p>
              ) : null}
              <div className="flex flex-wrap items-center justify-between gap-3">
                <p className="text-xs text-muted-foreground">修改网关或子网会重建网络配置；保存前请确认没有虚拟机依赖当前地址。</p>
                <div className="flex items-center gap-2">
                  <Button type="button" variant="ghost" size="sm" disabled={updateMutation.isPending} onClick={() => { setEditTarget(null); setEditError(""); }}>取消</Button>
                  <Button type="submit" size="sm" data-testid="network-edit-submit" disabled={updateMutation.isPending}>
                    {updateMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Pencil className="h-4 w-4" />}
                    保存配置
                  </Button>
                </div>
              </div>
            </form>
          </CardContent>
        </Card>
      ) : null}

      {query.isLoading ? <LoadingState label="正在读取虚拟网络" /> : null}
      {query.isError ? <ErrorState message={query.error instanceof Error ? query.error.message : "无法读取虚拟网络"} onRetry={() => void query.refetch()} /> : null}

      {query.data ? (
        <>
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <MetricCard label="虚拟网络" value={query.data.total} hint="已定义的全部网络" icon={Network} tone="primary" testId="network-total" />
            <MetricCard label="活跃网络" value={active} hint={`${networks.length - active} 个未激活`} icon={GitBranch} tone={active ? "success" : "neutral"} testId="network-active" />
            <MetricCard label="桥接网络" value={bridges} hint="已配置 bridge 名称" icon={Router} tone="neutral" testId="network-bridges" />
            <MetricCard label="NAT 网络" value={nat} hint="宿主转发模式为 NAT" icon={Activity} tone={nat ? "warning" : "neutral"} testId="network-nat" />
          </div>

          <Card className="mt-4">
            <CardHeader>
              <CardTitle>网络列表</CardTitle>
              <CardDescription>地址、DHCP 范围、桥接口和自动启动状态来自 libvirt XML。</CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              {networks.length ? (
                <Table className="min-w-[1080px]">
                  <TableHeader><TableRow><TableHead>名称</TableHead><TableHead>模式</TableHead><TableHead>状态</TableHead><TableHead>桥接</TableHead><TableHead>地址 / DHCP</TableHead><TableHead>自动启动</TableHead><TableHead>操作</TableHead></TableRow></TableHeader>
                  <TableBody>
                    {networks.map((network) => {
                      const hint = networkActionHint(session.data?.user.role, readOnly, network);
                      const editReason = editDisabledReason(network);
                      const hintID = `network-action-hint-${network.name}`;
                      return (
                      <TableRow key={network.uuid || network.name} data-testid={`network-row-${network.name}`}>
                        <TableCell>
                          <p className="font-medium">{network.name}</p>
                          <p className="font-mono text-[10px] text-muted-foreground">{network.uuid || "-"}</p>
                        </TableCell>
                        <TableCell><Badge variant="outline">{network.forwardMode || "isolated"}</Badge></TableCell>
                        <TableCell><Badge variant={network.active ? "success" : "secondary"}>{network.active ? "活跃" : "未激活"}</Badge></TableCell>
                        <TableCell><span className="font-mono text-xs">{network.bridge || "-"}</span></TableCell>
                        <TableCell>
                          {network.addresses.length ? network.addresses.map((address) => (
                            <div key={`${address.family}-${address.address}`} className="py-0.5">
                              <p className="font-mono text-xs">{addressLabel(address)}</p>
                              {address.dhcpStart || address.dhcpEnd ? (
                                <p className="font-mono text-[10px] text-muted-foreground">DHCP {address.dhcpStart || "-"} - {address.dhcpEnd || "-"}</p>
                              ) : null}
                              {address.hosts?.length ? (
                                <p className="text-[10px] text-muted-foreground">静态主机 {address.hosts.length} 条</p>
                              ) : null}
                            </div>
                          )) : <span className="text-xs text-muted-foreground">未配置地址</span>}
                        </TableCell>
                        <TableCell>{network.autostart ? "是" : "否"}</TableCell>
                        <TableCell className="align-top">
                          <div className="flex min-w-[17rem] flex-col gap-2">
                            <div className="flex flex-wrap items-center gap-1.5">
                              {canManage ? (
                                <>
                                  <Button
                                    type="button"
                                    variant={editTarget?.name === network.name ? "secondary" : "outline"}
                                    size="sm"
                                    data-testid={`network-edit-${network.name}`}
                                    aria-label={`编辑网络 ${network.name}`}
                                    aria-describedby={editReason ? hintID : undefined}
                                    title={editReason || "编辑网络配置"}
                                    disabled={Boolean(editReason) || updateMutation.isPending}
                                    onClick={() => openEdit(network)}
                                  >
                                    <Pencil className="h-3.5 w-3.5" />编辑
                                  </Button>
                                  {network.active ? (
                                    <Button
                                      type="button"
                                      variant="outline"
                                      size="sm"
                                      data-testid={`network-stop-${network.name}`}
                                      aria-label={`停止网络 ${network.name}`}
                                      aria-describedby={hint ? hintID : undefined}
                                      title="停止网络"
                                      disabled={actionMutation.isPending}
                                      onClick={() => runAction(network, "stop")}
                                    >
                                      {actionMutation.isPending && actionMutation.variables?.name === network.name && actionMutation.variables.action === "stop"
                                        ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" />
                                        : <Square className="h-3.5 w-3.5" />}停止
                                    </Button>
                                  ) : (
                                    <Button
                                      type="button"
                                      variant="outline"
                                      size="sm"
                                      data-testid={`network-start-${network.name}`}
                                      aria-label={`启动网络 ${network.name}`}
                                      title="启动网络"
                                      disabled={actionMutation.isPending}
                                      onClick={() => runAction(network, "start")}
                                    >
                                      {actionMutation.isPending && actionMutation.variables?.name === network.name && actionMutation.variables.action === "start"
                                        ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" />
                                        : <Play className="h-3.5 w-3.5" />}启动
                                    </Button>
                                  )}
                                  <Button
                                    type="button"
                                    variant="ghost"
                                    size="sm"
                                    className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                                    data-testid={`network-delete-${network.name}`}
                                    aria-label={`删除网络 ${network.name}`}
                                    aria-describedby={network.active ? hintID : undefined}
                                    title={network.active ? "请先停止网络再删除" : "删除网络定义"}
                                    disabled={network.active || deleteMutation.isPending}
                                    onClick={() => openDelete(network)}
                                  >
                                    <Trash2 className="h-3.5 w-3.5" />删除
                                  </Button>
                                </>
                              ) : null}
                              <Button
                                type="button"
                                variant={leasesNetwork === network.name ? "secondary" : "outline"}
                                size="sm"
                                data-testid={`network-leases-${network.name}`}
                                aria-label={`查看网络 ${network.name} 的 DHCP 租约`}
                                title="查看 DHCP 租约"
                                onClick={() => setLeasesNetwork((current) => current === network.name ? null : network.name)}
                              >
                                <ScrollText className="h-3.5 w-3.5" />租约
                              </Button>
                              <Button
                                type="button"
                                variant={hostsNetwork === network.name ? "secondary" : "outline"}
                                size="sm"
                                data-testid={`network-hosts-${network.name}`}
                                aria-label={`管理网络 ${network.name} 的静态主机`}
                                title={staticHostCount(network) ? `管理静态主机（${staticHostCount(network)} 条）` : "管理静态主机条目"}
                                onClick={() => toggleHosts(network)}
                              >
                                <ListPlus className="h-3.5 w-3.5" />静态主机
                              </Button>
                            </div>
                            {hint ? (
                              <p id={hintID} data-testid={`network-action-hint-${network.name}`} className="flex items-start gap-1.5 text-[11px] leading-relaxed text-muted-foreground">
                                <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" />{hint}
                              </p>
                            ) : null}
                          </div>
                        </TableCell>
                      </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              ) : <p className="p-5 text-sm text-muted-foreground">当前宿主机没有定义虚拟网络。</p>}
            </CardContent>
          </Card>

          {leasesNetwork ? (
            <Card className="mt-4" data-testid="network-leases-card">
              <CardHeader>
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <CardTitle>DHCP 租约 · {leasesNetwork}</CardTitle>
                    <CardDescription>
                      来自 libvirt 网络 DHCP lease。未激活网络不会返回租约。
                    </CardDescription>
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge variant={leasesNetworkRecord?.active ? "success" : "secondary"}>{leasesNetworkRecord?.active ? "活跃" : "未激活"}</Badge>
                    <Button variant="outline" size="sm" onClick={() => void leasesQuery.refetch()} disabled={leasesQuery.isFetching}>
                      <RefreshCw className={leasesQuery.isFetching ? "h-4 w-4 animate-spin" : "h-4 w-4"} />刷新
                    </Button>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="p-0">
                {leasesQuery.isLoading ? <LoadingState label="正在读取 DHCP 租约" /> : null}
                {leasesQuery.isError ? <ErrorState message={leasesQuery.error instanceof Error ? leasesQuery.error.message : "无法读取 DHCP 租约"} onRetry={() => void leasesQuery.refetch()} className="m-4" /> : null}
                {leasesQuery.data && leases.length === 0 ? (
                  <div className="flex flex-col items-center px-4 py-10 text-center" data-testid="network-leases-empty">
                    <ScrollText className="h-8 w-8 text-muted-foreground" />
                    <p className="mt-3 font-medium">暂无 DHCP 租约</p>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {leasesNetworkRecord?.active ? "该网络当前没有客户端取得租约。" : "网络未激活，或未配置 DHCP 地址池。"}
                    </p>
                  </div>
                ) : null}
                {leasesQuery.data && leases.length > 0 ? (
                  <Table className="min-w-[880px]">
                    <TableHeader><TableRow><TableHead>主机名</TableHead><TableHead>IP 地址</TableHead><TableHead>MAC</TableHead><TableHead>接口</TableHead><TableHead>客户端标识</TableHead><TableHead>到期时间</TableHead></TableRow></TableHeader>
                    <TableBody>
                      {leases.map((lease, index) => (
                        <TableRow key={`${lease.ipAddress}-${lease.mac}-${index}`} data-testid={`network-lease-row-${index}`}>
                          <TableCell>{lease.hostname || "-"}</TableCell>
                          <TableCell><span className="font-mono text-xs">{lease.ipAddress}/{lease.prefix}</span></TableCell>
                          <TableCell><span className="font-mono text-xs">{lease.mac || "-"}</span></TableCell>
                          <TableCell><span className="font-mono text-xs">{lease.interface || "-"}</span></TableCell>
                          <TableCell><span className="block max-w-56 truncate font-mono text-[10px] text-muted-foreground" title={[lease.clientId, lease.iaid].filter(Boolean).join(" · ")}>{[lease.clientId, lease.iaid].filter(Boolean).join(" · ") || "-"}</span></TableCell>
                          <TableCell><span className="whitespace-nowrap text-xs text-muted-foreground">{leaseExpiry(lease.expiresAt)}</span></TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                ) : null}
              </CardContent>
            </Card>
          ) : null}

          {hostsNetwork ? (
            <Card className="mt-4" data-testid="network-hosts-card">
              <CardHeader>
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <CardTitle>静态 DHCP 主机 · {hostsNetwork}</CardTitle>
                    <CardDescription>
                      {hostsNetworkRecord?.active
                        ? "网络正在运行：条目会立即应用到运行配置，并同步写入持久配置，无需停止网络。"
                        : "网络未激活：条目会写入持久配置，并在网络下次启动时生效。"}
                    </CardDescription>
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant={hostsNetworkRecord?.active ? "success" : "secondary"}>{hostsNetworkRecord?.active ? "实时生效" : "待启动生效"}</Badge>
                    <Button type="button" variant="outline" size="sm" data-testid={`network-hosts-export-${hostsNetwork}`} disabled={hostTransfer !== ""} onClick={() => void exportHosts(hostsNetwork)}>
                      {hostTransfer === "export" ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}导出 CSV
                    </Button>
                    <Button type="button" variant="outline" size="sm" data-testid={`network-hosts-import-${hostsNetwork}`} disabled={!canManage || hostTransfer !== "" || !hostsHaveDHCP} title={disabledReason || "导入 CSV 静态主机"} onClick={() => hostFileInput.current?.click()}>
                      {hostTransfer === "import" ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}导入 CSV
                    </Button>
                    <input ref={hostFileInput} type="file" accept=".csv,text/csv" className="sr-only" data-testid={`network-hosts-import-file-${hostsNetwork}`} onChange={(event) => { const file = event.target.files?.[0]; if (file) void importHosts(hostsNetwork, file); }} />
                  </div>
                </div>
              </CardHeader>
              <CardContent className="space-y-4">
                {!hostsHaveDHCP ? (
                  <p className="flex items-start gap-2 rounded-md border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning" role="status">
                    <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />该网络没有可用的 IPv4 DHCP 地址池，无法添加静态主机条目。
                  </p>
                ) : null}
                {!canManage ? (
                  <p className="flex items-start gap-2 rounded-md border bg-muted/40 px-3 py-2 text-xs text-muted-foreground" role="status">
                    <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" />{disabledReason || "当前账号只能查看静态主机条目。"}
                  </p>
                ) : null}
                {canManage && hostsHaveDHCP ? (
                  <form id="network-host-form" data-testid="network-host-form" onSubmit={submitHost} className="space-y-3 rounded-lg border bg-muted/20 p-3">
                    <div className="grid gap-3 md:grid-cols-3">
                      <div className="space-y-1.5">
                        <Label htmlFor="network-host-mac">MAC 地址</Label>
                        <Input id="network-host-mac" value={hostForm.mac} onChange={(event) => updateHostForm("mac", event.target.value)} placeholder="52:54:00:12:34:56" required autoComplete="off" disabled={addHostMutation.isPending} className="font-mono" />
                      </div>
                      <div className="space-y-1.5">
                        <Label htmlFor="network-host-ip">IPv4 地址</Label>
                        <Input id="network-host-ip" value={hostForm.ip} onChange={(event) => updateHostForm("ip", event.target.value)} placeholder="192.168.122.50" required autoComplete="off" disabled={addHostMutation.isPending} className="font-mono" />
                      </div>
                      <div className="space-y-1.5">
                        <Label htmlFor="network-host-name">主机名（可选）</Label>
                        <Input id="network-host-name" value={hostForm.name} onChange={(event) => updateHostForm("name", event.target.value)} placeholder="web-01" autoComplete="off" disabled={addHostMutation.isPending} />
                      </div>
                    </div>
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <p className="text-xs text-muted-foreground">IP 必须位于当前子网内，且不能是网关、网络地址或广播地址。CSV 列为 <span className="font-mono">mac,ip,name</span>，同名同 IP 条目会跳过。</p>
                      <Button type="submit" size="sm" data-testid="network-host-submit" disabled={addHostMutation.isPending}>
                        {addHostMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
                        添加静态主机
                      </Button>
                    </div>
                  </form>
                ) : null}
                {hostError ? (
                  <p className="flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert" data-testid="network-host-error">
                    <AlertTriangle className="h-3.5 w-3.5" />{hostError}
                  </p>
                ) : null}
                <div className="overflow-hidden rounded-md border">
                  {hosts.length ? (
                    <Table className="min-w-[620px]">
                      <TableHeader><TableRow><TableHead>主机名</TableHead><TableHead>IP 地址</TableHead><TableHead>MAC 地址</TableHead><TableHead className="w-20 text-right">操作</TableHead></TableRow></TableHeader>
                      <TableBody>
                        {hosts.map((host) => (
                          <TableRow key={host.mac} data-testid={`network-host-row-${host.mac}`}>
                            <TableCell>{host.name || "-"}</TableCell>
                            <TableCell><span className="font-mono text-xs">{host.ip}</span></TableCell>
                            <TableCell><span className="font-mono text-xs">{host.mac}</span></TableCell>
                            <TableCell className="text-right">
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                                data-testid={`network-host-delete-${host.mac}`}
                                aria-label={`删除静态主机 ${host.mac}`}
                                disabled={!canManage || deleteHostMutation.isPending}
                                title={disabledReason || "删除静态主机"}
                                onClick={() => { if (hostsNetworkRecord) { setHostError(""); setHostDeleteTarget({ network: hostsNetworkRecord, host }); } }}
                              >
                                <Trash2 className="h-3.5 w-3.5" />删除
                              </Button>
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  ) : (
                    <div className="flex flex-col items-center px-4 py-8 text-center" data-testid="network-hosts-empty">
                      <ListPlus className="h-7 w-7 text-muted-foreground" />
                      <p className="mt-2 text-sm font-medium">暂无静态主机条目</p>
                      <p className="mt-1 text-xs text-muted-foreground">静态条目用于为固定 MAC 保留指定 IP，与动态 DHCP 租约分开管理。</p>
                    </div>
                  )}
                </div>
              </CardContent>
            </Card>
          ) : null}
        </>
      ) : null}

      <AlertDialog open={hostDeleteTarget !== null} onOpenChange={(open) => { if (!open) { setHostDeleteTarget(null); setHostError(""); } }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除静态主机条目？</AlertDialogTitle>
            <AlertDialogDescription>
              将从网络 <span className="font-medium text-foreground">{hostDeleteTarget?.network.name}</span> 删除
              <span className="font-mono text-foreground"> {hostDeleteTarget?.host.mac} → {hostDeleteTarget?.host.ip}</span> 的固定地址分配。
              {hostDeleteTarget?.network.active ? "该变更会立即应用到运行中的网络。" : "该变更会写入持久配置。"}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {hostError ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert">{hostError}</p> : null}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteHostMutation.isPending}>取消</AlertDialogCancel>
            <Button
              variant="destructive"
              data-testid="confirm-delete-network-host"
              disabled={deleteHostMutation.isPending}
              onClick={() => {
                if (!hostDeleteTarget) return;
                setHostError("");
                deleteHostMutation.mutate({ name: hostDeleteTarget.network.name, host: hostDeleteTarget.host });
              }}
            >
              {deleteHostMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
              删除条目
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => { if (!open) { setDeleteTarget(null); setDeleteError(""); } }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除虚拟网络？</AlertDialogTitle>
            <AlertDialogDescription>
              将从 libvirt 永久删除网络 <span className="font-medium text-foreground">{deleteTarget?.name}</span>。仅未激活网络可以删除，正在使用该网络的虚拟机不会自动调整配置。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="space-y-2">
            <Label htmlFor="delete-network-name">输入网络名称确认</Label>
            <Input id="delete-network-name" value={deleteName} onChange={(event) => setDeleteName(event.target.value)} placeholder={deleteTarget?.name} autoComplete="off" />
          </div>
          {deleteError ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert">{deleteError}</p> : null}
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <Button
              variant="destructive"
              data-testid="confirm-delete-network"
              disabled={!deleteTarget || deleteName !== deleteTarget.name || deleteMutation.isPending}
              onClick={() => {
                if (!deleteTarget) return;
                setDeleteError("");
                deleteMutation.mutate(deleteTarget.name);
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

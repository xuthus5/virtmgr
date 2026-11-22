import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, Plus } from "lucide-react";
import { api } from "@/api/client";
import type { NetworkDHCPHost, NetworkDHCPHostInput, NetworkDNSHost, NetworkDNSHostInput, VirtualNetwork, VirtualNetworkAction, VirtualNetworkCreateInput } from "@/api/types";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { useSessionQuery } from "@/features/auth/use-session";
import { NetworkCreateCard } from "@/features/networks/network-create-card";
import { NetworkDeleteDialog, NetworkDNSHostDeleteDialog, NetworkHostDeleteDialog } from "@/features/networks/network-dialogs";
import { NetworkDiagnosticsCard } from "@/features/networks/network-diagnostics-card";
import { NetworkDNSHostsCard } from "@/features/networks/network-dns-hosts-card";
import { NetworkEditCard } from "@/features/networks/network-edit-card";
import { networkEditForm, networkUpdateInput, type NetworkEditFormState } from "@/features/networks/network-edit-model";
import { NetworkHostsCard } from "@/features/networks/network-hosts-card";
import { NetworkLeasesCard } from "@/features/networks/network-leases-card";
import { NetworkPortForwardsCard } from "@/features/networks/network-port-forwards-card";
import { NetworkTableCard } from "@/features/networks/network-table-card";
import { actionError, createNetworkInput, hostIdentity, hostIsIPv6, initialCreateForm, initialDNSHostForm, initialHostForm, mtuInputError, splitDNSHostnames, staticDNSHosts, staticHosts, type CreateFormState, type DNSHostFormState, type HostFormState } from "@/features/networks/network-ui";
import { useHostBridgesQuery, useNetworksQuery } from "@/features/networks/use-networks-query";
import { useNetworkDiagnosticsQuery } from "@/features/networks/use-network-diagnostics-query";
import { useNetworkLeasesQuery } from "@/features/networks/use-network-leases-query";
import { queryKeys } from "@/lib/query-keys";

export function NetworksPage() {
  const session = useSessionQuery();
  const queryClient = useQueryClient();
  const query = useNetworksQuery();
  const hostBridgesQuery = useHostBridgesQuery();
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState<CreateFormState>(initialCreateForm);
  const [createError, setCreateError] = useState("");
  const [editTarget, setEditTarget] = useState<VirtualNetwork | null>(null);
  const [editForm, setEditForm] = useState<NetworkEditFormState>({ mode: "nat", subnet: "", dhcpStart: "", dhcpEnd: "", ipv6Subnet: "", ipv6Mode: "dhcpv6", ipv6DhcpStart: "", ipv6DhcpEnd: "", natIpv6: true, dnsDomain: "", dnsServers: "", bridge: "", mtu: "", autostart: false });
  const [editError, setEditError] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [actionErrorText, setActionErrorText] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<VirtualNetwork | null>(null);
  const [deleteName, setDeleteName] = useState("");
  const [deleteError, setDeleteError] = useState("");
  const [leasesNetwork, setLeasesNetwork] = useState<string | null>(null);
  const [hostsNetwork, setHostsNetwork] = useState<string | null>(null);
  const [diagnosticsNetwork, setDiagnosticsNetwork] = useState<string | null>(null);
  const [portForwardsNetwork, setPortForwardsNetwork] = useState<string | null>(null);
  const [hostForm, setHostForm] = useState<HostFormState>(initialHostForm);
  const [hostError, setHostError] = useState("");
  const [hostTransfer, setHostTransfer] = useState<"import" | "export" | "">("");
  const [hostDeleteTarget, setHostDeleteTarget] = useState<{ network: VirtualNetwork; host: NetworkDHCPHost } | null>(null);
  const [dnsHostForm, setDNSHostForm] = useState<DNSHostFormState>(initialDNSHostForm);
  const [dnsHostError, setDNSHostError] = useState("");
  const [dnsHostDeleteTarget, setDNSHostDeleteTarget] = useState<{ network: VirtualNetwork; host: NetworkDNSHost } | null>(null);
  const networks = query.data?.items ?? [];
  const hostBridges = hostBridgesQuery.data?.items ?? [];
  const leasesQuery = useNetworkLeasesQuery(leasesNetwork);
  const diagnosticsQuery = useNetworkDiagnosticsQuery(diagnosticsNetwork);
  const leases = leasesQuery.data?.items ?? [];
  const leasesNetworkRecord = networks.find((network) => network.name === leasesNetwork);
  const hostsNetworkRecord = networks.find((network) => network.name === hostsNetwork);
  const hosts = staticHosts(hostsNetworkRecord);
  const dnsHosts = staticDNSHosts(hostsNetworkRecord);
  const hostsHaveDHCP = Boolean(hostsNetworkRecord?.addresses.some((address) => address.dhcpEnabled || address.dhcpStart));
  const active = networks.filter((network) => network.active).length;
  const bridges = networks.filter((network) => network.forwardMode === "bridge").length;
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
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.list });
    },
    onError: (cause) => setCreateError(actionError(cause, "创建虚拟网络失败，请稍后重试。")),
  });

  const updateMutation = useMutation({
    mutationFn: ({ name, input }: { name: string; input: ReturnType<typeof networkUpdateInput> }) => api.updateNetwork(name, input),
    onSuccess: (network) => {
      setSuccessMessage(`网络 ${network.name} 配置已更新。`);
      setEditError("");
      setActionErrorText("");
      setEditTarget(null);
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.list });
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.leases(network.name) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.diagnostics(network.name) });
    },
    onError: (cause) => setEditError(actionError(cause, "更新虚拟网络失败，请稍后重试。")),
  });

  const actionMutation = useMutation({
    mutationFn: ({ name, action }: { name: string; action: VirtualNetworkAction }) => api.networkAction(name, action),
    onSuccess: (_, variables) => {
      setSuccessMessage(`网络 ${variables.name} 已${variables.action === "start" ? "启动" : "停止"}。`);
      setActionErrorText("");
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.list });
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.leases(variables.name) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.diagnostics(variables.name) });
    },
    onError: (cause) => setActionErrorText(actionError(cause, "更新网络状态失败，请稍后重试。")),
  });

  const deleteMutation = useMutation({
    mutationFn: (name: string) => api.deleteNetwork(name),
    onSuccess: (_, name) => {
      setSuccessMessage(`网络 ${name} 已删除。`);
      setActionErrorText("");
      setDeleteTarget(null);
      setDeleteName("");
      setDeleteError("");
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.list });
      setLeasesNetwork((current) => current === name ? null : current);
      setHostsNetwork((current) => current === name ? null : current);
      setDiagnosticsNetwork((current) => current === name ? null : current);
      setPortForwardsNetwork((current) => current === name ? null : current);
      queryClient.removeQueries({ queryKey: queryKeys.networks.leases(name) });
      queryClient.removeQueries({ queryKey: queryKeys.networks.diagnostics(name) });
      queryClient.removeQueries({ queryKey: queryKeys.networks.portForwards(name) });
    },
    onError: (cause) => setDeleteError(actionError(cause, "删除虚拟网络失败，请稍后重试。")),
  });

  const addHostMutation = useMutation({
    mutationFn: ({ name, input }: { name: string; input: NetworkDHCPHostInput }) => api.addNetworkDHCPHost(name, input),
    onSuccess: (network) => {
      setSuccessMessage(`网络 ${network.name} 的静态主机条目已添加，${network.active ? "已立即生效。" : "将在网络启动后生效。"}`);
      setHostError("");
      setActionErrorText("");
      setHostForm(initialHostForm);
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.list });
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.diagnostics(network.name) });
    },
    onError: (cause) => setHostError(actionError(cause, "添加静态主机失败，请稍后重试。")),
  });

  const deleteHostMutation = useMutation({
    mutationFn: ({ name, host }: { name: string; host: NetworkDHCPHost }) => api.deleteNetworkDHCPHost(name, hostIdentity(host)),
    onSuccess: (network, variables) => {
      setSuccessMessage(`网络 ${network.name} 的静态主机 ${hostIdentity(variables.host)} 已删除。`);
      setHostError("");
      setActionErrorText("");
      setHostDeleteTarget(null);
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.list });
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.diagnostics(network.name) });
    },
    onError: (cause) => setHostError(actionError(cause, "删除静态主机失败，请稍后重试。")),
  });

  const addDNSHostMutation = useMutation({
    mutationFn: ({ name, input }: { name: string; input: NetworkDNSHostInput }) => api.addNetworkDNSHost(name, input),
    onSuccess: (network) => {
      setSuccessMessage(`网络 ${network.name} 的 DNS 记录已添加，${network.active ? "已立即生效。" : "将在网络启动后生效。"}`);
      setDNSHostError("");
      setActionErrorText("");
      setDNSHostForm(initialDNSHostForm);
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.list });
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.diagnostics(network.name) });
    },
    onError: (cause) => setDNSHostError(actionError(cause, "添加 DNS 记录失败，请稍后重试。")),
  });

  const deleteDNSHostMutation = useMutation({
    mutationFn: ({ name, host }: { name: string; host: NetworkDNSHost }) => api.deleteNetworkDNSHost(name, host.ip),
    onSuccess: (network, variables) => {
      setSuccessMessage(`网络 ${network.name} 的 DNS 记录 ${variables.host.ip} 已删除。`);
      setDNSHostError("");
      setActionErrorText("");
      setDNSHostDeleteTarget(null);
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.list });
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.diagnostics(network.name) });
    },
    onError: (cause) => setDNSHostError(actionError(cause, "删除 DNS 记录失败，请稍后重试。")),
  });

  const reserveLeaseMutation = useMutation({
    mutationFn: ({ name, ip }: { name: string; ip: string }) => api.reserveNetworkLease(name, { ip }),
    onSuccess: (network, variables) => {
      setSuccessMessage(`租约 ${variables.ip} 已保留为网络 ${network.name} 的静态主机。`);
      setActionErrorText("");
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.list });
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.leases(network.name) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.diagnostics(network.name) });
    },
    onError: (cause) => setActionErrorText(actionError(cause, "保留租约地址失败，请稍后重试。")),
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

  function updateDNSHostForm<K extends keyof DNSHostFormState>(key: K, value: DNSHostFormState[K]) {
    setDNSHostForm((form) => ({ ...form, [key]: value }));
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
      void queryClient.invalidateQueries({ queryKey: queryKeys.networks.list });
    } catch (cause) {
      setHostError(actionError(cause, "导入静态主机失败，请检查 CSV 格式后重试。"));
    } finally {
      setHostTransfer("");
    }
  }

  function submitCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canManage) return;
    setCreateError("");
    setSuccessMessage("");
    setActionErrorText("");
    if (createForm.mode !== "bridge" && !createForm.subnet.trim() && !createForm.ipv6Subnet.trim()) {
      setCreateError("IPv4 与 IPv6 子网至少需要填写一项。");
      return;
    }
    const mtuError = mtuInputError(createForm.mtu, Boolean(createForm.ipv6Subnet.trim()));
    if (mtuError) {
      setCreateError(mtuError);
      return;
    }
    createMutation.mutate(createNetworkInput(createForm));
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
    const mtuError = mtuInputError(editForm.mtu, Boolean(editForm.ipv6Subnet.trim()));
    if (mtuError) {
      setEditError(mtuError);
      return;
    }
    updateMutation.mutate({ name: editTarget.name, input: networkUpdateInput(editForm) });
  }

  function runAction(network: VirtualNetwork, action: VirtualNetworkAction) {
    if (!canManage) return;
    setSuccessMessage("");
    setActionErrorText("");
    actionMutation.mutate({ name: network.name, action });
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
    setDNSHostError("");
    setDNSHostForm(initialDNSHostForm);
    setSuccessMessage("");
    setActionErrorText("");
  }

  function submitHost(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!hostsNetwork || !canManage || !hostsHaveDHCP) return;
    setHostError("");
    setSuccessMessage("");
    const ipv6 = hostIsIPv6(hostForm);
    if (ipv6 && !hostForm.id.trim() && !hostForm.name.trim()) {
      setHostError("DHCPv6 静态主机需要填写 DUID/Client ID 或主机名。");
      return;
    }
    addHostMutation.mutate({
      name: hostsNetwork,
      input: {
        mac: ipv6 ? undefined : hostForm.mac.trim(),
        id: ipv6 ? hostForm.id.trim() || undefined : undefined,
        ip: hostForm.ip.trim(),
        name: hostForm.name.trim() || undefined,
      },
    });
  }

  function submitDNSHost(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!hostsNetwork || !canManage) return;
    setDNSHostError("");
    setSuccessMessage("");
    const hostnames = splitDNSHostnames(dnsHostForm.hostnames);
    if (!dnsHostForm.ip.trim()) {
      setDNSHostError("请填写 IPv4 或 IPv6 地址。");
      return;
    }
    if (!hostnames.length) {
      setDNSHostError("请至少填写一个 DNS 主机名。");
      return;
    }
    addDNSHostMutation.mutate({ name: hostsNetwork, input: { ip: dnsHostForm.ip.trim(), hostnames } });
  }

  return (
    <>
      <PageHeader
        title="虚拟网络"
        breadcrumb="总览 / 网络"
        description="管理 libvirt 虚拟网络、NAT、桥接、DHCP 范围和自动启动策略。"
        actions={(
          <Button data-testid="network-create-toggle" disabled={!canManage || createMutation.isPending} title={disabledReason || undefined} onClick={() => { setCreateOpen((open) => !open); setCreateError(""); }}>
            <Plus className="h-4 w-4" />新建网络
          </Button>
        )}
      />

      {!canManage ? (
        <div className="mb-4 flex items-start gap-2 rounded-lg border border-warning/35 bg-warning/10 px-4 py-3 text-sm text-warning" role="status" data-testid="network-manage-disabled-notice">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <span>{readOnly ? "只读模式已开启，网络写操作已禁用，仍可查看 DHCP 租约和诊断信息。" : "当前账号没有管理网络的权限，仍可查看 DHCP 租约和诊断信息。"}</span>
        </div>
      ) : null}

      <datalist id="network-bridge-options">
        {hostBridges.map((bridge) => <option key={bridge.name} value={bridge.name} />)}
      </datalist>

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

      {createOpen ? <NetworkCreateCard form={createForm} error={createError} canManage={canManage} pending={createMutation.isPending} onChange={updateCreateForm} onSubmit={submitCreate} onCancel={() => { setCreateOpen(false); setCreateError(""); }} /> : null}
      {editTarget ? <NetworkEditCard target={editTarget} form={editForm} error={editError} pending={updateMutation.isPending} onChange={updateEditForm} onSubmit={submitEdit} onCancel={() => { setEditTarget(null); setEditError(""); }} /> : null}

      {query.isLoading ? <LoadingState label="正在读取虚拟网络" /> : null}
      {query.isError ? <ErrorState message={query.error instanceof Error ? query.error.message : "无法读取虚拟网络"} onRetry={() => void query.refetch()} /> : null}

      {query.data ? (
        <>
          <NetworkTableCard
            networks={networks}
            total={query.data.total}
            active={active}
            bridges={bridges}
            nat={nat}
            role={session.data?.user.role}
            readOnly={readOnly}
            canManage={canManage}
            editTargetName={editTarget?.name}
            leasesNetwork={leasesNetwork}
            hostsNetwork={hostsNetwork}
            actionPendingName={actionMutation.isPending ? actionMutation.variables?.name : undefined}
            actionPendingAction={actionMutation.isPending ? actionMutation.variables?.action : undefined}
            updatePending={updateMutation.isPending}
            deletePending={deleteMutation.isPending}
            onEdit={openEdit}
            onAction={runAction}
            onDelete={openDelete}
            onLeases={(network) => setLeasesNetwork((current) => current === network.name ? null : network.name)}
            onHosts={toggleHosts}
            diagnosticsNetwork={diagnosticsNetwork}
            portForwardsNetwork={portForwardsNetwork}
            onDiagnostics={(network) => { setDiagnosticsNetwork((current) => current === network.name ? null : network.name); setHostError(""); }}
            onPortForwards={(network) => setPortForwardsNetwork((current) => current === network.name ? null : network.name)}
          />

          {leasesNetwork ? <NetworkLeasesCard network={leasesNetwork} record={leasesNetworkRecord} leases={leases} loaded={Boolean(leasesQuery.data)} loading={leasesQuery.isLoading} error={leasesQuery.error} fetching={leasesQuery.isFetching} canManage={canManage} disabledReason={disabledReason} reservingIP={reserveLeaseMutation.isPending ? reserveLeaseMutation.variables?.ip : undefined} onRefresh={() => void leasesQuery.refetch()} onReserve={(lease) => { setSuccessMessage(""); setActionErrorText(""); reserveLeaseMutation.mutate({ name: leasesNetwork, ip: lease.ipAddress }); }} /> : null}
          {diagnosticsNetwork ? <NetworkDiagnosticsCard network={diagnosticsNetwork} diagnostics={diagnosticsQuery.data} loading={diagnosticsQuery.isLoading} error={diagnosticsQuery.error} fetching={diagnosticsQuery.isFetching} onRefresh={() => void diagnosticsQuery.refetch()} /> : null}
          {portForwardsNetwork ? <NetworkPortForwardsCard network={networks.find((network) => network.name === portForwardsNetwork) ?? { name: portForwardsNetwork, active: false, persistent: false, autostart: false, addresses: [] }} canManage={canManage} disabledReason={disabledReason} onClose={() => setPortForwardsNetwork(null)} /> : null}
          {hostsNetwork ? (
            <>
              <NetworkHostsCard
                network={hostsNetwork}
                record={hostsNetworkRecord}
                hosts={hosts}
                hasDHCP={hostsHaveDHCP}
                canManage={canManage}
                disabledReason={disabledReason}
                form={hostForm}
                error={hostError}
                transfer={hostTransfer}
                addPending={addHostMutation.isPending}
                deletePending={deleteHostMutation.isPending}
                onFormChange={updateHostForm}
                onSubmit={submitHost}
                onExport={(network) => void exportHosts(network)}
                onImport={importHosts}
                onDelete={(host) => { if (hostsNetworkRecord) { setHostError(""); setHostDeleteTarget({ network: hostsNetworkRecord, host }); } }}
              />
              <NetworkDNSHostsCard
                network={hostsNetwork}
                record={hostsNetworkRecord}
                hosts={dnsHosts}
                canManage={canManage}
                disabledReason={disabledReason}
                form={dnsHostForm}
                error={dnsHostError}
                addPending={addDNSHostMutation.isPending}
                deletePending={deleteDNSHostMutation.isPending}
                onFormChange={updateDNSHostForm}
                onSubmit={submitDNSHost}
                onDelete={(host) => { if (hostsNetworkRecord) { setDNSHostError(""); setDNSHostDeleteTarget({ network: hostsNetworkRecord, host }); } }}
              />
            </>
          ) : null}
        </>
      ) : null}

      <NetworkHostDeleteDialog
        target={hostDeleteTarget}
        error={hostError}
        pending={deleteHostMutation.isPending}
        onOpenChange={(open) => { if (!open) { setHostDeleteTarget(null); setHostError(""); } }}
        onConfirm={() => {
          if (!hostDeleteTarget) return;
          setHostError("");
          deleteHostMutation.mutate({ name: hostDeleteTarget.network.name, host: hostDeleteTarget.host });
        }}
      />

      <NetworkDNSHostDeleteDialog
        target={dnsHostDeleteTarget}
        error={dnsHostError}
        pending={deleteDNSHostMutation.isPending}
        onOpenChange={(open) => { if (!open) { setDNSHostDeleteTarget(null); setDNSHostError(""); } }}
        onConfirm={() => {
          if (!dnsHostDeleteTarget) return;
          setDNSHostError("");
          deleteDNSHostMutation.mutate({ name: dnsHostDeleteTarget.network.name, host: dnsHostDeleteTarget.host });
        }}
      />

      <NetworkDeleteDialog
        target={deleteTarget}
        name={deleteName}
        error={deleteError}
        pending={deleteMutation.isPending}
        onNameChange={setDeleteName}
        onOpenChange={(open) => { if (!open) { setDeleteTarget(null); setDeleteError(""); } }}
        onConfirm={() => {
          if (!deleteTarget) return;
          setDeleteError("");
          deleteMutation.mutate(deleteTarget.name);
        }}
      />
    </>
  );
}

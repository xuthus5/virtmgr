import { CircuitBoard, Cpu, Database, HardDrive, MemoryStick, Network as NetworkIcon, RefreshCw, Server, TriangleAlert } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { DetailItem } from "@/components/data/detail-item";
import { MetricCard } from "@/components/data/metric-card";
import { ResourceBar } from "@/components/data/resource-bar";
import { StatusBadge } from "@/components/feedback/status-badge";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { PageHeader } from "@/components/layout/page-header";
import { useHostDevicesQuery } from "@/features/host/use-host-devices-query";
import { useHostMetricsQuery, useHostQuery } from "@/features/host/use-host-query";
import { ResourceTrendCard } from "@/features/metrics/resource-trend-card";
import { formatBytes, percent } from "@/lib/utils";

const deviceTypeLabels: Record<string, string> = {
  pci: "PCI",
  usb: "USB 控制器",
  usb_device: "USB 设备",
  net: "网络",
  storage: "块设备",
  drm: "显示设备",
};

export function HostPage() {
  const hostQuery = useHostQuery();
  const metricsQuery = useHostMetricsQuery();
  const devicesQuery = useHostDevicesQuery();

  if (hostQuery.isLoading) return <LoadingState label="正在读取物理主机信息" />;
  if (hostQuery.isError || !hostQuery.data) {
    return <ErrorState message={hostQuery.error instanceof Error ? hostQuery.error.message : "无法读取物理主机信息"} onRetry={() => void hostQuery.refetch()} />;
  }
  const data = hostQuery.data;
  const poolCapacity = data.storagePools.reduce((sum, pool) => sum + pool.capacityBytes, 0);
  const poolAllocation = data.storagePools.reduce((sum, pool) => sum + pool.allocationBytes, 0);
  const activeNetworks = data.networks.filter((network) => network.active).length;

  return (
    <>
      <PageHeader
        title="物理主机"
        breadcrumb="总览 / 物理主机"
        description={<span className="flex flex-wrap items-center gap-2"><span className="h-2 w-2 rounded-full bg-success" />{data.host.name} · libvirt {data.host.libvirtVersion}</span>}
        actions={<Button variant="outline" onClick={() => void Promise.all([hostQuery.refetch(), devicesQuery.refetch()])} disabled={hostQuery.isFetching || devicesQuery.isFetching}><RefreshCw className={hostQuery.isFetching || devicesQuery.isFetching ? "h-4 w-4 animate-spin" : "h-4 w-4"} />刷新</Button>}
      />

      {data.warnings?.length ? (
        <div className="mb-4 flex items-start gap-2 rounded-lg border border-warning/35 bg-warning/10 px-4 py-3 text-sm text-warning" role="status" data-testid="host-warnings">
          <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0" />
          <span>{data.warnings.join("；")}</span>
        </div>
      ) : null}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard label="CPU 线程" value={data.host.cpus} hint={`${data.host.cpuSockets} 插槽 · ${data.host.cpuCores} 核`} icon={Cpu} tone="primary" testId="host-cpu" />
        <MetricCard label="物理内存" value={formatBytes(data.host.memoryBytes)} hint={`${data.host.cpuMhz} MHz`} icon={MemoryStick} tone="success" testId="host-memory" />
        <MetricCard label="存储池" value={data.storagePools.length} hint={`${formatBytes(poolCapacity)} 总容量`} icon={Database} tone="neutral" testId="host-pools" />
        <MetricCard label="虚拟网络" value={data.networks.length} hint={`${activeNetworks} 个活跃`} icon={NetworkIcon} tone="neutral" testId="host-networks" />
      </div>

      <div className="mt-4">
        <ResourceTrendCard
          title="实时资源趋势"
          description="宿主机 CPU、内存和物理网卡的近期采样，用于识别突发负载和容量压力。"
          metrics={metricsQuery.data}
          isLoading={metricsQuery.isLoading}
          error={metricsQuery.isError ? (metricsQuery.error instanceof Error ? metricsQuery.error.message : "无法读取宿主机资源指标") : undefined}
          testId="host-metrics"
          historyKey="host"
        />
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-[1.1fr_0.9fr]">
        <Card>
          <CardHeader><CardTitle>资源分配</CardTitle><CardDescription>虚拟机配置量相对宿主机物理资源。</CardDescription></CardHeader>
          <CardContent className="space-y-6">
            <div className="rounded-lg border bg-muted/25 p-4">
              <div className="mb-4 flex items-center gap-2 text-sm font-medium"><Cpu className="h-4 w-4 text-primary" />vCPU</div>
              <ResourceBar label="vCPU 分配率" used={data.allocatedVcpus} total={data.host.cpus} displayUsed={`${data.allocatedVcpus} vCPU`} displayTotal={`${data.host.cpus} 线程`} />
            </div>
            <div className="rounded-lg border bg-muted/25 p-4">
              <div className="mb-4 flex items-center gap-2 text-sm font-medium"><MemoryStick className="h-4 w-4 text-primary" />内存</div>
              <ResourceBar label="内存分配率" used={data.allocatedMemoryBytes} total={data.host.memoryBytes} displayUsed={formatBytes(data.allocatedMemoryBytes)} displayTotal={formatBytes(data.host.memoryBytes)} />
            </div>
            <div className="rounded-lg border bg-muted/25 p-4">
              <div className="mb-4 flex items-center gap-2 text-sm font-medium"><HardDrive className="h-4 w-4 text-primary" />存储</div>
              <ResourceBar label="存储分配率" used={poolAllocation} total={poolCapacity} displayUsed={formatBytes(poolAllocation)} displayTotal={formatBytes(poolCapacity)} />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>主机信息</CardTitle><CardDescription>libvirt 节点信息与运行状态。</CardDescription></CardHeader>
          <CardContent>
            <DetailItem label="主机名">{data.host.name}</DetailItem>
            <DetailItem label="libvirt 版本">{data.host.libvirtVersion}</DetailItem>
            <DetailItem label="CPU 型号">{data.host.cpuModel}</DetailItem>
            <DetailItem label="拓扑">{data.host.cpuSockets} 插槽 · {data.host.cpuCores} 核 · {data.host.cpuThreads} 线程</DetailItem>
            <DetailItem label="CPU 频率">{data.host.cpuMhz} MHz</DetailItem>
            <DetailItem label="物理内存">{formatBytes(data.host.memoryBytes)}</DetailItem>
            <DetailItem label="虚拟机">{data.counts.total} 台 · {data.counts.running} 运行 · {data.counts.paused} 暂停 · {data.counts.shutoff} 关机</DetailItem>
          </CardContent>
        </Card>
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-2">
        <Card>
          <CardHeader><CardTitle>存储池</CardTitle><CardDescription>池状态、目标路径和容量使用情况。</CardDescription></CardHeader>
          <CardContent className="p-0">
            {data.storagePools.length ? <Table>
              <TableHeader><TableRow><TableHead>名称</TableHead><TableHead>状态</TableHead><TableHead>类型</TableHead><TableHead>容量</TableHead><TableHead>使用率</TableHead></TableRow></TableHeader>
              <TableBody>{data.storagePools.map((pool) => (
                <TableRow key={pool.name} data-testid={`host-pool-row-${pool.name}`}>
                  <TableCell><p className="font-medium">{pool.name}</p><p className="max-w-52 truncate font-mono text-[11px] text-muted-foreground">{pool.targetPath || "-"}</p></TableCell>
                  <TableCell><Badge variant={pool.state === "active" ? "success" : pool.state === "inactive" ? "secondary" : "warning"}>{pool.state}</Badge></TableCell>
                  <TableCell className="font-mono text-xs">{pool.type}</TableCell>
                  <TableCell className="text-xs">{formatBytes(pool.capacityBytes)}</TableCell>
                  <TableCell className="text-xs">{percent(pool.allocationBytes, pool.capacityBytes)}%</TableCell>
                </TableRow>
              ))}</TableBody>
            </Table> : <p className="p-5 text-sm text-muted-foreground">没有存储池。</p>}
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>虚拟网络</CardTitle><CardDescription>网络模式、桥接口和激活状态。</CardDescription></CardHeader>
          <CardContent className="p-0">
            {data.networks.length ? <Table>
              <TableHeader><TableRow><TableHead>名称</TableHead><TableHead>状态</TableHead><TableHead>模式</TableHead><TableHead>桥接口</TableHead><TableHead>地址</TableHead></TableRow></TableHeader>
              <TableBody>{data.networks.map((network) => (
                <TableRow key={network.name} data-testid={`host-network-row-${network.name}`}>
                  <TableCell className="font-medium">{network.name}</TableCell>
                  <TableCell><Badge variant={network.active ? "success" : "secondary"}>{network.active ? "活跃" : "未激活"}</Badge></TableCell>
                  <TableCell className="font-mono text-xs">{network.forwardMode || "isolated"}</TableCell>
                  <TableCell className="font-mono text-xs">{network.bridge || "-"}</TableCell>
                  <TableCell className="max-w-52 truncate font-mono text-xs text-muted-foreground">{network.addresses.map((address) => `${address.address}/${address.prefix}`).join(", ") || "-"}</TableCell>
                </TableRow>
              ))}</TableBody>
            </Table> : <p className="p-5 text-sm text-muted-foreground">没有虚拟网络。</p>}
          </CardContent>
        </Card>
      </div>

      <Card className="mt-4">
        <CardHeader><CardTitle>虚拟机资源分配</CardTitle><CardDescription>按虚拟机展示当前配置的 vCPU 与内存。</CardDescription></CardHeader>
        <CardContent className="p-0">
          {data.vms.length ? <Table>
            <TableHeader><TableRow><TableHead>名称</TableHead><TableHead>状态</TableHead><TableHead>UUID</TableHead><TableHead>vCPU</TableHead><TableHead>内存</TableHead></TableRow></TableHeader>
            <TableBody>{data.vms.map((vm) => (
              <TableRow key={vm.uuid} data-testid={`host-vm-row-${vm.name}`}>
                <TableCell className="font-medium"><span className="inline-flex items-center gap-2"><Server className="h-3.5 w-3.5 text-muted-foreground" />{vm.name}</span></TableCell>
                <TableCell><StatusBadge state={vm.state} /></TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">{vm.uuid}</TableCell>
                <TableCell>{vm.vcpus}</TableCell>
                <TableCell>{formatBytes(vm.memoryBytes)}</TableCell>
              </TableRow>
            ))}</TableBody>
          </Table> : <p className="p-5 text-sm text-muted-foreground">宿主机上没有虚拟机。</p>}
        </CardContent>
      </Card>

      <Card className="mt-4">
        <CardHeader className="flex-row items-start justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2"><CircuitBoard className="h-4 w-4 text-primary" />主机设备</CardTitle>
            <CardDescription>libvirt 节点设备视图，覆盖 PCI、USB、网络和块设备。</CardDescription>
          </div>
          <Badge variant="outline" data-testid="host-device-total">{devicesQuery.data?.total ?? "-"}</Badge>
        </CardHeader>
        <CardContent className="p-0">
          {devicesQuery.isLoading ? (
            <p className="p-5 text-sm text-muted-foreground" data-testid="host-devices-loading">正在读取主机设备。</p>
          ) : devicesQuery.isError ? (
            <div className="flex flex-col items-start gap-3 p-5 text-sm text-destructive" data-testid="host-devices-error">
              <span>{devicesQuery.error instanceof Error ? devicesQuery.error.message : "无法读取主机设备"}</span>
              <Button variant="outline" size="sm" onClick={() => void devicesQuery.refetch()}>重试</Button>
            </div>
          ) : devicesQuery.data?.items.length ? <Table>
            <TableHeader><TableRow><TableHead>类型</TableHead><TableHead>设备</TableHead><TableHead>地址</TableHead><TableHead>厂商 / 型号</TableHead><TableHead>驱动</TableHead><TableHead>状态</TableHead></TableRow></TableHeader>
            <TableBody>{devicesQuery.data.items.map((device) => (
              <TableRow key={device.name} data-testid={`host-device-row-${device.name}`}>
                <TableCell><Badge variant="outline">{deviceTypeLabels[device.type] ?? device.type}</Badge><p className="mt-1 font-mono text-[10px] text-muted-foreground">{device.capabilities.join(", ")}</p></TableCell>
                <TableCell><p className="font-medium">{device.name}</p><p className="max-w-60 truncate font-mono text-[11px] text-muted-foreground">{device.parent || "无父设备"}</p></TableCell>
                <TableCell className="font-mono text-xs">{device.address || "-"}</TableCell>
                <TableCell><p>{device.vendor || device.vendorId || "-"}</p><p className="max-w-60 truncate text-xs text-muted-foreground">{device.product || device.productId || "-"}</p></TableCell>
                <TableCell className="font-mono text-xs">{device.driver || "-"}</TableCell>
                <TableCell><Badge variant={device.active ? "success" : "secondary"}>{device.active ? "在线" : "离线"}</Badge></TableCell>
              </TableRow>
            ))}</TableBody>
          </Table> : <p className="p-5 text-sm text-muted-foreground" data-testid="host-devices-empty">宿主机未报告节点设备。</p>}
        </CardContent>
      </Card>
    </>
  );
}

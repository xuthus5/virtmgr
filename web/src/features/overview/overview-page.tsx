import { Link } from "@tanstack/react-router";
import { CircleDot, Cpu, MemoryStick, Pause, Power, RefreshCw, Server, TriangleAlert } from "lucide-react";
import { MetricCard } from "@/components/data/metric-card";
import { ResourceBar } from "@/components/data/resource-bar";
import { DetailItem } from "@/components/data/detail-item";
import { StatusBadge } from "@/components/feedback/status-badge";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useOverviewQuery } from "@/features/overview/use-overview-query";
import { formatBytes } from "@/lib/utils";

export function OverviewPage() {
  const overview = useOverviewQuery();

  if (overview.isLoading) return <LoadingState label="正在读取 libvirt 总览" className="min-h-[60vh]" />;
  if (overview.isError) {
    return <ErrorState message={overview.error instanceof Error ? overview.error.message : "无法读取宿主机数据"} onRetry={() => void overview.refetch()} className="mt-10" />;
  }
  const data = overview.data;
  if (!data) return null;

  return (
    <>
      <PageHeader
        title="总览"
        breadcrumb="总览 / 物理机"
        description={<span className="flex flex-wrap items-center gap-2"><span className="h-2 w-2 rounded-full bg-success" />{data.host.name} · libvirt {data.host.libvirtVersion}</span>}
        actions={<Button variant="outline" onClick={() => void overview.refetch()}><RefreshCw className="h-4 w-4" />刷新</Button>}
      />

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard label="虚拟机总数" value={data.counts.total} hint="libvirt 中已定义的全部域" icon={Server} tone="primary" testId="metric-total-value" />
        <MetricCard label="运行中" value={data.counts.running} hint="Running / Blocked" icon={CircleDot} tone="success" testId="metric-running-value" />
        <MetricCard label="已暂停" value={data.counts.paused} hint="Paused / Suspended" icon={Pause} tone={data.counts.paused ? "warning" : "neutral"} testId="metric-paused-value" />
        <MetricCard label="已关机" value={data.counts.shutoff + data.counts.failed} hint={`${data.counts.failed} 台异常`} icon={Power} tone={data.counts.failed ? "danger" : "neutral"} testId="metric-stopped-value" />
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-[1.15fr_0.85fr]">
        <Card>
          <CardHeader>
            <CardTitle>资源分配</CardTitle>
            <CardDescription>虚拟机当前分配量与宿主机物理容量的比例。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="rounded-lg border bg-muted/25 p-4">
              <div className="mb-4 flex items-center gap-2 text-sm font-medium"><Cpu className="h-4 w-4 text-primary" />vCPU</div>
              <ResourceBar label="vCPU 分配率" used={data.allocatedVcpus} total={data.host.cpus} displayUsed={`${data.allocatedVcpus} vCPU`} displayTotal={`${data.host.cpus} 物理线程`} />
            </div>
            <div className="rounded-lg border bg-muted/25 p-4">
              <div className="mb-4 flex items-center gap-2 text-sm font-medium"><MemoryStick className="h-4 w-4 text-primary" />内存</div>
              <ResourceBar label="内存分配率" used={data.allocatedMemoryBytes} total={data.host.memoryBytes} displayUsed={formatBytes(data.allocatedMemoryBytes)} displayTotal={formatBytes(data.host.memoryBytes)} />
            </div>
            {data.warnings?.length ? <p className="rounded-md border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning"><TriangleAlert className="mr-1 inline h-3.5 w-3.5" />{data.warnings.join("，")}</p> : null}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>宿主机</CardTitle>
            <CardDescription>libvirt 报告的物理资源与版本信息。</CardDescription>
          </CardHeader>
          <CardContent>
            <DetailItem label="主机名">{data.host.name}</DetailItem>
            <DetailItem label="libvirt">{data.host.libvirtVersion}</DetailItem>
            <DetailItem label="CPU 型号">{data.host.cpuModel}</DetailItem>
            <DetailItem label="CPU">{data.host.cpus} 线程 · {data.host.cpuCores} 核</DetailItem>
            <DetailItem label="CPU 频率">{data.host.cpuMhz} MHz</DetailItem>
            <DetailItem label="内存">{formatBytes(data.host.memoryBytes)}</DetailItem>
          </CardContent>
        </Card>
      </div>

      <Card className="mt-4">
        <CardHeader>
          <div className="flex items-center justify-between gap-3">
            <div><CardTitle>虚拟机状态</CardTitle><CardDescription>按名称排序，最多展示 8 台。</CardDescription></div>
            <Button variant="ghost" size="sm" asChild><Link to="/vms">查看全部</Link></Button>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow><TableHead>名称</TableHead><TableHead>状态</TableHead><TableHead>vCPU</TableHead><TableHead>内存</TableHead><TableHead>磁盘</TableHead><TableHead>网络</TableHead></TableRow>
            </TableHeader>
            <TableBody>
              {data.vms.map((vm) => (
                <TableRow key={vm.uuid}>
                  <TableCell>
                    <Link to="/vms/$uuid" params={{ uuid: vm.uuid }} className="font-medium text-primary hover:underline">{vm.name}</Link>
                    <p className="font-mono text-[10px] text-muted-foreground">{vm.uuid.slice(0, 8)}</p>
                  </TableCell>
                  <TableCell><StatusBadge state={vm.state} /></TableCell>
                  <TableCell className="tabular-nums">{vm.vcpus}</TableCell>
                  <TableCell className="tabular-nums">{formatBytes(vm.memoryBytes)}</TableCell>
                  <TableCell className="text-muted-foreground">{vm.disks.length} 块</TableCell>
                  <TableCell className="text-muted-foreground">{vm.interfaces[0]?.source ?? "-"}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </>
  );
}

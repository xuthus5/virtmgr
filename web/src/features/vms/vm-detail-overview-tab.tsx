import { Cpu, HardDrive, MemoryStick, Network } from "lucide-react";
import type { ResourceMetrics, VM } from "@/api/types";
import { MetricCard } from "@/components/data/metric-card";
import { VMConsoleCard } from "@/features/console/vm-console-card";
import { MetricHistoryCard } from "@/features/metrics/metric-history-card";
import { ResourceTrendCard } from "@/features/metrics/resource-trend-card";
import { formatBytes } from "@/lib/utils";

export function VMDetailOverviewTab({ vm, uuid, canMutate, consoleReason, metrics, metricsLoading, metricsError }: {
  vm: VM;
  uuid: string;
  canMutate: boolean;
  consoleReason: string;
  metrics: ResourceMetrics | undefined;
  metricsLoading: boolean;
  metricsError?: string;
}) {
  return <div role="tabpanel" id="vm-tabpanel-overview" aria-labelledby="vm-tab-overview" data-testid="vm-tabpanel-overview">
    <VMConsoleCard uuid={uuid} graphics={vm.graphics} serials={vm.serials} state={vm.state} canConnect={canMutate} disabledReason={consoleReason} />

    <div className="mt-4 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <MetricCard label="vCPU" value={vm.vcpus} hint={`上限 ${vm.maxVcpus || vm.vcpus} vCPU`} icon={Cpu} tone="primary" testId="vm-detail-vcpus" />
      <MetricCard label="内存" value={formatBytes(vm.memoryBytes)} hint={`最大 ${formatBytes(vm.maxMemoryBytes)}`} icon={MemoryStick} tone="success" testId="vm-detail-memory" />
      <MetricCard label="磁盘" value={vm.disks.length} hint="已连接块设备" icon={HardDrive} tone="neutral" testId="vm-detail-disks" />
      <MetricCard label="网络接口" value={vm.interfaces.length} hint="虚拟网卡" icon={Network} tone="neutral" testId="vm-detail-interfaces" />
    </div>

    <div className="mt-4">
      <ResourceTrendCard title="资源趋势" description="当前虚拟机的 CPU、内存和虚拟网卡采样；运行中或阻塞状态会持续更新。" metrics={metrics} isLoading={metricsLoading} error={metricsError} testId="vm-metrics" historyKey={uuid} showDiskIO />
    </div>

    <div className="mt-4">
      <MetricHistoryCard scope="vm" targetId={uuid} title="虚拟机历史趋势" description="虚拟机运行期间的 CPU、内存、磁盘和网络分钟级采样。" testId="vm-metric-history" />
    </div>
  </div>;
}

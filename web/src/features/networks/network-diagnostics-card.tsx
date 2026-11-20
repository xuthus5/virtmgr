import { AlertTriangle, CheckCircle2, Info, RefreshCw, ShieldCheck } from "lucide-react";
import type { NetworkCheckStatus, NetworkDiagnostics } from "@/api/types";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

const statusMeta: Record<NetworkCheckStatus, { label: string; variant: "success" | "warning" | "destructive"; icon: typeof CheckCircle2 }> = {
  ok: { label: "正常", variant: "success", icon: CheckCircle2 },
  warning: { label: "警告", variant: "warning", icon: Info },
  error: { label: "异常", variant: "destructive", icon: AlertTriangle },
};

export function NetworkDiagnosticsCard({ network, diagnostics, loading, error, fetching, onRefresh }: {
  network: string;
  diagnostics?: NetworkDiagnostics;
  loading: boolean;
  error: Error | null;
  fetching: boolean;
  onRefresh: () => void;
}) {
  return <Card className="mt-4" data-testid="network-diagnostics-card">
    <CardHeader>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <CardTitle className="flex items-center gap-2"><ShieldCheck className="h-4 w-4" />IPv6 / 转发诊断 · {network}</CardTitle>
          <CardDescription>真实检查网络状态、桥接口、转发、NAT66 防火墙、DHCPv6/RA/DNS 服务、网关与出网连通性，并比较持久配置和运行态。诊断不会修改网络。</CardDescription>
        </div>
        <Button variant="outline" size="sm" data-testid="network-diagnostics-refresh" onClick={onRefresh} disabled={fetching}>
          <RefreshCw className={fetching ? "h-4 w-4 animate-spin" : "h-4 w-4"} />重新检查
        </Button>
      </div>
    </CardHeader>
    <CardContent className="space-y-4">
      {loading ? <LoadingState label="正在检查网络状态" /> : null}
      {error ? <ErrorState message={error.message || "无法读取网络诊断"} onRetry={onRefresh} /> : null}
      {diagnostics ? (
        <>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4" data-testid="network-diagnostics-summary">
            <div className="rounded-md border bg-muted/20 p-3"><p className="text-[11px] text-muted-foreground">网络状态</p><p className="mt-1 text-sm font-medium">{diagnostics.active ? "活跃" : "未激活"}</p></div>
            <div className="rounded-md border bg-muted/20 p-3"><p className="text-[11px] text-muted-foreground">桥接口</p><p className="mt-1 font-mono text-sm">{diagnostics.bridge || "-"}{diagnostics.bridgeReady ? " · 就绪" : " · 未就绪"}</p></div>
            <div className="rounded-md border bg-muted/20 p-3"><p className="text-[11px] text-muted-foreground">宿主机转发</p><p className="mt-1 text-sm">IPv4 {diagnostics.ipv4Forwarding ? "已开启" : "未开启"} · IPv6 {diagnostics.ipv6Forwarding ? "已开启" : "未开启"}</p></div>
            <div className="rounded-md border bg-muted/20 p-3"><p className="text-[11px] text-muted-foreground">NAT66 / 防火墙</p><p className="mt-1 text-sm">{diagnostics.natIpv6 ? "NAT66 已启用" : "NAT66 未启用"} · {diagnostics.firewallBackend || "防火墙未检测"} · {diagnostics.nat66Firewall ? "规则就绪" : "规则未确认"}</p></div>
            <div className="rounded-md border bg-muted/20 p-3"><p className="text-[11px] text-muted-foreground">DHCPv6 / RA / DNS</p><p className="mt-1 text-sm">{diagnostics.ipv6Configured ? `DHCPv6 ${diagnostics.dhcpv6Service ? "运行" : "未确认"} · RA ${diagnostics.raService ? "运行" : "未确认"}` : "未配置 IPv6"} · DNS {diagnostics.dnsResolution ? "有响应" : "未确认"}</p></div>
            <div className="rounded-md border bg-muted/20 p-3"><p className="text-[11px] text-muted-foreground">连通性</p><p className="mt-1 text-sm">{diagnostics.ipv6Configured ? `IPv6 网关 ${diagnostics.gatewayReachable ? "可达" : "未确认"} · 宿主机 IPv6 出网 ${diagnostics.outboundReachable ? "可达" : "未确认"}` : "未配置 IPv6"}</p></div>
            <div className="rounded-md border bg-muted/20 p-3"><p className="text-[11px] text-muted-foreground">运行态配置</p><p className="mt-1 text-sm">{diagnostics.runtimeDrift ? "存在漂移" : "一致"} · 租约 {diagnostics.leaseCount} 条</p></div>
          </div>
          <div className="divide-y rounded-md border" data-testid="network-diagnostics-checks">
            {diagnostics.checks.map((check) => {
              const meta = statusMeta[check.status] ?? statusMeta.warning;
              const Icon = meta.icon;
              return <div key={check.id} className="flex items-start gap-3 p-3" data-testid={`network-diagnostic-${check.id}`}>
                <Icon className="mt-0.5 h-4 w-4 shrink-0" />
                <div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><p className="text-sm font-medium">{check.title}</p><Badge variant={meta.variant}>{meta.label}</Badge></div><p className="mt-1 text-xs leading-relaxed text-muted-foreground">{check.detail}</p></div>
              </div>;
            })}
          </div>
          <p className="text-[11px] text-muted-foreground">检查时间：{new Date(diagnostics.checkedAt).toLocaleString("zh-CN")}{diagnostics.libvirtVersion ? ` · libvirt ${diagnostics.libvirtVersion}` : ""}{diagnostics.dnsmasqVersion ? ` · ${diagnostics.dnsmasqVersion}` : ""}</p>
        </>
      ) : null}
    </CardContent>
  </Card>;
}

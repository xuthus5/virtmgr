import { RefreshCw, ScrollText } from "lucide-react";
import type { DHCPLease, VirtualNetwork } from "@/api/types";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { leaseExpiry } from "@/features/networks/network-ui";

export function NetworkLeasesCard({ network, record, leases, loaded, loading, error, fetching, onRefresh }: {
  network: string;
  record?: VirtualNetwork;
  leases: DHCPLease[];
  loaded: boolean;
  loading: boolean;
  error: Error | null;
  fetching: boolean;
  onRefresh: () => void;
}) {
  return <Card className="mt-4" data-testid="network-leases-card">
    <CardHeader>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <CardTitle>DHCP 租约 · {network}</CardTitle>
          <CardDescription>来自 libvirt 网络 DHCP lease。未激活网络不会返回租约。</CardDescription>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant={record?.active ? "success" : "secondary"}>{record?.active ? "活跃" : "未激活"}</Badge>
          <Button variant="outline" size="sm" onClick={onRefresh} disabled={fetching}>
            <RefreshCw className={fetching ? "h-4 w-4 animate-spin" : "h-4 w-4"} />刷新
          </Button>
        </div>
      </div>
    </CardHeader>
    <CardContent className="p-0">
      {loading ? <LoadingState label="正在读取 DHCP 租约" /> : null}
      {error ? <ErrorState message={error.message || "无法读取 DHCP 租约"} onRetry={onRefresh} className="m-4" /> : null}
      {loaded && leases.length === 0 ? (
        <div className="flex flex-col items-center px-4 py-10 text-center" data-testid="network-leases-empty">
          <ScrollText className="h-8 w-8 text-muted-foreground" />
          <p className="mt-3 font-medium">暂无 DHCP 租约</p>
          <p className="mt-1 text-sm text-muted-foreground">{record?.active ? "该网络当前没有客户端取得租约。" : "网络未激活，或未配置 DHCP 地址池。"}</p>
        </div>
      ) : null}
      {leases.length > 0 ? (
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
  </Card>;
}

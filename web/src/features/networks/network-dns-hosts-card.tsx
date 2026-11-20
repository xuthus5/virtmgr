import type { FormEventHandler } from "react";
import { AlertTriangle, Globe2, Info, LoaderCircle, Plus, Trash2 } from "lucide-react";
import type { NetworkDNSHost, VirtualNetwork } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { DNSHostFormState } from "@/features/networks/network-ui";

export function NetworkDNSHostsCard({ network, record, hosts, canManage, disabledReason, form, error, addPending, deletePending, onFormChange, onSubmit, onDelete }: {
  network: string;
  record?: VirtualNetwork;
  hosts: NetworkDNSHost[];
  canManage: boolean;
  disabledReason: string;
  form: DNSHostFormState;
  error: string;
  addPending: boolean;
  deletePending: boolean;
  onFormChange: <K extends keyof DNSHostFormState>(key: K, value: DNSHostFormState[K]) => void;
  onSubmit: FormEventHandler<HTMLFormElement>;
  onDelete: (host: NetworkDNSHost) => void;
}) {
  return <Card className="mt-4" data-testid="network-dns-hosts-card">
    <CardHeader>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <CardTitle>静态 DNS A/AAAA 记录 · {network}</CardTitle>
          <CardDescription>为网络内置 DNS 添加固定主机名到 IPv4/IPv6 地址的映射；同一地址可绑定多个主机名。</CardDescription>
        </div>
        <Badge variant={record?.active ? "success" : "secondary"}>{record?.active ? "实时生效" : "待启动生效"}</Badge>
      </div>
    </CardHeader>
    <CardContent className="space-y-4">
      {!canManage ? (
        <p className="flex items-start gap-2 rounded-md border bg-muted/40 px-3 py-2 text-xs text-muted-foreground" role="status">
          <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" />{disabledReason || "当前账号只能查看 DNS 静态记录。"}
        </p>
      ) : null}
      {canManage ? (
        <form data-testid="network-dns-host-form" onSubmit={onSubmit} className="space-y-3 rounded-lg border bg-muted/20 p-3">
          <div className="grid gap-3 md:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="network-dns-host-ip">DNS 记录 IPv4 / IPv6 地址</Label>
              <Input id="network-dns-host-ip" data-testid="network-dns-host-ip" value={form.ip} onChange={(event) => onFormChange("ip", event.target.value)} placeholder="192.168.122.50 或 fd00:122::50" required autoComplete="off" disabled={addPending} className="font-mono" />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="network-dns-hostnames">DNS 主机名（逗号或空格分隔）</Label>
              <Input id="network-dns-hostnames" data-testid="network-dns-hostnames" value={form.hostnames} onChange={(event) => onFormChange("hostnames", event.target.value)} placeholder="web-01, web-01.lab.example" required autoComplete="off" disabled={addPending} className="font-mono" />
            </div>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-xs text-muted-foreground">地址必须位于该网络的 IPv4 或 IPv6 子网内；主机名不能与其他静态 DNS 记录重复。</p>
            <Button type="submit" size="sm" data-testid="network-dns-host-submit" disabled={addPending}>
              {addPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
              添加 DNS 记录
            </Button>
          </div>
        </form>
      ) : null}
      {error ? (
        <p className="flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert" data-testid="network-dns-host-error">
          <AlertTriangle className="h-3.5 w-3.5" />{error}
        </p>
      ) : null}
      <div className="overflow-x-auto rounded-md border">
        {hosts.length ? (
          <Table className="min-w-[560px]">
            <TableHeader><TableRow><TableHead>主机名</TableHead><TableHead>地址</TableHead><TableHead>类型</TableHead><TableHead className="w-20 text-right">操作</TableHead></TableRow></TableHeader>
            <TableBody>
              {hosts.map((host) => (
                <TableRow key={host.ip} data-testid={`network-dns-host-row-${host.ip}`}>
                  <TableCell><span className="block max-w-80 break-all font-mono text-xs">{host.hostnames.join(", ")}</span></TableCell>
                  <TableCell><span className="font-mono text-xs">{host.ip}</span></TableCell>
                  <TableCell><Badge variant="outline">{host.ip.includes(":") ? "AAAA" : "A"}</Badge></TableCell>
                  <TableCell className="text-right">
                    <Button type="button" variant="ghost" size="sm" className="text-destructive hover:bg-destructive/10 hover:text-destructive" data-testid={`network-dns-host-delete-${host.ip}`} aria-label={`删除 DNS 记录 ${host.ip}`} disabled={!canManage || deletePending} title={disabledReason || "删除 DNS 记录"} onClick={() => onDelete(host)}>
                      <Trash2 className="h-3.5 w-3.5" />删除
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : (
          <div className="flex flex-col items-center px-4 py-8 text-center" data-testid="network-dns-hosts-empty">
            <Globe2 className="h-7 w-7 text-muted-foreground" />
            <p className="mt-2 text-sm font-medium">暂无静态 DNS 记录</p>
            <p className="mt-1 text-xs text-muted-foreground">这些记录由 libvirt 网络内置 DNS 提供，不依赖客户机 DHCP 租约。</p>
          </div>
        )}
      </div>
    </CardContent>
  </Card>;
}

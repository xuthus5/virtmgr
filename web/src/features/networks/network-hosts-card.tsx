import { useRef, type FormEventHandler } from "react";
import { AlertTriangle, Download, Info, ListPlus, LoaderCircle, Plus, Trash2, Upload } from "lucide-react";
import type { NetworkDHCPHost, VirtualNetwork } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { hostIdentity, hostIsIPv6, type HostFormState } from "@/features/networks/network-ui";

export function NetworkHostsCard({ network, record, hosts, hasDHCP, canManage, disabledReason, form, error, transfer, addPending, deletePending, onFormChange, onSubmit, onExport, onImport, onDelete }: {
  network: string;
  record?: VirtualNetwork;
  hosts: NetworkDHCPHost[];
  hasDHCP: boolean;
  canManage: boolean;
  disabledReason: string;
  form: HostFormState;
  error: string;
  transfer: "import" | "export" | "";
  addPending: boolean;
  deletePending: boolean;
  onFormChange: <K extends keyof HostFormState>(key: K, value: HostFormState[K]) => void;
  onSubmit: FormEventHandler<HTMLFormElement>;
  onExport: (network: string) => void;
  onImport: (network: string, file: File) => Promise<void>;
  onDelete: (host: NetworkDHCPHost) => void;
}) {
  const fileInput = useRef<HTMLInputElement>(null);
  const ipv6Host = hostIsIPv6(form);

  async function importFile(networkName: string, file: File) {
    await onImport(networkName, file);
    if (fileInput.current) fileInput.current.value = "";
  }

  return <Card className="mt-4" data-testid="network-hosts-card">
    <CardHeader>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <CardTitle>静态 DHCP / DHCPv6 主机 · {network}</CardTitle>
          <CardDescription>{record?.active ? "网络正在运行：条目会立即应用到运行配置，并同步写入持久配置，无需停止网络。" : "网络未激活：条目会写入持久配置，并在网络下次启动时生效。"}</CardDescription>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={record?.active ? "success" : "secondary"}>{record?.active ? "实时生效" : "待启动生效"}</Badge>
          <Button type="button" variant="outline" size="sm" data-testid={`network-hosts-export-${network}`} disabled={transfer !== ""} onClick={() => onExport(network)}>
            {transfer === "export" ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}导出 CSV
          </Button>
          <Button type="button" variant="outline" size="sm" data-testid={`network-hosts-import-${network}`} disabled={!canManage || transfer !== "" || !hasDHCP} title={disabledReason || "导入 CSV 静态主机"} onClick={() => fileInput.current?.click()}>
            {transfer === "import" ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}导入 CSV
          </Button>
          <input ref={fileInput} type="file" accept=".csv,text/csv" className="sr-only" data-testid={`network-hosts-import-file-${network}`} onChange={(event) => { const file = event.target.files?.[0]; if (file) void importFile(network, file); }} />
        </div>
      </div>
    </CardHeader>
    <CardContent className="space-y-4">
      {!hasDHCP ? (
        <p className="flex items-start gap-2 rounded-md border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning" role="status">
          <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />该网络没有可用的 DHCP 或 DHCPv6 地址池，无法添加静态主机条目。
        </p>
      ) : null}
      {!canManage ? (
        <p className="flex items-start gap-2 rounded-md border bg-muted/40 px-3 py-2 text-xs text-muted-foreground" role="status">
          <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" />{disabledReason || "当前账号只能查看静态主机条目。"}
        </p>
      ) : null}
      {canManage && hasDHCP ? (
        <form id="network-host-form" data-testid="network-host-form" onSubmit={onSubmit} className="space-y-3 rounded-lg border bg-muted/20 p-3">
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <div className="space-y-1.5">
              <Label htmlFor="network-host-mac">MAC 地址{ipv6Host ? "（IPv6 不使用）" : ""}</Label>
              <Input id="network-host-mac" data-testid="network-host-mac" value={form.mac} onChange={(event) => onFormChange("mac", event.target.value)} placeholder="52:54:00:12:34:56" required={!ipv6Host} autoComplete="off" disabled={addPending || ipv6Host} className="font-mono" />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="network-host-id">DUID / Client ID（IPv6）</Label>
              <Input id="network-host-id" data-testid="network-host-id" value={form.id} onChange={(event) => onFormChange("id", event.target.value)} placeholder="00:03:00:01:52:54:00:12:34:56" autoComplete="off" disabled={addPending || !ipv6Host} className="font-mono" />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="network-host-ip">IPv4 / IPv6 地址</Label>
              <Input id="network-host-ip" value={form.ip} onChange={(event) => onFormChange("ip", event.target.value)} placeholder="192.168.122.50 或 fd00:122::50" required autoComplete="off" disabled={addPending} className="font-mono" />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="network-host-name">主机名（可选）</Label>
              <Input id="network-host-name" value={form.name} onChange={(event) => onFormChange("name", event.target.value)} placeholder="web-01" autoComplete="off" disabled={addPending} />
            </div>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-xs text-muted-foreground">IPv4 条目使用 MAC；DHCPv6 条目使用 DUID/Client ID 或主机名，不能填写 MAC。CSV 列为 <span className="font-mono">mac,id,ip,name</span>。</p>
            <Button type="submit" size="sm" data-testid="network-host-submit" disabled={addPending}>
              {addPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
              添加静态主机
            </Button>
          </div>
        </form>
      ) : null}
      {error ? (
        <p className="flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert" data-testid="network-host-error">
          <AlertTriangle className="h-3.5 w-3.5" />{error}
        </p>
      ) : null}
      <div className="overflow-hidden rounded-md border">
        {hosts.length ? (
          <Table className="min-w-[620px]">
            <TableHeader><TableRow><TableHead>主机名</TableHead><TableHead>IP 地址</TableHead><TableHead>DUID / Client ID</TableHead><TableHead>MAC 地址</TableHead><TableHead className="w-20 text-right">操作</TableHead></TableRow></TableHeader>
            <TableBody>
              {hosts.map((host) => {
                const identity = hostIdentity(host);
                return (
                <TableRow key={identity} data-testid={`network-host-row-${identity}`}>
                  <TableCell>{host.name || "-"}</TableCell>
                  <TableCell><span className="font-mono text-xs">{host.ip}</span></TableCell>
                  <TableCell><span className="block max-w-64 truncate font-mono text-[10px]" title={host.id}>{host.id || "-"}</span></TableCell>
                  <TableCell><span className="font-mono text-xs">{host.mac || "-"}</span></TableCell>
                  <TableCell className="text-right">
                    <Button type="button" variant="ghost" size="sm" className="text-destructive hover:bg-destructive/10 hover:text-destructive" data-testid={`network-host-delete-${identity}`} aria-label={`删除静态主机 ${identity}`} disabled={!canManage || deletePending} title={disabledReason || "删除静态主机"} onClick={() => onDelete(host)}>
                      <Trash2 className="h-3.5 w-3.5" />删除
                    </Button>
                  </TableCell>
                </TableRow>
                );
              })}
            </TableBody>
          </Table>
        ) : (
          <div className="flex flex-col items-center px-4 py-8 text-center" data-testid="network-hosts-empty">
            <ListPlus className="h-7 w-7 text-muted-foreground" />
            <p className="mt-2 text-sm font-medium">暂无静态主机条目</p>
            <p className="mt-1 text-xs text-muted-foreground">静态条目可按 MAC、DUID/Client ID 或主机名保留固定地址，与动态租约分开管理。</p>
          </div>
        )}
      </div>
    </CardContent>
  </Card>;
}

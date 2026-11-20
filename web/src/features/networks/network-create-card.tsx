import type { FormEventHandler } from "react";
import { AlertTriangle, LoaderCircle, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { CreateFormState } from "@/features/networks/network-ui";

export function NetworkCreateCard({ form, error, canManage, pending, onChange, onSubmit, onCancel }: {
  form: CreateFormState;
  error: string;
  canManage: boolean;
  pending: boolean;
  onChange: <K extends keyof CreateFormState>(key: K, value: CreateFormState[K]) => void;
  onSubmit: FormEventHandler<HTMLFormElement>;
  onCancel: () => void;
}) {
  return <Card className="mb-4">
    <CardHeader>
      <CardTitle>创建虚拟网络</CardTitle>
      <CardDescription>{form.mode === "bridge" ? "接入宿主机已有网桥，地址和 DHCP 由上游网络提供。" : "创建持久化网络定义。IPv4 与 IPv6 至少配置一项；IPv6 支持 SLAAC 或 DHCPv6。"}</CardDescription>
    </CardHeader>
    <CardContent>
      <form id="network-create-form" data-testid="network-create-form" onSubmit={onSubmit} className="space-y-4">
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <div className="space-y-2">
            <Label htmlFor="network-name">网络名称</Label>
            <Input id="network-name" value={form.name} onChange={(event) => onChange("name", event.target.value)} placeholder="lab-net" required disabled={!canManage} autoComplete="off" />
          </div>
          <div className="space-y-2">
            <Label htmlFor="network-mode">网络模式</Label>
            <select id="network-mode" value={form.mode} onChange={(event) => onChange("mode", event.target.value as CreateFormState["mode"])} disabled={!canManage} className="flex h-9 w-full rounded-md border border-input bg-card px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 disabled:opacity-50">
              <option value="nat">NAT 转发</option>
              <option value="isolated">隔离网络</option>
              <option value="bridge">外部桥接</option>
            </select>
          </div>
          {form.mode === "bridge" ? (
            <div className="space-y-2">
              <Label htmlFor="network-bridge">宿主机网桥</Label>
              <Input id="network-bridge" data-testid="network-bridge" list="network-bridge-options" value={form.bridge} onChange={(event) => onChange("bridge", event.target.value)} placeholder="br0" required disabled={!canManage} autoComplete="off" />
              <p className="text-[11px] text-muted-foreground">仅列出 libvirt 已识别的活跃网桥；也可输入列表中未显示的名称。</p>
            </div>
          ) : (
            <div className="space-y-2">
              <Label htmlFor="network-subnet">IPv4 子网（可选）</Label>
              <Input id="network-subnet" data-testid="network-subnet" value={form.subnet} onChange={(event) => onChange("subnet", event.target.value)} placeholder="10.20.30.0/24 或留空" disabled={!canManage} />
            </div>
          )}
          <div className="flex flex-col justify-end gap-2 pb-1 text-sm">
            <label className="flex items-center gap-2">
              <input type="checkbox" className="h-4 w-4" checked={form.autostart} onChange={(event) => onChange("autostart", event.target.checked)} disabled={!canManage} />
              开机自动启动
            </label>
            <label className="flex items-center gap-2">
              <input type="checkbox" className="h-4 w-4" checked={form.start} onChange={(event) => onChange("start", event.target.checked)} disabled={!canManage} />
              创建后立即启动
            </label>
          </div>
          {form.mode !== "bridge" ? (
            <>
              <div className="space-y-2">
                <Label htmlFor="network-dhcp-start">DHCP 起始地址（可选）</Label>
                <Input id="network-dhcp-start" value={form.dhcpStart} onChange={(event) => onChange("dhcpStart", event.target.value)} placeholder="10.20.30.10" disabled={!canManage} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="network-dhcp-end">DHCP 结束地址（可选）</Label>
                <Input id="network-dhcp-end" value={form.dhcpEnd} onChange={(event) => onChange("dhcpEnd", event.target.value)} placeholder="10.20.30.200" disabled={!canManage} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="network-mtu">网络 MTU（可选）</Label>
                <Input id="network-mtu" data-testid="network-mtu" type="number" min={576} max={9000} value={form.mtu} onChange={(event) => onChange("mtu", event.target.value)} placeholder="留空使用默认值" disabled={!canManage} />
                <p className="text-[11px] text-muted-foreground">范围 576-9000；启用 IPv6 时至少 1280。留空表示不固定 MTU。</p>
              </div>
              <div className="space-y-2 md:col-span-2 xl:col-span-4">
                <div className="rounded-md border border-dashed bg-muted/20 p-3">
                  <div className="mb-3 space-y-1">
                    <p className="text-xs font-medium text-foreground">IPv6 网络（可选）</p>
                    <p className="text-[11px] text-muted-foreground">仅填 IPv4 子网为 IPv4-only；仅填 IPv6 子网为 IPv6-only；两者同时填写为双栈。</p>
                  </div>
                  <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                    <div className="space-y-2">
                      <Label htmlFor="network-ipv6-subnet">IPv6 子网</Label>
                      <Input id="network-ipv6-subnet" data-testid="network-ipv6-subnet" value={form.ipv6Subnet} onChange={(event) => onChange("ipv6Subnet", event.target.value)} placeholder="fd00:20:30::/64" disabled={!canManage} />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="network-ipv6-mode">IPv6 地址分配</Label>
                      <select id="network-ipv6-mode" data-testid="network-ipv6-mode" value={form.ipv6Mode} onChange={(event) => onChange("ipv6Mode", event.target.value as CreateFormState["ipv6Mode"])} disabled={!canManage || !form.ipv6Subnet.trim()} className="flex h-9 w-full rounded-md border border-input bg-card px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 disabled:opacity-50">
                        <option value="dhcpv6">DHCPv6（有状态）</option>
                        <option value="slaac">SLAAC / RA（/64）</option>
                      </select>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="network-ipv6-dhcp-start">DHCPv6 起始地址</Label>
                      <Input id="network-ipv6-dhcp-start" data-testid="network-ipv6-dhcp-start" value={form.ipv6DhcpStart} onChange={(event) => onChange("ipv6DhcpStart", event.target.value)} placeholder="留空自动分配" disabled={!canManage || form.ipv6Mode !== "dhcpv6" || !form.ipv6Subnet.trim()} />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="network-ipv6-dhcp-end">DHCPv6 结束地址</Label>
                      <Input id="network-ipv6-dhcp-end" data-testid="network-ipv6-dhcp-end" value={form.ipv6DhcpEnd} onChange={(event) => onChange("ipv6DhcpEnd", event.target.value)} placeholder="留空自动分配" disabled={!canManage || form.ipv6Mode !== "dhcpv6" || !form.ipv6Subnet.trim()} />
                    </div>
                  </div>
                  <div className="mt-3 flex flex-wrap items-center gap-x-6 gap-y-2 text-sm">
                    <label className="flex items-center gap-2">
                      <input type="checkbox" data-testid="network-nat-ipv6" className="h-4 w-4" checked={form.natIpv6} onChange={(event) => onChange("natIpv6", event.target.checked)} disabled={!canManage || form.mode !== "nat" || !form.ipv6Subnet.trim()} />
                      启用 IPv6 NAT（NAT66）
                    </label>
                    <span className="text-[11px] text-muted-foreground">SLAAC 仅支持 /64；DHCPv6 支持 /64 至 /124。NAT66 仅提供客户机出站访问，不含 IPv6 入站端口映射。</span>
                  </div>
                </div>
              </div>
              <div className="space-y-2 md:col-span-2 xl:col-span-4">
                <div className="grid gap-3 rounded-md border bg-muted/10 p-3 md:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="network-dns-domain">DNS 域（可选）</Label>
                    <Input id="network-dns-domain" data-testid="network-dns-domain" value={form.dnsDomain} onChange={(event) => onChange("dnsDomain", event.target.value)} placeholder="lab.example.com" disabled={!canManage} />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="network-dns-servers">DNS 服务器（逗号分隔）</Label>
                    <Input id="network-dns-servers" data-testid="network-dns-servers" value={form.dnsServers} onChange={(event) => onChange("dnsServers", event.target.value)} placeholder="1.1.1.1, 2606:4700:4700::1111" disabled={!canManage} className="font-mono" />
                  </div>
                </div>
              </div>
            </>
          ) : null}
        </div>
        {error ? (
          <p className="flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert" data-testid="network-create-error">
            <AlertTriangle className="h-3.5 w-3.5" />{error}
          </p>
        ) : null}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-xs text-muted-foreground">{form.mode === "bridge" ? "VirtMgr 只创建 libvirt 网络定义，不创建或修改宿主机网桥。" : "IPv4/IPv6 至少填写一项；DHCP 地址留空时由系统推导可用范围。"}</p>
          <div className="flex items-center gap-2">
            <Button type="button" variant="ghost" size="sm" onClick={onCancel}>取消</Button>
            <Button type="submit" size="sm" data-testid="network-create-submit" disabled={!canManage || pending}>
              {pending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
              创建网络
            </Button>
          </div>
        </div>
      </form>
    </CardContent>
  </Card>;
}

import type { FormEventHandler } from "react";
import { AlertTriangle, LoaderCircle, Pencil } from "lucide-react";
import type { VirtualNetwork } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { NetworkEditFormState } from "@/features/networks/network-edit-model";

export function NetworkEditCard({ target, form, error, pending, onChange, onSubmit, onCancel }: {
  target: VirtualNetwork;
  form: NetworkEditFormState;
  error: string;
  pending: boolean;
  onChange: <K extends keyof NetworkEditFormState>(key: K, value: NetworkEditFormState[K]) => void;
  onSubmit: FormEventHandler<HTMLFormElement>;
  onCancel: () => void;
}) {
  return <Card className="mb-4">
    <CardHeader>
      <CardTitle>编辑虚拟网络 · {target.name}</CardTitle>
      <CardDescription>网络名称不可修改。保存后将在网络下次启动时使用新配置。</CardDescription>
    </CardHeader>
    <CardContent>
      <form id="network-edit-form" data-testid="network-edit-form" onSubmit={onSubmit} className="space-y-4">
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <div className="space-y-2">
            <Label htmlFor="network-edit-name">网络名称</Label>
            <Input id="network-edit-name" value={target.name} readOnly disabled className="font-mono" />
          </div>
          <div className="space-y-2">
            <Label htmlFor="network-edit-mode">网络模式</Label>
            <select id="network-edit-mode" value={form.mode} onChange={(event) => onChange("mode", event.target.value as NetworkEditFormState["mode"])} disabled={pending} className="flex h-9 w-full rounded-md border border-input bg-card px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 disabled:opacity-50">
              <option value="nat">NAT 转发</option>
              <option value="isolated">隔离网络</option>
              <option value="bridge">外部桥接</option>
            </select>
          </div>
          {form.mode === "bridge" ? (
            <div className="space-y-2">
              <Label htmlFor="network-edit-bridge">宿主机网桥</Label>
              <Input id="network-edit-bridge" data-testid="network-edit-bridge" list="network-bridge-options" value={form.bridge} onChange={(event) => onChange("bridge", event.target.value)} placeholder="br0" required disabled={pending} autoComplete="off" />
            </div>
          ) : (
            <div className="space-y-2">
              <Label htmlFor="network-edit-subnet">IPv4 子网（可选）</Label>
              <Input id="network-edit-subnet" data-testid="network-edit-subnet" value={form.subnet} onChange={(event) => onChange("subnet", event.target.value)} placeholder="10.20.30.0/24 或留空" disabled={pending} />
            </div>
          )}
          <div className="flex flex-col justify-end gap-2 pb-1 text-sm">
            <label className="flex items-center gap-2">
              <input type="checkbox" className="h-4 w-4" checked={form.autostart} onChange={(event) => onChange("autostart", event.target.checked)} disabled={pending} />
              开机自动启动
            </label>
          </div>
          {form.mode !== "bridge" ? (
            <>
              <div className="space-y-2">
                <Label htmlFor="network-edit-dhcp-start">DHCP 起始地址（可选）</Label>
                <Input id="network-edit-dhcp-start" value={form.dhcpStart} onChange={(event) => onChange("dhcpStart", event.target.value)} placeholder="10.20.30.10" disabled={pending} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="network-edit-dhcp-end">DHCP 结束地址（可选）</Label>
                <Input id="network-edit-dhcp-end" value={form.dhcpEnd} onChange={(event) => onChange("dhcpEnd", event.target.value)} placeholder="10.20.30.200" disabled={pending} />
              </div>
              <div className="space-y-2 md:col-span-2 xl:col-span-4">
                <div className="rounded-md border border-dashed bg-muted/20 p-3">
                  <p className="mb-3 text-xs font-medium">IPv6 双栈（可选）</p>
                  <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                    <div className="space-y-2">
                      <Label htmlFor="network-edit-ipv6-subnet">IPv6 子网</Label>
                      <Input id="network-edit-ipv6-subnet" data-testid="network-edit-ipv6-subnet" value={form.ipv6Subnet} onChange={(event) => onChange("ipv6Subnet", event.target.value)} placeholder="fd00:20:30::/64" disabled={pending} />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="network-edit-ipv6-mode">IPv6 地址分配</Label>
                      <select id="network-edit-ipv6-mode" data-testid="network-edit-ipv6-mode" value={form.ipv6Mode} onChange={(event) => onChange("ipv6Mode", event.target.value as NetworkEditFormState["ipv6Mode"])} disabled={pending || !form.ipv6Subnet.trim()} className="flex h-9 w-full rounded-md border border-input bg-card px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 disabled:opacity-50">
                        <option value="dhcpv6">DHCPv6（有状态）</option>
                        <option value="slaac">SLAAC / RA（/64）</option>
                      </select>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="network-edit-ipv6-dhcp-start">DHCPv6 起始地址</Label>
                      <Input id="network-edit-ipv6-dhcp-start" value={form.ipv6DhcpStart} onChange={(event) => onChange("ipv6DhcpStart", event.target.value)} placeholder="留空自动分配" disabled={pending || form.ipv6Mode !== "dhcpv6" || !form.ipv6Subnet.trim()} />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="network-edit-ipv6-dhcp-end">DHCPv6 结束地址</Label>
                      <Input id="network-edit-ipv6-dhcp-end" value={form.ipv6DhcpEnd} onChange={(event) => onChange("ipv6DhcpEnd", event.target.value)} placeholder="留空自动分配" disabled={pending || form.ipv6Mode !== "dhcpv6" || !form.ipv6Subnet.trim()} />
                    </div>
                  </div>
                  <div className="mt-3 flex flex-wrap items-center gap-x-6 gap-y-2 text-sm">
                    <label className="flex items-center gap-2">
                      <input type="checkbox" data-testid="network-edit-nat-ipv6" className="h-4 w-4" checked={form.natIpv6} onChange={(event) => onChange("natIpv6", event.target.checked)} disabled={pending || form.mode !== "nat" || !form.ipv6Subnet.trim()} />
                      启用 IPv6 NAT（NAT66）
                    </label>
                    <span className="text-[11px] text-muted-foreground">清空 IPv6 子网会移除 IPv6 和 DHCPv6 配置；已有静态主机时后端会阻止危险变更。NAT66 仅提供客户机出站访问。</span>
                  </div>
                </div>
              </div>
              <div className="space-y-2 md:col-span-2 xl:col-span-4">
                <div className="grid gap-3 rounded-md border bg-muted/10 p-3 md:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="network-edit-dns-domain">DNS 域（可选）</Label>
                    <Input id="network-edit-dns-domain" data-testid="network-edit-dns-domain" value={form.dnsDomain} onChange={(event) => onChange("dnsDomain", event.target.value)} placeholder="lab.example.com" disabled={pending} />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="network-edit-dns-servers">DNS 服务器（逗号分隔）</Label>
                    <Input id="network-edit-dns-servers" data-testid="network-edit-dns-servers" value={form.dnsServers} onChange={(event) => onChange("dnsServers", event.target.value)} placeholder="1.1.1.1, 2606:4700:4700::1111" disabled={pending} className="font-mono" />
                  </div>
                </div>
              </div>
            </>
          ) : null}
        </div>
        {error ? (
          <p className="flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert" data-testid="network-edit-error">
            <AlertTriangle className="h-3.5 w-3.5" />{error}
          </p>
        ) : null}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-xs text-muted-foreground">{form.mode === "bridge" ? "保存只更新 libvirt 网络定义，不创建或修改宿主机网桥。" : "IPv4/IPv6 至少保留一项；修改前缀前请确认静态主机和虚拟机地址已完成迁移。"}</p>
          <div className="flex items-center gap-2">
            <Button type="button" variant="ghost" size="sm" disabled={pending} onClick={onCancel}>取消</Button>
            <Button type="submit" size="sm" data-testid="network-edit-submit" disabled={pending}>
              {pending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Pencil className="h-4 w-4" />}
              保存配置
            </Button>
          </div>
        </div>
      </form>
    </CardContent>
  </Card>;
}

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
      <CardDescription>{form.mode === "bridge" ? "接入宿主机已有网桥，地址和 DHCP 由上游网络提供。" : "创建持久化网络定义。子网至少为 /29，网关固定为网络地址加一。"}</CardDescription>
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
              <Label htmlFor="network-subnet">IPv4 子网</Label>
              <Input id="network-subnet" value={form.subnet} onChange={(event) => onChange("subnet", event.target.value)} placeholder="10.20.30.0/24" required disabled={!canManage} />
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
            </>
          ) : null}
        </div>
        {error ? (
          <p className="flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert" data-testid="network-create-error">
            <AlertTriangle className="h-3.5 w-3.5" />{error}
          </p>
        ) : null}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-xs text-muted-foreground">{form.mode === "bridge" ? "VirtMgr 只创建 libvirt 网络定义，不创建或修改宿主机网桥。" : "DHCP 起止地址留空时自动使用子网内可用范围；只填写一项将被拒绝。"}</p>
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

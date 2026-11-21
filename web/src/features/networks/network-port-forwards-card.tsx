import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, ArrowRightLeft, LoaderCircle, Pencil, Plus, RefreshCw, Trash2, X } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { PortForward, PortForwardInput, VirtualNetwork } from "@/api/types";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
  buildPortForwardInput,
  emptyPortForwardForm,
  portForwardForm,
  type PortForwardFormState,
} from "@/features/networks/network-port-forward-model";
import { useNetworkPortForwardsQuery } from "@/features/networks/use-network-port-forwards-query";
import { queryKeys } from "@/lib/query-keys";
import { cn } from "@/lib/utils";

export function NetworkPortForwardsCard({ network, canManage, disabledReason, onClose }: {
  network: VirtualNetwork;
  canManage: boolean;
  disabledReason: string;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const query = useNetworkPortForwardsQuery(network.name);
  const [form, setForm] = useState<PortForwardFormState>(emptyPortForwardForm);
  const [editing, setEditing] = useState<PortForward | null>(null);
  const [formError, setFormError] = useState("");
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<PortForward | null>(null);
  const items = query.data?.items ?? [];
  const nat = network.forwardMode === "nat";

  function invalidate(name: string) {
    void queryClient.invalidateQueries({ queryKey: queryKeys.networks.portForwards(name) });
    void queryClient.invalidateQueries({ queryKey: queryKeys.networks.diagnostics(name) });
    void queryClient.invalidateQueries({ queryKey: queryKeys.networks.list });
  }

  const saveMutation = useMutation({
    mutationFn: (input: PortForwardInput) => editing
      ? api.updateNetworkPortForward(network.name, editing.id, input)
      : api.createNetworkPortForward(network.name, input),
    onSuccess: (saved) => {
      setNotice({ tone: "success", text: `${editing ? "已更新" : "已创建"}端口转发“${saved.name}”。` });
      setForm(emptyPortForwardForm);
      setEditing(null);
      setFormError("");
      invalidate(network.name);
    },
    onError: (cause) => setFormError(cause instanceof ApiError ? cause.message : "保存端口转发失败。"),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteNetworkPortForward(network.name, id),
    onSuccess: (_result, id) => {
      const removed = items.find((item) => item.id === id);
      setNotice({ tone: "success", text: `已删除端口转发${removed ? `“${removed.name}”` : ""}。` });
      setDeleteTarget(null);
      if (editing?.id === id) {
        setEditing(null);
        setForm(emptyPortForwardForm);
      }
      invalidate(network.name);
    },
    onError: (cause) => setNotice({ tone: "error", text: cause instanceof ApiError ? cause.message : "删除端口转发失败。" }),
  });

  function change<K extends keyof PortForwardFormState>(key: K, value: PortForwardFormState[K]) {
    setForm((current) => ({ ...current, [key]: value }));
    setFormError("");
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const result = buildPortForwardInput(form);
    if (!result.input) {
      setFormError(result.error ?? "请检查输入。");
      return;
    }
    setFormError("");
    setNotice(null);
    saveMutation.mutate(result.input);
  }

  function beginEdit(item: PortForward) {
    setEditing(item);
    setForm(portForwardForm(item));
    setFormError("");
    setNotice(null);
  }

  return <>
    <Card className="mt-4" data-testid="network-port-forwards-card">
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2"><ArrowRightLeft className="h-4 w-4" />入站端口转发 · {network.name}</CardTitle>
            <CardDescription>通过 nftables DNAT 将宿主机 IPv4/IPv6 端口转发到 NAT 网络内的客户机。规则会立即应用到宿主机防火墙。</CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <Badge variant={nat ? "success" : "warning"}>{nat ? "NAT 网络" : "非 NAT 网络"}</Badge>
            <Button type="button" variant="outline" size="sm" onClick={() => void query.refetch()} disabled={query.isFetching}>
              <RefreshCw className={cn("h-4 w-4", query.isFetching && "animate-spin")} />刷新
            </Button>
            <Button type="button" variant="ghost" size="sm" onClick={onClose}>收起</Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {!nat ? <p className="rounded-md border border-warning/35 bg-warning/10 px-3 py-2 text-xs text-warning">只有 NAT 网络支持入站端口转发。桥接和隔离网络请在上游路由器或宿主机外部防火墙配置。</p> : null}
        {!canManage ? <p className="rounded-md border bg-muted/40 px-3 py-2 text-xs text-muted-foreground" role="status">{disabledReason || "当前账号只能查看端口转发规则。"}</p> : null}
        {notice ? <p role="status" className={cn("rounded-md border px-3 py-2 text-xs", notice.tone === "success" ? "border-success/30 bg-success/10 text-success" : "border-destructive/30 bg-destructive/5 text-destructive")}>{notice.text}</p> : null}
        {query.isError ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{query.error instanceof Error ? query.error.message : "无法读取端口转发规则。"}</p> : null}

        {canManage && nat ? (
          <form onSubmit={submit} data-testid="network-port-forward-form" className="space-y-3 rounded-lg border bg-muted/20 p-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-sm font-medium">{editing ? `编辑规则 · ${editing.name}` : "新建入站规则"}</p>
              {editing ? <Button type="button" variant="ghost" size="sm" onClick={() => { setEditing(null); setForm(emptyPortForwardForm); setFormError(""); }}><X className="h-3.5 w-3.5" />取消编辑</Button> : null}
            </div>
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <div className="space-y-1.5"><Label htmlFor="port-forward-name">规则名称</Label><Input id="port-forward-name" data-testid="port-forward-name" value={form.name} onChange={(event) => change("name", event.target.value)} placeholder="web-https" maxLength={64} disabled={saveMutation.isPending} /></div>
              <div className="space-y-1.5"><Label htmlFor="port-forward-family">地址族</Label><select id="port-forward-family" data-testid="port-forward-family" value={form.family} onChange={(event) => change("family", event.target.value as PortForwardFormState["family"])} disabled={saveMutation.isPending} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm"><option value="ipv4">IPv4</option><option value="ipv6">IPv6</option></select></div>
              <div className="space-y-1.5"><Label htmlFor="port-forward-protocol">协议</Label><select id="port-forward-protocol" data-testid="port-forward-protocol" value={form.protocol} onChange={(event) => change("protocol", event.target.value as PortForwardFormState["protocol"])} disabled={saveMutation.isPending} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm"><option value="tcp">TCP</option><option value="udp">UDP</option></select></div>
              <div className="space-y-1.5"><Label htmlFor="port-forward-enabled">状态</Label><label className="flex h-9 items-center gap-2 rounded-md border border-input bg-card px-3 text-sm"><input id="port-forward-enabled" type="checkbox" checked={form.enabled} onChange={(event) => change("enabled", event.target.checked)} disabled={saveMutation.isPending} />立即启用</label></div>
              <div className="space-y-1.5"><Label htmlFor="port-forward-host-address">宿主机地址（可选）</Label><Input id="port-forward-host-address" data-testid="port-forward-host-address" value={form.hostAddress} onChange={(event) => change("hostAddress", event.target.value)} placeholder="留空表示全部地址" disabled={saveMutation.isPending} className="font-mono" /></div>
              <div className="space-y-1.5"><Label htmlFor="port-forward-host-port">宿主机端口</Label><Input id="port-forward-host-port" data-testid="port-forward-host-port" type="number" min={1} max={65535} value={form.hostPort} onChange={(event) => change("hostPort", event.target.value)} placeholder="443" disabled={saveMutation.isPending} /></div>
              <div className="space-y-1.5"><Label htmlFor="port-forward-guest-address">客户机地址</Label><Input id="port-forward-guest-address" data-testid="port-forward-guest-address" value={form.guestAddress} onChange={(event) => change("guestAddress", event.target.value)} placeholder={form.family === "ipv6" ? "fd00:122::20" : "192.168.122.20"} disabled={saveMutation.isPending} className="font-mono" /></div>
              <div className="space-y-1.5"><Label htmlFor="port-forward-guest-port">客户机端口</Label><Input id="port-forward-guest-port" data-testid="port-forward-guest-port" type="number" min={1} max={65535} value={form.guestPort} onChange={(event) => change("guestPort", event.target.value)} placeholder="443" disabled={saveMutation.isPending} /></div>
            </div>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <p className="text-xs text-muted-foreground">客户机地址必须位于当前网络对应地址族的子网内，且不能是网关地址；端口规则会立即重建受控 nftables 表。</p>
              <Button type="submit" size="sm" data-testid="port-forward-submit" disabled={saveMutation.isPending}>{saveMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : editing ? <Pencil className="h-4 w-4" /> : <Plus className="h-4 w-4" />}{saveMutation.isPending ? "正在应用..." : editing ? "保存修改" : "添加规则"}</Button>
            </div>
            {formError ? <p role="alert" data-testid="port-forward-error" className="flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive"><AlertTriangle className="h-3.5 w-3.5" />{formError}</p> : null}
          </form>
        ) : null}

        <div className="overflow-x-auto rounded-md border">
          {query.isLoading ? <p className="p-4 text-sm text-muted-foreground">正在读取端口转发规则...</p> : items.length ? (
            <Table className="min-w-[860px]">
              <TableHeader><TableRow><TableHead>名称</TableHead><TableHead>协议 / 地址族</TableHead><TableHead>宿主机</TableHead><TableHead>客户机</TableHead><TableHead>状态</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
              <TableBody>{items.map((item) => <TableRow key={item.id} data-testid={`port-forward-row-${item.id}`}>
                <TableCell><p className="font-medium">{item.name}</p><p className="font-mono text-[10px] text-muted-foreground">{item.id}</p></TableCell>
                <TableCell><Badge variant="outline">{item.protocol.toUpperCase()}</Badge> <Badge variant="secondary">{item.family.toUpperCase()}</Badge></TableCell>
                <TableCell className="font-mono text-xs">{item.hostAddress || "*"}:{item.hostPort}</TableCell>
                <TableCell className="font-mono text-xs">{item.family === "ipv6" ? `[${item.guestAddress}]` : item.guestAddress}:{item.guestPort}</TableCell>
                <TableCell><Badge variant={item.enabled ? "success" : "secondary"}>{item.enabled ? "启用" : "停用"}</Badge></TableCell>
                <TableCell className="text-right"><div className="flex justify-end gap-1"><Button type="button" variant="ghost" size="sm" data-testid={`port-forward-edit-${item.id}`} disabled={!canManage || !nat} onClick={() => beginEdit(item)}><Pencil className="h-3.5 w-3.5" />编辑</Button><Button type="button" variant="ghost" size="sm" className="text-destructive hover:bg-destructive/10 hover:text-destructive" data-testid={`port-forward-delete-${item.id}`} disabled={!canManage || !nat || deleteMutation.isPending} onClick={() => { setNotice(null); setDeleteTarget(item); }}><Trash2 className="h-3.5 w-3.5" />删除</Button></div></TableCell>
              </TableRow>)}</TableBody>
            </Table>
          ) : <div className="px-4 py-8 text-center" data-testid="port-forwards-empty"><ArrowRightLeft className="mx-auto h-7 w-7 text-muted-foreground" /><p className="mt-2 text-sm font-medium">暂无端口转发规则</p><p className="mt-1 text-xs text-muted-foreground">仅 NAT 网络支持入站转发；规则应用成功后才写入本地目录。</p></div>}
        </div>
      </CardContent>
    </Card>

    <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => { if (!open) setDeleteTarget(null); }}>
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>删除端口转发规则？</AlertDialogTitle><AlertDialogDescription>将从宿主机防火墙移除“{deleteTarget?.name}”的 DNAT 与 FORWARD 规则。已建立的连接可能中断。</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel disabled={deleteMutation.isPending}>取消</AlertDialogCancel><Button variant="destructive" data-testid="port-forward-delete-confirm" disabled={deleteMutation.isPending} onClick={() => { if (deleteTarget) deleteMutation.mutate(deleteTarget.id); }}>{deleteMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}确认删除</Button></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </>;
}

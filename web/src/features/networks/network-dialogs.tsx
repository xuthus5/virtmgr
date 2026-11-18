import { LoaderCircle, Trash2 } from "lucide-react";
import type { NetworkDHCPHost, VirtualNetwork } from "@/api/types";
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function NetworkDeleteDialog({ target, name, error, pending, onNameChange, onOpenChange, onConfirm }: {
  target: VirtualNetwork | null;
  name: string;
  error: string;
  pending: boolean;
  onNameChange: (value: string) => void;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}) {
  return <AlertDialog open={target !== null} onOpenChange={onOpenChange}>
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>删除虚拟网络？</AlertDialogTitle>
        <AlertDialogDescription>
          将从 libvirt 永久删除网络 <span className="font-medium text-foreground">{target?.name}</span>。仅未激活网络可以删除，正在使用该网络的虚拟机不会自动调整配置。
        </AlertDialogDescription>
      </AlertDialogHeader>
      <div className="space-y-2">
        <Label htmlFor="delete-network-name">输入网络名称确认</Label>
        <Input id="delete-network-name" value={name} onChange={(event) => onNameChange(event.target.value)} placeholder={target?.name} autoComplete="off" />
      </div>
      {error ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert">{error}</p> : null}
      <AlertDialogFooter>
        <AlertDialogCancel>取消</AlertDialogCancel>
        <Button variant="destructive" data-testid="confirm-delete-network" disabled={!target || name !== target.name || pending} onClick={onConfirm}>
          {pending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : null}
          确认删除
        </Button>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>;
}

export function NetworkHostDeleteDialog({ target, error, pending, onOpenChange, onConfirm }: {
  target: { network: VirtualNetwork; host: NetworkDHCPHost } | null;
  error: string;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}) {
  return <AlertDialog open={target !== null} onOpenChange={onOpenChange}>
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>删除静态主机条目？</AlertDialogTitle>
        <AlertDialogDescription>
          将从网络 <span className="font-medium text-foreground">{target?.network.name}</span> 删除
          <span className="font-mono text-foreground"> {target?.host.mac} → {target?.host.ip}</span> 的固定地址分配。
          {target?.network.active ? "该变更会立即应用到运行中的网络。" : "该变更会写入持久配置。"}
        </AlertDialogDescription>
      </AlertDialogHeader>
      {error ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert">{error}</p> : null}
      <AlertDialogFooter>
        <AlertDialogCancel disabled={pending}>取消</AlertDialogCancel>
        <Button variant="destructive" data-testid="confirm-delete-network-host" disabled={pending} onClick={onConfirm}>
          {pending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
          删除条目
        </Button>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>;
}

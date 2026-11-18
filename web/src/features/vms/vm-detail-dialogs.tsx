import { LoaderCircle, Save } from "lucide-react";
import type { VMAction } from "@/api/types";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function confirmCopy(action: VMAction | null) {
  if (action === "destroy") {
    return {
      title: "确认强制下电？",
      description: "该操作会立即终止虚拟机，不会等待客户机保存数据。未保存的磁盘写入可能丢失。",
      button: "确认强制下电",
    };
  }
  return {
    title: "确认强制重启？",
    description: "该操作会立即重置虚拟机并重启，未保存的数据可能丢失。",
    button: "确认强制重启",
  };
}

export function VMActionConfirmDialog({ action, pending, onOpenChange, onConfirm }: {
  action: VMAction | null;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (action: VMAction) => void;
}) {
  const copy = confirmCopy(action);
  return <AlertDialog open={action !== null} onOpenChange={onOpenChange}>
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{copy.title}</AlertDialogTitle>
        <AlertDialogDescription>{copy.description}</AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel disabled={pending}>取消</AlertDialogCancel>
        <AlertDialogAction disabled={pending} onClick={() => { if (action) onConfirm(action); }}>{copy.button}</AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>;
}

export function VMDeleteDialog({ vmName, open, name, error, pending, onOpenChange, onNameChange, onConfirm }: {
  vmName: string;
  open: boolean;
  name: string;
  error: string;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onNameChange: (value: string) => void;
  onConfirm: () => void;
}) {
  return <AlertDialog open={open} onOpenChange={onOpenChange}>
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>删除虚拟机？</AlertDialogTitle>
        <AlertDialogDescription>将删除 <span className="font-medium text-foreground">{vmName}</span> 的 libvirt 域定义，磁盘文件保留。此操作不可撤销。</AlertDialogDescription>
      </AlertDialogHeader>
      <div className="space-y-2">
        <Label htmlFor="delete-vm-name">输入虚拟机名称确认</Label>
        <Input id="delete-vm-name" value={name} onChange={(event) => onNameChange(event.target.value)} placeholder={vmName} autoComplete="off" />
      </div>
      {error ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert">{error}</p> : null}
      <AlertDialogFooter>
        <AlertDialogCancel>取消</AlertDialogCancel>
        <Button variant="destructive" data-testid="confirm-delete-vm" disabled={name !== vmName || pending} onClick={onConfirm}>
          {pending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : null}
          确认删除
        </Button>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>;
}

export function VMXMLSaveDialog({ vmName, open, pending, onOpenChange, onConfirm }: {
  vmName: string;
  open: boolean;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}) {
  return <AlertDialog open={open} onOpenChange={onOpenChange}>
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>保存持久配置 XML？</AlertDialogTitle>
        <AlertDialogDescription>将使用 libvirt 校验并更新 <span className="font-medium text-foreground">{vmName}</span> 的持久域定义。名称和 UUID 必须保持不变；不会启动虚拟机，运行中的域将在下次启动时使用新定义。</AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel disabled={pending}>取消</AlertDialogCancel>
        <Button data-testid="confirm-vm-xml-save" disabled={pending} onClick={onConfirm}>
          {pending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}保存定义
        </Button>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>;
}

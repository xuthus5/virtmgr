import { useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, KeyRound, LoaderCircle } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { VM } from "@/api/types";
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
import { useGuestAgentQuery } from "@/features/guest-agent/use-guest-agent-query";

function errorMessage(cause: unknown, fallback: string) {
  return cause instanceof ApiError ? cause.message : fallback;
}

export function VMPasswordCard({ vm, canMutate, disabledReason }: {
  vm: VM;
  canMutate: boolean;
  disabledReason: string;
}) {
  const query = useGuestAgentQuery(vm.uuid);
  const [username, setUsername] = useState("root");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const info = query.data;
  const agentConnected = info?.state === "connected";
  const passwordSupported = Boolean(info?.capabilities.passwordReset);
  const available = canMutate && agentConnected && passwordSupported;

  const unavailableReason = query.isError
    ? query.error instanceof Error ? query.error.message : "无法读取 Guest Agent 状态"
    : query.isLoading || !info
      ? "正在检测 Guest Agent 状态"
      : !canMutate
        ? disabledReason || "当前账号没有客户机密码修改权限"
        : !agentConnected
          ? info.channelConfigured
            ? "QEMU Guest Agent 未连接。请启动客户机，并确认 qemu-guest-agent 服务正在运行。"
            : "QGA 通道未配置。请先关闭虚拟机，再在 Guest Agent 卡片中补充通道。"
          : !passwordSupported
            ? "当前 Guest Agent 未声明密码重置能力，请升级客户机内的 qemu-guest-agent。"
            : "";

  const mutation = useMutation({
    mutationFn: () => api.resetGuestAgentPassword(vm.uuid, { username: username.trim(), password }),
    onSuccess: (task) => {
      setConfirmOpen(false);
      setPassword("");
      setConfirmation("");
      setNotice({ tone: "success", text: `客户机用户 ${username.trim()} 的密码修改任务已创建（${task.id}）。密码明文不会写入审计或任务记录。` });
    },
    onError: (cause) => {
      setConfirmOpen(false);
      setNotice({ tone: "error", text: errorMessage(cause, "修改客户机用户密码失败。") });
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!available || !username.trim() || !password) return;
    if (password !== confirmation) {
      setNotice({ tone: "error", text: "两次输入的新密码不一致。" });
      return;
    }
    setNotice(null);
    setConfirmOpen(true);
  }

  const stateLabel = query.isLoading
    ? "正在检测 Guest Agent"
    : query.isError
      ? "Guest Agent 状态不可用"
      : agentConnected && passwordSupported
        ? "Guest Agent 已连接"
        : agentConnected
          ? "代理不支持密码修改"
          : "Guest Agent 未连接";
  const stateVariant = agentConnected && passwordSupported ? "success" : query.isError ? "destructive" : "warning";

  return (
    <Card className="mt-4 scroll-mt-20" id="vm-password-card" data-testid="vm-password-card">
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2"><KeyRound className="h-4 w-4 text-primary" />修改 / 重置客户机密码</CardTitle>
            <CardDescription>通过 QEMU Guest Agent 为虚拟机内已有的 Linux 或 Windows 用户设置新密码。此操作不会修改 VirtMgr 控制台登录密码。</CardDescription>
          </div>
          <Badge variant={stateVariant} data-testid="vm-password-state">{stateLabel}</Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {unavailableReason ? (
          <div className="flex items-start gap-2 rounded-md border border-warning/35 bg-warning/10 px-3 py-2 text-xs text-warning" role="status">
            <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <span>{unavailableReason} <a href="#vm-guest-agent-card" className="font-medium underline underline-offset-4">查看 Guest Agent 安装与诊断</a></span>
          </div>
        ) : null}

        <form className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_auto]" onSubmit={submit}>
          <div className="space-y-2">
            <Label htmlFor="vm-guest-password-username">客户机用户名</Label>
            <Input id="vm-guest-password-username" data-testid="vm-password-username" autoComplete="off" value={username} onChange={(event) => setUsername(event.target.value)} disabled={!available} required />
          </div>
          <div className="space-y-2">
            <Label htmlFor="vm-guest-password-new">新密码</Label>
            <Input id="vm-guest-password-new" data-testid="vm-password-new" type="password" autoComplete="new-password" maxLength={256} value={password} onChange={(event) => setPassword(event.target.value)} disabled={!available} required />
          </div>
          <div className="space-y-2">
            <Label htmlFor="vm-guest-password-confirm">确认新密码</Label>
            <Input id="vm-guest-password-confirm" data-testid="vm-password-confirm" type="password" autoComplete="new-password" maxLength={256} value={confirmation} onChange={(event) => setConfirmation(event.target.value)} disabled={!available} required />
          </div>
          <div className="flex items-end">
            <Button type="submit" variant="destructive" data-testid="vm-password-submit" disabled={!available || mutation.isPending || !username.trim() || !password || !confirmation} className="w-full md:w-auto">
              {mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <KeyRound className="h-4 w-4" />}修改/重置密码
            </Button>
          </div>
        </form>

        {notice ? (
          <p className={`flex items-start gap-2 rounded-md border px-3 py-2 text-xs ${notice.tone === "success" ? "border-success/30 bg-success/10 text-success" : "border-destructive/30 bg-destructive/5 text-destructive"}`} data-testid="vm-password-notice" role="status">
            {notice.tone === "success" ? <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0" /> : <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />}{notice.text}
          </p>
        ) : null}

        <details className="rounded-md border bg-muted/20 px-3 py-2 text-xs">
          <summary className="cursor-pointer font-medium">客户机需要安装 Guest Agent 才能修改密码</summary>
          <div className="mt-3 space-y-2 text-muted-foreground">
            <p>Debian / Ubuntu：<code className="ml-1 rounded bg-background px-1.5 py-0.5 font-mono text-foreground">sudo apt install qemu-guest-agent && sudo systemctl enable --now qemu-guest-agent</code></p>
            <p>RHEL / Fedora：<code className="ml-1 rounded bg-background px-1.5 py-0.5 font-mono text-foreground">sudo dnf install qemu-guest-agent && sudo systemctl enable --now qemu-guest-agent</code></p>
            <p>Windows：通过 virtio-win ISO 安装 <span className="font-mono text-foreground">guest-agent\qemu-ga-x86_64.msi</span>，并确认 QEMU Guest Agent 服务已启动。</p>
            <p>密码通过 Guest Agent 直接写入客户机，VirtMgr 不保存密码明文，审计只记录用户名和任务结果。</p>
          </div>
        </details>
      </CardContent>

      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent data-testid="vm-password-confirm-dialog">
          <AlertDialogHeader>
            <AlertDialogTitle>确认修改客户机用户密码？</AlertDialogTitle>
            <AlertDialogDescription>将通过 Guest Agent 修改虚拟机 <span className="font-medium text-foreground">{vm.name}</span> 内用户 <span className="font-mono text-foreground">{username.trim()}</span> 的密码。请确认用户名和虚拟机无误。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={mutation.isPending}>取消</AlertDialogCancel>
            <Button variant="destructive" data-testid="vm-password-confirm-submit" disabled={mutation.isPending} onClick={() => mutation.mutate()}>
              {mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : null}确认修改
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}

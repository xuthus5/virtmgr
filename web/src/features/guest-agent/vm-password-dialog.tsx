import { useEffect, useRef, useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, KeyRound, LoaderCircle } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { VM } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Modal } from "@/components/ui/modal";
import { useGuestAgentQuery } from "@/features/guest-agent/use-guest-agent-query";

function errorMessage(cause: unknown, fallback: string) {
  return cause instanceof ApiError ? cause.message : fallback;
}

export function VMPasswordDialog({ vm, open, onOpenChange, canMutate, disabledReason, onOpenGuest }: {
  vm: VM;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  canMutate: boolean;
  disabledReason: string;
  onOpenGuest: () => void;
}) {
  const query = useGuestAgentQuery(vm.uuid, open);
  const usernameRef = useRef<HTMLInputElement>(null);
  const [username, setUsername] = useState("root");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const info = query.data;
  const agentConnected = info?.state === "connected";
  const passwordSupported = Boolean(info?.capabilities.passwordReset);
  const available = canMutate && agentConnected && passwordSupported;

  useEffect(() => {
    if (!open) return;
    setConfirmed(false);
    setNotice(null);
  }, [open]);

  useEffect(() => {
    if (!open || !available) return;
    const timer = window.setTimeout(() => usernameRef.current?.focus(), 50);
    return () => window.clearTimeout(timer);
  }, [available, open]);

  const unavailableReason = query.isError
    ? query.error instanceof Error ? query.error.message : "无法读取 Guest Agent 状态"
    : query.isLoading || !info
      ? "正在检测 Guest Agent 状态"
      : !canMutate
        ? disabledReason || "当前账号没有客户机密码修改权限"
        : !agentConnected
          ? info.channelConfigured
            ? "QEMU Guest Agent 未连接。请启动客户机，并确认 qemu-guest-agent 服务正在运行。"
            : "QGA 通道未配置。请先关闭虚拟机，再补充 Guest Agent 通道。"
          : !passwordSupported
            ? "当前 Guest Agent 未声明密码重置能力，请升级客户机内的 qemu-guest-agent。"
            : "";

  const mutation = useMutation({
    mutationFn: () => api.resetGuestAgentPassword(vm.uuid, { username: username.trim(), password }),
    onSuccess: (task) => {
      setPassword("");
      setConfirmation("");
      setConfirmed(false);
      setNotice({ tone: "success", text: `客户机用户 ${username.trim()} 的密码修改任务已创建（${task.id}）。密码明文不会写入审计或任务记录。` });
    },
    onError: (cause) => {
      setNotice({ tone: "error", text: errorMessage(cause, "修改客户机用户密码失败。") });
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!available || !confirmed || !username.trim() || !password) return;
    if (password !== confirmation) {
      setNotice({ tone: "error", text: "两次输入的新密码不一致。" });
      return;
    }
    setNotice(null);
    mutation.mutate();
  }

  function close() {
    if (mutation.isPending) return;
    onOpenChange(false);
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
    <Modal
      open={open}
      onClose={close}
      title="修改 / 重置客户机密码"
      description="通过 QEMU Guest Agent 为虚拟机内已有的 Linux 或 Windows 用户设置新密码。此操作不会修改 VirtMgr 控制台登录密码。"
      testId="vm-password-dialog"
      className="max-w-xl"
      footer={(
        <>
          <Button type="button" variant="ghost" disabled={mutation.isPending} onClick={close}>取消</Button>
          <Button type="submit" form="vm-password-form" variant="destructive" data-testid="vm-password-submit" disabled={!available || !confirmed || mutation.isPending || !username.trim() || !password || !confirmation}>
            {mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <KeyRound className="h-4 w-4" />}修改/重置密码
          </Button>
        </>
      )}
    >
      <div className="space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={stateVariant} data-testid="vm-password-state">{stateLabel}</Badge>
          <span className="text-xs text-muted-foreground">目标：{vm.name}</span>
        </div>

        {unavailableReason ? (
          <div className="flex flex-col gap-3 rounded-md border border-warning/35 bg-warning/10 px-3 py-3 text-xs text-warning" role="status">
            <span className="flex items-start gap-2"><AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />{unavailableReason}</span>
            <Button type="button" size="sm" variant="outline" className="self-start" onClick={() => { onOpenChange(false); onOpenGuest(); }}>前往客户机设置</Button>
          </div>
        ) : null}

        <form id="vm-password-form" className="space-y-4" onSubmit={submit}>
          <div className="space-y-2">
            <Label htmlFor="vm-guest-password-username">客户机用户名</Label>
            <Input ref={usernameRef} id="vm-guest-password-username" data-testid="vm-password-username" autoComplete="off" value={username} onChange={(event) => setUsername(event.target.value)} disabled={!available} required />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="vm-guest-password-new">新密码</Label>
              <Input id="vm-guest-password-new" data-testid="vm-password-new" type="password" autoComplete="new-password" maxLength={256} value={password} onChange={(event) => setPassword(event.target.value)} disabled={!available} required />
            </div>
            <div className="space-y-2">
              <Label htmlFor="vm-guest-password-confirm">确认新密码</Label>
              <Input id="vm-guest-password-confirm" data-testid="vm-password-confirm" type="password" autoComplete="new-password" maxLength={256} value={confirmation} onChange={(event) => setConfirmation(event.target.value)} disabled={!available} required />
            </div>
          </div>
          <label className="flex items-start gap-2 rounded-md border bg-muted/20 px-3 py-2 text-xs">
            <input type="checkbox" className="mt-0.5" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} disabled={!available} data-testid="vm-password-confirm-check" />
            <span>我已确认虚拟机 <strong className="font-medium text-foreground">{vm.name}</strong> 和客户机用户 <strong className="font-mono font-medium text-foreground">{username.trim() || "-"}</strong>，并了解修改后会立即影响客户机登录。</span>
          </label>
        </form>

        {notice ? (
          <p className={`flex items-start gap-2 rounded-md border px-3 py-2 text-xs ${notice.tone === "success" ? "border-success/30 bg-success/10 text-success" : "border-destructive/30 bg-destructive/5 text-destructive"}`} data-testid="vm-password-notice" role="status">
            {notice.tone === "success" ? <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0" /> : <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />}{notice.text}
          </p>
        ) : null}

        <details className="rounded-md border bg-muted/20 px-3 py-2 text-xs">
          <summary className="cursor-pointer font-medium">密码是如何写入客户机的？</summary>
          <p className="mt-3 text-muted-foreground">密码通过 Guest Agent 直接写入客户机，VirtMgr 不保存密码明文，审计只记录用户名和任务结果。</p>
        </details>
      </div>
    </Modal>
  );
}

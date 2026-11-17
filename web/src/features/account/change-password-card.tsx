import { useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { CheckCircle2, KeyRound, LoaderCircle } from "lucide-react";
import { ApiError, api } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

function actionError(cause: unknown, fallback: string) {
  return cause instanceof ApiError ? cause.message : fallback;
}

export function ChangePasswordCard({ readOnly }: { readOnly: boolean }) {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const mutation = useMutation({
    mutationFn: () => api.changePassword({ currentPassword, newPassword }),
    onSuccess: () => {
      setMessage("密码已更新，其他设备上的登录会话已撤销。");
      setError("");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
    },
    onError: (cause) => {
      setMessage("");
      setError(actionError(cause, "修改密码失败，请稍后重试。"));
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (readOnly) return;
    if (newPassword !== confirmPassword) {
      setError("两次输入的新密码不一致。");
      return;
    }
    if (newPassword === currentPassword) {
      setError("新密码不能与当前密码相同。");
      return;
    }
    setError("");
    setMessage("");
    mutation.mutate();
  }

  return (
    <Card data-testid="change-password-card">
      <CardHeader>
        <CardTitle className="flex items-center gap-2"><KeyRound className="h-4 w-4 text-primary" />修改我的密码</CardTitle>
        <CardDescription>密码至少 12 个字符。修改成功后，当前会话保留，其他设备上的会话将立即失效。</CardDescription>
      </CardHeader>
      <CardContent>
        {readOnly ? <p className="mb-3 rounded-md border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning">系统处于只读模式，密码修改已禁用。</p> : null}
        <form className="grid gap-3 md:grid-cols-[1fr_1fr_1fr_auto]" onSubmit={submit}>
          <div className="space-y-2">
            <Label htmlFor="current-password">当前密码</Label>
            <Input id="current-password" type="password" autoComplete="current-password" value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} required disabled={readOnly} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="new-password">新密码</Label>
            <Input id="new-password" type="password" autoComplete="new-password" minLength={12} value={newPassword} onChange={(event) => setNewPassword(event.target.value)} required disabled={readOnly} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="confirm-password">确认新密码</Label>
            <Input id="confirm-password" type="password" autoComplete="new-password" minLength={12} value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} required disabled={readOnly} />
          </div>
          <div className="flex items-end">
            <Button type="submit" data-testid="change-password-submit" disabled={readOnly || mutation.isPending} className="w-full md:w-auto">
              {mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <KeyRound className="h-4 w-4" />}更新密码
            </Button>
          </div>
          {message ? <p className="flex items-center gap-2 rounded-md border border-success/30 bg-success/5 px-3 py-2 text-xs text-success md:col-span-4" role="status"><CheckCircle2 className="h-4 w-4" />{message}</p> : null}
          {error ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive md:col-span-4" role="alert">{error}</p> : null}
        </form>
      </CardContent>
    </Card>
  );
}

import { useEffect, useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { LogIn, Server, ShieldCheck } from "lucide-react";
import { ApiError, api } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { sessionQueryKey, useSessionQuery } from "@/features/auth/use-session";

export function LoginPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const session = useSessionQuery();
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (session.data) void navigate({ to: "/", replace: true });
  }, [navigate, session.data]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      const result = await api.login(username, password);
      queryClient.setQueryData(sessionQueryKey, result);
      await navigate({ to: "/", replace: true });
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : "无法连接 VirtMgr 服务");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="grid min-h-screen bg-background lg:grid-cols-[1.1fr_0.9fr]">
      <section className="relative hidden overflow-hidden border-r bg-card p-12 lg:flex lg:flex-col lg:justify-between">
        <div className="flex items-center gap-3">
          <div className="grid h-10 w-10 place-items-center rounded-xl bg-primary text-lg font-bold text-primary-foreground shadow-sm">V</div>
          <div>
            <p className="font-semibold">VirtMgr Console</p>
            <p className="text-[10px] uppercase tracking-[0.18em] text-muted-foreground">Libvirt Operations</p>
          </div>
        </div>
        <div className="max-w-lg">
          <p className="text-xs font-semibold uppercase tracking-[0.2em] text-primary">KVM / libvirt</p>
          <h1 className="mt-4 text-4xl font-semibold tracking-tight">在一处管理物理机上的全部虚拟机。</h1>
          <p className="mt-4 text-sm leading-6 text-muted-foreground">查看宿主机资源、虚拟机状态和基础设施概况。所有数据直接来自 libvirt，不维护第二份虚拟机状态。</p>
        </div>
        <div className="grid max-w-lg grid-cols-2 gap-3 text-sm">
          <div className="rounded-lg border bg-background/60 p-4"><Server className="mb-2 h-4 w-4 text-primary" />libvirt 原生读取</div>
          <div className="rounded-lg border bg-background/60 p-4"><ShieldCheck className="mb-2 h-4 w-4 text-success" />默认只读启动</div>
        </div>
      </section>
      <main className="grid place-items-center p-6">
        <Card className="w-full max-w-md">
          <CardHeader>
            <div className="mb-5 flex items-center gap-3 lg:hidden">
              <div className="grid h-9 w-9 place-items-center rounded-lg bg-primary font-bold text-primary-foreground">V</div>
              <span className="font-semibold">VirtMgr Console</span>
            </div>
            <CardTitle className="text-xl">登录 VirtMgr</CardTitle>
            <CardDescription>使用本地账号凭据进入虚拟机控制台。</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={handleSubmit}>
              <div className="space-y-2">
                <Label htmlFor="username">用户名</Label>
                <Input id="username" name="username" autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} required />
              </div>
              <div className="space-y-2">
                <Label htmlFor="password">密码</Label>
                <Input id="password" name="password" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} required autoFocus />
              </div>
              {error ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert">{error}</p> : null}
              <Button type="submit" className="w-full" disabled={submitting}>
                <LogIn className="h-4 w-4" />{submitting ? "正在登录" : "登录"}
              </Button>
            </form>
          </CardContent>
        </Card>
      </main>
    </div>
  );
}

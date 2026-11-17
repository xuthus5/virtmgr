import { useEffect } from "react";
import { Link } from "@tanstack/react-router";
import { ArrowLeft, KeyRound, Terminal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

const recoveryCommand = "sudo -u virtmgr /usr/local/bin/virtmgr reset-password --username <用户名> --config /etc/virtmgr/server.yaml";

export function ForgotPasswordPage() {
  useEffect(() => {
    document.title = "找回密码 · VirtMgr";
  }, []);

  return (
    <div className="grid min-h-screen place-items-center bg-background p-6">
      <Card className="w-full max-w-2xl">
        <CardHeader>
          <div className="mb-3 grid h-10 w-10 place-items-center rounded-xl bg-primary/10 text-primary"><KeyRound className="h-5 w-5" /></div>
          <CardTitle className="text-xl">找回本地账号密码</CardTitle>
          <CardDescription>VirtMgr 单机版不依赖邮件或外部身份服务，密码恢复需要在宿主机上执行命令。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5 text-sm">
          <div className="space-y-2">
            <p className="font-medium">1. 登录 VirtMgr 所在宿主机</p>
            <p className="text-muted-foreground">使用具有 sudo 权限的 SSH 账号或本地终端。Docker 部署时请在容器所在宿主机执行对应容器的命令。</p>
          </div>
          <div className="space-y-2">
            <p className="flex items-center gap-2 font-medium"><Terminal className="h-4 w-4 text-primary" />2. 重置指定账号密码</p>
            <pre className="overflow-x-auto rounded-lg border bg-muted/50 p-3 text-xs"><code>{recoveryCommand}</code></pre>
            <p className="text-xs text-muted-foreground">命令会提示输入两次新密码，并撤销该账号的全部登录会话。密码不会写入审计日志。</p>
          </div>
          <div className="space-y-2 rounded-lg border border-warning/30 bg-warning/10 p-3 text-xs text-warning">
            <p className="font-medium">请勿通过公开网络提供密码重置接口</p>
            <p>本地命令行恢复依赖宿主机文件权限进行授权，适合单机部署，也能避免未认证的远程重置。</p>
          </div>
          <Button asChild variant="outline"><Link to="/login"><ArrowLeft className="h-4 w-4" />返回登录</Link></Button>
        </CardContent>
      </Card>
    </div>
  );
}

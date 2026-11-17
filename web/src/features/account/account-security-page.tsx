import { Link } from "@tanstack/react-router";
import { ShieldCheck, UserRound } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { LoadingState } from "@/components/feedback/states";
import { PageHeader } from "@/components/layout/page-header";
import { useSessionQuery } from "@/features/auth/use-session";
import { ChangePasswordCard } from "@/features/account/change-password-card";

const roleLabels = { admin: "管理员", operator: "运维人员", viewer: "只读用户" } as const;

export function AccountSecurityPage() {
  const session = useSessionQuery();
  if (!session.data) return <LoadingState label="正在读取账号信息" />;
  const user = session.data.user;

  return (
    <div data-testid="account-security-page">
      <PageHeader
        title="账号安全"
        breadcrumb="总览 / 账号安全"
        description="维护当前登录账号的密码。修改密码无需进入用户管理，也不会影响其他用户。"
      />
      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
        <ChangePasswordCard readOnly={Boolean(session.data.readOnly)} />
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><UserRound className="h-4 w-4 text-primary" />当前账号</CardTitle>
            <CardDescription>此账号用于登录 VirtMgr 本地控制台。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4 text-sm">
            <div>
              <p className="text-xs text-muted-foreground">用户名</p>
              <p className="mt-1 font-medium">{user.username}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">角色</p>
              <Badge className="mt-1" variant={user.role === "admin" ? "success" : user.role === "operator" ? "warning" : "default"}>
                <ShieldCheck className="h-3 w-3" />{roleLabels[user.role]}
              </Badge>
            </div>
            {user.role === "admin" ? (
              <p className="border-t pt-4 text-xs text-muted-foreground">
                需要管理其他账号时，可前往 <Link to="/users" className="font-medium text-primary underline-offset-4 hover:underline">用户管理</Link>。
              </p>
            ) : (
              <p className="border-t pt-4 text-xs text-muted-foreground">如需重置其他用户或管理角色，请联系管理员。</p>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

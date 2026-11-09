import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Ban, CheckCircle2, LoaderCircle, ShieldAlert, ShieldCheck, UserPlus, Users } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { Role, User, UserCreateInput } from "@/api/types";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useSessionQuery } from "@/features/auth/use-session";
import { useUsersQuery, usersQueryKey } from "@/features/users/use-users-query";

type CreateFormState = {
  username: string;
  password: string;
  role: Role;
};

const initialCreateForm: CreateFormState = { username: "", password: "", role: "viewer" };

const createdFormatter = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

function formatCreated(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : createdFormatter.format(date);
}

function actionError(cause: unknown, fallback: string) {
  return cause instanceof ApiError ? cause.message : fallback;
}

function roleBadgeVariant(role: Role): "default" | "warning" | "success" {
  if (role === "admin") return "success";
  if (role === "operator") return "warning";
  return "default";
}

export function UsersPage() {
  const session = useSessionQuery();
  const queryClient = useQueryClient();
  const isAdmin = session.data?.user.role === "admin";
  const readOnly = Boolean(session.data?.readOnly);
  const usersQuery = useUsersQuery(isAdmin);
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState(initialCreateForm);
  const [createError, setCreateError] = useState("");
  const [actionMessage, setActionMessage] = useState("");
  const [actionErrorText, setActionErrorText] = useState("");

  const createMutation = useMutation({
    mutationFn: (input: UserCreateInput) => api.createUser(input),
    onSuccess: (user) => {
      setActionMessage(`用户 ${user.username} 已创建。`);
      setCreateError("");
      setCreateOpen(false);
      setCreateForm(initialCreateForm);
      void queryClient.invalidateQueries({ queryKey: usersQueryKey });
    },
    onError: (cause) => {
      setCreateError(actionError(cause, "创建用户失败，请稍后重试。"));
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ user, role, disabled }: { user: User; role: Role; disabled: boolean }) => api.updateUser(user.id, { role, disabled }),
    onSuccess: (user) => {
      setActionMessage(`用户 ${user.username} 已更新。`);
      setActionErrorText("");
      void queryClient.invalidateQueries({ queryKey: usersQueryKey });
    },
    onError: (cause) => {
      setActionErrorText(actionError(cause, "更新用户失败，请稍后重试。"));
    },
  });

  if (!session.data) return <LoadingState label="正在确认访问权限" />;
  if (!isAdmin) {
    return (
      <>
        <PageHeader title="用户管理" breadcrumb="总览 / 用户管理" description="维护本地账号及其角色和启用状态。" />
        <Card>
          <CardContent className="flex flex-col items-center py-14 text-center">
            <ShieldAlert className="h-8 w-8 text-warning" />
            <p className="mt-4 font-medium">当前账号无权查看用户管理</p>
            <p className="mt-1 text-sm text-muted-foreground">用户管理仅对管理员开放。</p>
          </CardContent>
        </Card>
      </>
    );
  }

  const users = usersQuery.data?.items ?? [];
  const currentUserID = session.data.user.id;

  function submitCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setCreateError("");
    setActionMessage("");
    createMutation.mutate(createForm);
  }

  function updateUser(user: User, role: Role, disabled: boolean) {
    setActionMessage("");
    setActionErrorText("");
    updateMutation.mutate({ user, role, disabled });
  }

  return (
    <>
      <PageHeader
        title="用户管理"
        breadcrumb="总览 / 用户管理"
        description="维护本地账号、角色和启用状态。角色变更会在下一次请求时立即生效。"
        actions={(
          <Button data-testid="user-create-toggle" disabled={readOnly} title={readOnly ? "系统当前处于只读模式" : undefined} onClick={() => { setCreateOpen((open) => !open); setCreateError(""); }}>
            <UserPlus className="h-4 w-4" />新建用户
          </Button>
        )}
      />

      {actionMessage ? <div className="mb-4 flex items-center gap-2 rounded-md border border-success/30 bg-success/5 px-3 py-2 text-xs text-success" data-testid="user-action-success"><CheckCircle2 className="h-4 w-4" />{actionMessage}</div> : null}
      {actionErrorText ? <div className="mb-4 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert">{actionErrorText}</div> : null}

      {createOpen ? (
        <Card className="mb-4" data-testid="user-create-form">
          <CardHeader>
            <CardTitle>新建本地用户</CardTitle>
            <CardDescription>用户名允许字母、数字、点、下划线和短横线；初始密码至少 12 个字符。</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="grid gap-4 md:grid-cols-[1fr_1fr_180px_auto]" onSubmit={submitCreate}>
              <div className="space-y-2">
                <Label htmlFor="new-user-username">用户名</Label>
                <Input id="new-user-username" autoComplete="off" value={createForm.username} onChange={(event) => setCreateForm((form) => ({ ...form, username: event.target.value }))} required />
              </div>
              <div className="space-y-2">
                <Label htmlFor="new-user-password">初始密码</Label>
                <Input id="new-user-password" type="password" autoComplete="new-password" minLength={12} value={createForm.password} onChange={(event) => setCreateForm((form) => ({ ...form, password: event.target.value }))} required />
              </div>
              <div className="space-y-2">
                <Label htmlFor="new-user-role">角色</Label>
                <select id="new-user-role" value={createForm.role} onChange={(event) => setCreateForm((form) => ({ ...form, role: event.target.value as Role }))} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                  <option value="viewer">只读用户</option>
                  <option value="operator">运维人员</option>
                  <option value="admin">管理员</option>
                </select>
              </div>
              <div className="flex items-end">
                <Button type="submit" data-testid="user-create-submit" disabled={createMutation.isPending} className="w-full md:w-auto">
                  {createMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <UserPlus className="h-4 w-4" />}创建用户
                </Button>
              </div>
              {createError ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive md:col-span-4" role="alert">{createError}</p> : null}
            </form>
          </CardContent>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>本地账号</CardTitle>
          <CardDescription>共 <span data-testid="users-total">{usersQuery.data?.total ?? 0}</span> 个账号。当前登录账号不能在这里修改自身角色或状态。</CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {readOnly ? <p className="border-b bg-muted/40 px-5 py-2 text-xs text-muted-foreground">系统处于只读模式，账号写操作已禁用。</p> : null}
          {usersQuery.isLoading ? <LoadingState label="正在读取本地用户" /> : null}
          {usersQuery.isError ? <ErrorState message={usersQuery.error instanceof Error ? usersQuery.error.message : "无法读取本地用户"} onRetry={() => void usersQuery.refetch()} className="m-4" /> : null}
          {usersQuery.data ? (
            <Table className="min-w-[760px]">
              <TableHeader><TableRow><TableHead>用户名</TableHead><TableHead>角色</TableHead><TableHead>状态</TableHead><TableHead>创建时间</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
              <TableBody>
                {users.map((user) => {
                  const self = user.id === currentUserID;
                  const controlsDisabled = readOnly || self || updateMutation.isPending;
                  const disabledReason = self ? "不能修改当前登录账号" : readOnly ? "系统当前处于只读模式" : undefined;
                  return (
                    <TableRow key={user.id} data-testid={`user-row-${user.username}`}>
                      <TableCell>
                        <div className="flex items-center gap-2"><Users className="h-4 w-4 text-muted-foreground" /><span className="font-medium">{user.username}</span>{self ? <Badge variant="outline">当前账号</Badge> : null}</div>
                        <p className="mt-1 font-mono text-[10px] text-muted-foreground">ID {user.id}</p>
                      </TableCell>
                      <TableCell>
                        <select aria-label={`修改 ${user.username} 的角色`} value={user.role} disabled={controlsDisabled} title={disabledReason} onChange={(event) => updateUser(user, event.target.value as Role, user.disabled)} className="h-8 rounded-md border border-input bg-card px-2 text-xs shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60">
                          <option value="viewer">只读用户</option>
                          <option value="operator">运维人员</option>
                          <option value="admin">管理员</option>
                        </select>
                      </TableCell>
                      <TableCell><Badge variant={user.disabled ? "secondary" : "success"}>{user.disabled ? <Ban className="h-3 w-3" /> : <CheckCircle2 className="h-3 w-3" />}{user.disabled ? "已禁用" : "已启用"}</Badge></TableCell>
                      <TableCell className="whitespace-nowrap text-xs text-muted-foreground">{formatCreated(user.createdAt)}</TableCell>
                      <TableCell className="text-right">
                        <Button variant="outline" size="sm" data-testid={`user-toggle-${user.username}`} disabled={controlsDisabled} title={disabledReason} onClick={() => updateUser(user, user.role, !user.disabled)}>
                          {user.disabled ? <CheckCircle2 className="h-3.5 w-3.5" /> : <Ban className="h-3.5 w-3.5" />}{user.disabled ? "启用" : "禁用"}
                        </Button>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          ) : null}
        </CardContent>
      </Card>

      <div className="mt-4 flex flex-wrap gap-2 text-xs text-muted-foreground">
        <Badge variant={roleBadgeVariant("viewer")}>只读用户：查看资源</Badge>
        <Badge variant={roleBadgeVariant("operator")}>运维人员：生命周期与存储管理</Badge>
        <Badge variant={roleBadgeVariant("admin")}><ShieldCheck className="h-3 w-3" />管理员：全部操作与用户管理</Badge>
      </div>
    </>
  );
}

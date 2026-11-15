import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, Copy, ExternalLink, KeyRound, LoaderCircle, Plus, ShieldAlert, Trash2 } from "lucide-react";
import { ApiError, api, openapiUrl } from "@/api/client";
import type { APIToken, APITokenCreated, APITokenScope } from "@/api/types";
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { apiTokensQueryKey, useAPITokensQuery } from "@/features/settings/use-api-tokens-query";

const dateFormatter = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false,
});

function formatDate(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : dateFormatter.format(date);
}

function tokenState(token: APIToken) {
  if (token.revokedAt) return { label: "已撤销", variant: "secondary" as const };
  if (token.expired) return { label: "已过期", variant: "destructive" as const };
  return { label: "有效", variant: "success" as const };
}

function tokenError(cause: unknown, fallback: string) {
  return cause instanceof ApiError ? cause.message : fallback;
}

export function APITokensCard() {
  const queryClient = useQueryClient();
  const query = useAPITokensQuery();
  const [formOpen, setFormOpen] = useState(false);
  const [name, setName] = useState("");
  const [scope, setScope] = useState<APITokenScope>("read");
  const [expiresInDays, setExpiresInDays] = useState("90");
  const [created, setCreated] = useState<APITokenCreated | null>(null);
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [copied, setCopied] = useState(false);
  const [revokeTarget, setRevokeTarget] = useState<APIToken | null>(null);

  const createMutation = useMutation({
    mutationFn: api.createAPIToken,
    onSuccess: (result) => {
      setCreated(result);
      setCopied(false);
      setFormOpen(false);
      setName("");
      setExpiresInDays("90");
      setNotice({ tone: "success", text: `API Token ${result.apiToken.name} 已创建。明文只显示一次。` });
      void queryClient.invalidateQueries({ queryKey: apiTokensQueryKey });
    },
    onError: (cause) => setNotice({ tone: "error", text: tokenError(cause, "创建 API Token 失败，请稍后重试。") }),
  });

  const revokeMutation = useMutation({
    mutationFn: (token: APIToken) => api.revokeAPIToken(token.id),
    onSuccess: (_, token) => {
      setRevokeTarget(null);
      setNotice({ tone: "success", text: `API Token ${token.name} 已撤销，立即停止认证。` });
      void queryClient.invalidateQueries({ queryKey: apiTokensQueryKey });
    },
    onError: (cause) => setNotice({ tone: "error", text: tokenError(cause, "撤销 API Token 失败，请稍后重试。") }),
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setNotice(null);
    createMutation.mutate({ name: name.trim(), scopes: [scope], expiresInDays: Number(expiresInDays) });
  }

  async function copyToken() {
    if (!created) return;
    try {
      await navigator.clipboard.writeText(created.token);
      setCopied(true);
    } catch {
      setNotice({ tone: "error", text: "浏览器未允许自动复制，请手动选中 Token 文本复制。" });
    }
  }

  const tokens = query.data?.items ?? [];

  return (
    <Card data-testid="api-token-card">
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2"><KeyRound className="h-4 w-4 text-primary" />API Token</CardTitle>
            <CardDescription>为脚本和自动化客户端签发当前账号的 Bearer Token。read 仅允许 GET，write 同时包含 read。</CardDescription>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button asChild variant="outline" size="sm"><a href={openapiUrl} target="_blank" rel="noreferrer"><ExternalLink className="h-4 w-4" />OpenAPI</a></Button>
            <Button data-testid="api-token-create-toggle" size="sm" onClick={() => { setFormOpen((current) => !current); setCreated(null); setNotice(null); }}><Plus className="h-4 w-4" />新建 Token</Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-start gap-2 rounded-md border border-warning/35 bg-warning/10 px-3 py-2 text-xs text-warning">
          <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" />
          <p>Token 只保存在当前用户范围内，数据库仅保存 SHA-256 哈希。请像密码一样保管明文，泄露后立即撤销。</p>
        </div>

        {created ? (
          <div className="space-y-2 rounded-md border border-success/35 bg-success/5 p-3" data-testid="api-token-created-panel">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-sm font-medium">请立即保存 {created.apiToken.name} 的明文 Token</p>
              <Badge variant="warning">只显示一次</Badge>
            </div>
            <div className="flex flex-col gap-2 sm:flex-row">
              <code data-testid="api-token-secret" className="min-w-0 flex-1 overflow-x-auto rounded-md border bg-background px-3 py-2 font-mono text-xs">{created.token}</code>
              <Button type="button" variant="outline" data-testid="api-token-copy" onClick={() => void copyToken()}>{copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}{copied ? "已复制" : "复制"}</Button>
            </div>
          </div>
        ) : null}

        {notice ? <p role="status" data-testid="api-token-notice" className={notice.tone === "success" ? "rounded-md border border-success/30 bg-success/5 px-3 py-2 text-sm text-success" : "rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive"}>{notice.text}</p> : null}

        {formOpen ? (
          <form onSubmit={submit} className="grid gap-3 rounded-lg border bg-muted/20 p-4 md:grid-cols-[1.2fr_0.8fr_0.6fr_auto] md:items-end" data-testid="api-token-form">
            <div className="space-y-2">
              <Label htmlFor="api-token-name">名称</Label>
              <Input id="api-token-name" data-testid="api-token-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="ci-deploy" maxLength={64} required />
            </div>
            <div className="space-y-2">
              <Label htmlFor="api-token-scope">作用域</Label>
              <select id="api-token-scope" data-testid="api-token-scope" value={scope} onChange={(event) => setScope(event.target.value as APITokenScope)} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">
                <option value="read">read · 只读 API</option>
                <option value="write">write · 读写 API</option>
              </select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="api-token-expiry">有效期（天）</Label>
              <Input id="api-token-expiry" data-testid="api-token-expiry" type="number" min={0} max={3650} value={expiresInDays} onChange={(event) => setExpiresInDays(event.target.value)} />
            </div>
            <Button type="submit" data-testid="api-token-submit" disabled={createMutation.isPending || !name.trim()}>
              {createMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}签发
            </Button>
          </form>
        ) : null}

        {query.isLoading ? <p className="text-sm text-muted-foreground">正在读取 API Token…</p> : null}
        {query.isError ? <p className="text-sm text-destructive">{query.error instanceof Error ? query.error.message : "无法读取 API Token"}</p> : null}
        {!query.isLoading && !query.isError && tokens.length === 0 ? <p className="rounded-md border border-dashed px-4 py-8 text-center text-sm text-muted-foreground" data-testid="api-token-empty">当前账号还没有 API Token。</p> : null}
        {tokens.length ? (
          <Table className="min-w-[820px]" containerProps={{ "data-testid": "api-token-table" }}>
            <TableHeader><TableRow><TableHead>名称</TableHead><TableHead>作用域</TableHead><TableHead>状态</TableHead><TableHead>创建 / 过期</TableHead><TableHead>最近使用</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
            <TableBody>
              {tokens.map((token) => {
                const state = tokenState(token);
                return (
                  <TableRow key={token.id} data-testid={`api-token-row-${token.id}`}>
                    <TableCell><p className="font-medium">{token.name}</p><p className="max-w-56 truncate font-mono text-[10px] text-muted-foreground">{token.id}</p></TableCell>
                    <TableCell><div className="flex flex-wrap gap-1">{token.scopes.map((item) => <Badge key={item} variant={item === "write" ? "warning" : "outline"}>{item}</Badge>)}</div></TableCell>
                    <TableCell><Badge variant={state.variant}>{state.label}</Badge></TableCell>
                    <TableCell className="whitespace-nowrap text-xs"><p>{formatDate(token.createdAt)}</p><p className="text-muted-foreground">{token.expiresAt ? `至 ${formatDate(token.expiresAt)}` : "永不过期"}</p></TableCell>
                    <TableCell className="whitespace-nowrap text-xs text-muted-foreground">{formatDate(token.lastUsedAt)}</TableCell>
                    <TableCell className="text-right"><Button variant="outline" size="sm" data-testid={`api-token-revoke-${token.id}`} disabled={Boolean(token.revokedAt) || revokeMutation.isPending} onClick={() => setRevokeTarget(token)}><Trash2 className="h-3.5 w-3.5" />撤销</Button></TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        ) : null}
      </CardContent>

      <AlertDialog open={Boolean(revokeTarget)} onOpenChange={(open) => { if (!open) setRevokeTarget(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>撤销 API Token？</AlertDialogTitle>
            <AlertDialogDescription>撤销后，使用 {revokeTarget?.name} 的所有客户端会在下一次请求时立即失去认证，无法恢复。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>保留 Token</AlertDialogCancel>
            <Button variant="destructive" data-testid="confirm-revoke-api-token" disabled={!revokeTarget || revokeMutation.isPending} onClick={() => { if (revokeTarget) revokeMutation.mutate(revokeTarget); }}>
              {revokeMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}确认撤销
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}

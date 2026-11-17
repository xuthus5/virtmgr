import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Cloud, FileText, KeyRound, LoaderCircle, Server, ShieldAlert, Terminal, Users } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type {
  GuestAgentAuthorizedKeys,
  GuestAgentCloudInit,
  GuestAgentFileContent,
  GuestAgentHealthHistory,
  GuestAgentPolicy,
  GuestAgentProcess,
  GuestAgentProcessSignalInput,
  GuestAgentService,
  GuestAgentUser,
} from "@/api/types";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { formatBytes } from "@/lib/utils";

type PendingAction =
  | { kind: "file-write" }
  | { kind: "ssh-add" }
  | { kind: "ssh-remove" }
  | { kind: "signal"; process: GuestAgentProcess };

function encodeText(value: string) {
  const bytes = new TextEncoder().encode(value);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function decodeText(value: string) {
  try {
    const binary = atob(value);
    const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
    const text = new TextDecoder().decode(bytes);
    return /[\u0000-\u0008\u000B\u000C\u000E-\u001F]/.test(text) ? "" : text;
  } catch {
    return "";
  }
}

function splitKeys(value: string) {
  return value.split("\n").map((line) => line.trim()).filter(Boolean);
}

function errorMessage(cause: unknown, fallback: string) {
  return cause instanceof ApiError ? cause.message : fallback;
}

export function GuestAgentControls({ uuid, canMutate, disabledReason, notify }: {
  uuid: string;
  canMutate: boolean;
  disabledReason: string;
  notify: (tone: "success" | "error", text: string) => void;
}) {
  const [filePath, setFilePath] = useState("/etc/os-release");
  const [fileContent, setFileContent] = useState<GuestAgentFileContent | null>(null);
  const [writePath, setWritePath] = useState("/etc/motd");
  const [writeText, setWriteText] = useState("");
  const [overwrite, setOverwrite] = useState(false);
  const [username, setUsername] = useState("root");
  const [keys, setKeys] = useState("");
  const [authorizedKeys, setAuthorizedKeys] = useState<GuestAgentAuthorizedKeys | null>(null);
  const [processes, setProcesses] = useState<GuestAgentProcess[]>([]);
  const [services, setServices] = useState<GuestAgentService[]>([]);
  const [cloudInit, setCloudInit] = useState<GuestAgentCloudInit | null>(null);
  const [users, setUsers] = useState<GuestAgentUser[]>([]);
  const [policy, setPolicy] = useState<GuestAgentPolicy | null>(null);
  const [health, setHealth] = useState<GuestAgentHealthHistory | null>(null);
  const [pending, setPending] = useState<PendingAction | null>(null);

  const readFileMutation = useMutation({
    mutationFn: () => api.guestAgentFile(uuid, filePath.trim(), 0, 256 * 1024),
    onSuccess: (content) => { setFileContent(content); notify("success", "文件读取完成，操作已审计。"); },
    onError: (cause) => notify("error", errorMessage(cause, "读取客户机文件失败。")),
  });
  const writeFileMutation = useMutation({
    mutationFn: () => api.writeGuestAgentFile(uuid, {
      path: writePath.trim(), dataBase64: encodeText(writeText), mode: "replace", overwrite,
    }),
    onSuccess: (response) => {
      setPending(null);
      notify("success", `已写入 ${formatBytes(response.result.bytesWritten)}，任务 ${response.task.id} 已完成。`);
    },
    onError: (cause) => { setPending(null); notify("error", errorMessage(cause, "写入客户机文件失败。")); },
  });
  const loadKeysMutation = useMutation({
    mutationFn: () => api.guestAgentSSHKeys(uuid, username.trim()),
    onSuccess: (result) => { setAuthorizedKeys(result); notify("success", `已读取 ${result.keys.length} 个公钥。`); },
    onError: (cause) => notify("error", errorMessage(cause, "读取 SSH 公钥失败。")),
  });
  const addKeysMutation = useMutation({
    mutationFn: () => api.addGuestAgentSSHKeys(uuid, { username: username.trim(), keys: splitKeys(keys) }),
    onSuccess: () => { setPending(null); setKeys(""); notify("success", "SSH 公钥已添加；审计只记录指纹与数量。"); },
    onError: (cause) => { setPending(null); notify("error", errorMessage(cause, "添加 SSH 公钥失败。")); },
  });
  const removeKeysMutation = useMutation({
    mutationFn: () => api.removeGuestAgentSSHKeys(uuid, { username: username.trim(), keys: splitKeys(keys) }),
    onSuccess: () => { setPending(null); setKeys(""); notify("success", "SSH 公钥已移除。"); },
    onError: (cause) => { setPending(null); notify("error", errorMessage(cause, "移除 SSH 公钥失败。")); },
  });
  const processesMutation = useMutation({
    mutationFn: (announce: boolean) => api.guestAgentProcesses(uuid).then((result) => ({ result, announce })),
    onSuccess: ({ result, announce }) => {
      setProcesses(result.items);
      if (announce) notify("success", `已读取 ${result.total} 个进程。`);
    },
    onError: (cause) => notify("error", errorMessage(cause, "读取进程失败。")),
  });
  const signalMutation = useMutation({
    mutationFn: (input: GuestAgentProcessSignalInput) => api.signalGuestAgentProcess(uuid, input),
    onSuccess: (task) => { setPending(null); notify("success", `进程信号任务 ${task.id} 已完成。`); void processesMutation.mutate(false); },
    onError: (cause) => { setPending(null); notify("error", errorMessage(cause, "发送进程信号失败。")); },
  });
  const servicesMutation = useMutation({
    mutationFn: () => api.guestAgentServices(uuid),
    onSuccess: (result) => { setServices(result.items); notify("success", `已读取 ${result.total} 个 systemd 服务。`); },
    onError: (cause) => notify("error", errorMessage(cause, "读取客户机服务失败。")),
  });
  const cloudInitMutation = useMutation({
    mutationFn: () => api.guestAgentCloudInit(uuid),
    onSuccess: (result) => { setCloudInit(result); notify("success", result.installed ? `Cloud-init 状态已读取：${result.state}` : "未检测到 cloud-init，或客户机无法执行固定诊断命令。"); },
    onError: (cause) => notify("error", errorMessage(cause, "读取 Cloud-init 状态失败。")),
  });
  const usersMutation = useMutation({
    mutationFn: () => api.guestAgentUsers(uuid),
    onSuccess: (result) => { setUsers(result.items); notify("success", `已读取 ${result.total} 个登录用户。`); },
    onError: (cause) => notify("error", errorMessage(cause, "读取登录用户失败。")),
  });
  const policyMutation = useMutation({
    mutationFn: () => Promise.all([api.guestAgentPolicy(uuid), api.guestAgentHealth(uuid)]),
    onSuccess: ([policyResult, healthResult]) => { setPolicy(policyResult); setHealth(healthResult); notify("success", "策略与健康历史已刷新。"); },
    onError: (cause) => notify("error", errorMessage(cause, "读取策略或健康历史失败。")),
  });
  const updatePolicyMutation = useMutation({
    mutationFn: (expected: boolean) => api.updateGuestAgentPolicy(uuid, { expected }),
    onSuccess: (updated) => { setPolicy(updated); notify("success", updated.expected ? "已启用 Guest Agent 期望状态监控。" : "已停用 Guest Agent 期望状态监控。"); },
    onError: (cause) => notify("error", errorMessage(cause, "更新策略失败。")),
  });

  function confirmPending() {
    if (!pending) return;
    switch (pending.kind) {
      case "file-write": writeFileMutation.mutate(); break;
      case "ssh-add": addKeysMutation.mutate(); break;
      case "ssh-remove": removeKeysMutation.mutate(); break;
      case "signal": signalMutation.mutate({ pid: pending.process.pid, signal: "term" }); break;
    }
  }

  if (!canMutate) {
    return <p className="rounded-md border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">{disabledReason || "当前账号无权访问客户机敏感操作。"}</p>;
  }

  return (
    <div className="space-y-4 border-t pt-4" data-testid="guest-agent-controls">
      <div className="flex items-start gap-2 rounded-md border border-warning/30 bg-warning/5 px-3 py-2 text-xs text-warning">
        <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" />
        高风险能力：不提供任意命令执行。文件、密码、SSH 公钥与进程动作全部经过固定接口、权限、CSRF、只读校验和审计脱敏。
      </div>

      <section className="space-y-3 rounded-md border p-3" aria-labelledby="guest-files-heading">
        <div className="flex items-center gap-2"><FileText className="h-4 w-4" /><h3 id="guest-files-heading" className="text-sm font-semibold">文件读写</h3><Badge variant="outline">单次上限 1 MiB</Badge></div>
        <div className="grid gap-2 sm:grid-cols-[1fr_auto]">
          <Input aria-label="客户机文件路径" value={filePath} onChange={(event) => setFilePath(event.target.value)} placeholder="/etc/os-release" />
          <Button variant="outline" data-testid="guest-agent-file-read" onClick={() => readFileMutation.mutate()} disabled={readFileMutation.isPending || !filePath.trim()}>
            {readFileMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : null}读取
          </Button>
        </div>
        {fileContent ? (
          <div className="space-y-2 text-xs">
            <div className="flex flex-wrap gap-2 text-muted-foreground"><span>{formatBytes(fileContent.size)}</span><span className="font-mono">{fileContent.sha256}</span>{fileContent.eof ? <Badge variant="secondary">EOF</Badge> : null}</div>
            <pre data-testid="guest-agent-file-preview" className="max-h-48 overflow-auto rounded-md bg-muted/40 p-2 font-mono text-[11px] whitespace-pre-wrap">{decodeText(fileContent.dataBase64) || "内容为二进制或不可安全展示，请通过 API 获取 Base64。"}</pre>
          </div>
        ) : null}
        <div className="grid gap-2 sm:grid-cols-2">
          <div className="space-y-1"><Label htmlFor="guest-write-path">写入路径</Label><Input id="guest-write-path" value={writePath} onChange={(event) => setWritePath(event.target.value)} /></div>
          <label className="flex items-end gap-2 pb-2 text-xs"><input type="checkbox" checked={overwrite} onChange={(event) => setOverwrite(event.target.checked)} />确认覆盖已有文件</label>
        </div>
        <textarea aria-label="写入文件内容" value={writeText} onChange={(event) => setWriteText(event.target.value)} className="min-h-24 w-full rounded-md border bg-card p-2 font-mono text-xs" placeholder="写入内容（UTF-8）" />
        <Button variant="destructive" size="sm" disabled={!writePath.trim() || !overwrite} onClick={() => setPending({ kind: "file-write" })}>确认写入文件</Button>
      </section>

      <section className="space-y-2 rounded-md border p-3" aria-labelledby="guest-ssh-heading">
        <div className="flex items-center gap-2"><KeyRound className="h-4 w-4" /><h3 id="guest-ssh-heading" className="text-sm font-semibold">SSH authorized_keys</h3></div>
        <div className="flex gap-2"><Input aria-label="SSH 用户名" value={username} onChange={(event) => setUsername(event.target.value)} /><Button variant="outline" onClick={() => loadKeysMutation.mutate()} disabled={loadKeysMutation.isPending || !username.trim()}>读取</Button></div>
        {authorizedKeys ? <p className="text-xs text-muted-foreground">当前用户 <span className="font-mono">{authorizedKeys.username}</span> 有 {authorizedKeys.keys.length} 个公钥。</p> : null}
        <textarea aria-label="SSH 公钥" value={keys} onChange={(event) => setKeys(event.target.value)} className="min-h-20 w-full rounded-md border bg-card p-2 font-mono text-xs" placeholder="每行一个 OpenSSH 公钥" />
        <div className="flex flex-wrap gap-2"><Button size="sm" disabled={!keys.trim()} onClick={() => setPending({ kind: "ssh-add" })}>添加公钥</Button><Button size="sm" variant="outline" disabled={!keys.trim()} onClick={() => setPending({ kind: "ssh-remove" })}>移除公钥</Button></div>
      </section>

      <section className="grid gap-4 rounded-md border p-3 lg:grid-cols-2" aria-labelledby="guest-process-heading">
        <div className="space-y-2">
          <div className="flex items-center justify-between gap-2"><div className="flex items-center gap-2"><Terminal className="h-4 w-4" /><h3 id="guest-process-heading" className="text-sm font-semibold">进程</h3></div><Button variant="outline" size="sm" data-testid="guest-agent-process-load" onClick={() => processesMutation.mutate(true)} disabled={processesMutation.isPending}>读取</Button></div>
          <div className="max-h-56 divide-y overflow-auto rounded-md border text-xs">
            {processes.length ? processes.map((process) => (
              <div key={process.pid} data-testid={`guest-agent-process-${process.pid}`} className="flex items-center gap-2 px-2 py-1.5">
                <span className="w-14 font-mono">{process.pid}</span><span className="min-w-0 flex-1 truncate" title={`${process.command} ${process.args ?? ""}`}>{process.command} <span className="text-muted-foreground">{process.args}</span></span>
                <Button variant="destructive" size="sm" data-testid={`guest-agent-process-signal-${process.pid}`} disabled={process.pid <= 1} onClick={() => setPending({ kind: "signal", process })}>终止</Button>
              </div>
            )) : <p className="p-3 text-muted-foreground">尚未读取进程。PID 1 不允许终止，信号固定为 SIGTERM。</p>}
          </div>
        </div>
        <div className="space-y-2">
          <div className="flex items-center justify-between gap-2"><div className="flex items-center gap-2"><Users className="h-4 w-4" /><h3 className="text-sm font-semibold">登录用户</h3></div><Button variant="outline" size="sm" onClick={() => usersMutation.mutate()} disabled={usersMutation.isPending}>读取</Button></div>
          <div className="max-h-56 divide-y overflow-auto rounded-md border text-xs">
            {users.length ? users.map((user) => <div key={`${user.username}-${user.domain}-${user.loginTime}`} className="flex items-center justify-between gap-2 px-2 py-1.5"><span className="font-mono">{user.username}{user.domain ? `@${user.domain}` : ""}</span><span className="text-muted-foreground">{user.loginTime ? new Date(user.loginTime * 1000).toLocaleString("zh-CN") : "-"}</span></div>) : <p className="p-3 text-muted-foreground">尚未读取登录用户。</p>}
          </div>
        </div>
      </section>

      <section className="space-y-2 rounded-md border p-3" aria-labelledby="guest-service-heading" data-testid="guest-agent-services">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2"><Server className="h-4 w-4" /><h3 id="guest-service-heading" className="text-sm font-semibold">systemd 服务诊断</h3><Badge variant="outline">只读</Badge></div>
          <Button variant="outline" size="sm" data-testid="guest-agent-service-load" onClick={() => servicesMutation.mutate()} disabled={servicesMutation.isPending}>{servicesMutation.isPending ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" /> : null}读取</Button>
        </div>
        <p className="text-xs text-muted-foreground">仅执行固定的 systemctl 只读查询，不支持启动、停止、重启或修改服务。</p>
        <div className="max-h-64 divide-y overflow-auto rounded-md border text-xs">
          {services.length ? services.map((service) => (
            <div key={service.unit} data-testid={`guest-agent-service-${service.unit}`} className="flex min-w-0 flex-wrap items-center gap-2 px-2 py-1.5 sm:flex-nowrap">
              <span className="min-w-0 flex-1 truncate font-mono" title={service.unit}>{service.unit}</span>
              <Badge variant={service.active === "failed" ? "destructive" : service.active === "active" ? "success" : "secondary"}>{service.active}/{service.sub}</Badge>
              <span className="w-full truncate text-muted-foreground sm:w-64" title={service.description}>{service.description || "-"}</span>
            </div>
          )) : <p className="p-3 text-muted-foreground">尚未读取服务状态。仅支持使用 systemd 的 Linux 客户机。</p>}
        </div>
      </section>

      <section className="min-w-0 space-y-3 rounded-md border p-3" aria-labelledby="guest-cloud-init-heading" data-testid="guest-agent-cloud-init">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2"><Cloud className="h-4 w-4" /><h3 id="guest-cloud-init-heading" className="text-sm font-semibold">Cloud-init 执行诊断</h3><Badge variant="outline">只读</Badge></div>
          <Button variant="outline" size="sm" data-testid="guest-agent-cloud-init-load" onClick={() => cloudInitMutation.mutate()} disabled={cloudInitMutation.isPending}>{cloudInitMutation.isPending ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" /> : null}读取</Button>
        </div>
        <p className="text-xs text-muted-foreground">仅执行固定的 cloud-init 状态查询，并读取固定日志尾部；不执行任意命令或修改客户机配置。</p>
        {cloudInit ? (
          <div className="min-w-0 space-y-3 text-xs" data-testid="guest-agent-cloud-init-result">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant={!cloudInit.installed ? "secondary" : cloudInit.state === "done" ? "success" : cloudInit.state === "error" ? "destructive" : cloudInit.state === "running" ? "warning" : "secondary"}>
                {!cloudInit.installed ? "未检测到" : cloudInit.state}
              </Badge>
              {cloudInit.status ? <span>status: <span className="font-mono">{cloudInit.status}</span></span> : null}
              {cloudInit.extendedStatus ? <span>extended: <span className="font-mono">{cloudInit.extendedStatus}</span></span> : null}
              {cloudInit.logTruncated ? <Badge variant="warning">日志已截断</Badge> : null}
            </div>
            {cloudInit.installed ? <div className="grid min-w-0 gap-2 sm:grid-cols-2">
              <div className="min-w-0 rounded-md border bg-muted/20 p-2"><p className="text-muted-foreground">阶段</p><p className="mt-1 break-all font-mono">{cloudInit.stage || "-"}</p></div>
              <div className="min-w-0 rounded-md border bg-muted/20 p-2"><p className="text-muted-foreground">数据源</p><p className="mt-1 break-all font-mono">{cloudInit.datasource || "-"}</p></div>
              <div className="min-w-0 rounded-md border bg-muted/20 p-2"><p className="text-muted-foreground">最近更新</p><p className="mt-1 break-all font-mono">{cloudInit.lastUpdate || "-"}</p></div>
              <div className="min-w-0 rounded-md border bg-muted/20 p-2"><p className="text-muted-foreground">检查时间</p><p className="mt-1 break-all font-mono">{new Date(cloudInit.checkedAt).toLocaleString("zh-CN")}</p></div>
            </div> : null}
            {cloudInit.errors.length ? <div className="rounded-md border border-destructive/30 bg-destructive/5 p-2 text-destructive"><p className="font-medium">执行错误</p><ul className="mt-1 list-disc space-y-1 break-all pl-4 font-mono">{cloudInit.errors.map((item) => <li key={item}>{item}</li>)}</ul></div> : null}
            {cloudInit.recoverableErrors.length ? <div className="rounded-md border border-warning/30 bg-warning/5 p-2 text-warning"><p className="font-medium">可恢复错误</p><ul className="mt-1 list-disc space-y-1 break-all pl-4 font-mono">{cloudInit.recoverableErrors.map((item) => <li key={item}>{item}</li>)}</ul></div> : null}
            {cloudInit.log ? <div className="min-w-0 space-y-1"><div className="flex flex-wrap items-center justify-between gap-2 text-muted-foreground"><span>日志尾部</span><span className="break-all font-mono">{cloudInit.logPath || "-"}</span></div><pre data-testid="guest-agent-cloud-init-log" className="max-h-64 overflow-auto rounded-md bg-muted/40 p-2 font-mono text-[11px] whitespace-pre-wrap break-all">{cloudInit.log}</pre></div> : <p className="text-muted-foreground">未读取到 Cloud-init 日志尾部。</p>}
          </div>
        ) : <p className="text-muted-foreground">尚未读取 Cloud-init 状态。</p>}
      </section>

      <section className="space-y-2 rounded-md border p-3" aria-labelledby="guest-policy-heading">
        <div className="flex flex-wrap items-center justify-between gap-2"><h3 id="guest-policy-heading" className="text-sm font-semibold">期望状态与健康历史</h3><Button variant="outline" size="sm" onClick={() => policyMutation.mutate()} disabled={policyMutation.isPending}>刷新</Button></div>
        {policy ? <div className="flex flex-wrap items-center gap-3 text-xs"><Badge variant={policy.expected ? "success" : "secondary"}>{policy.expected ? "监控中" : "未监控"}</Badge><span>状态 {policy.lastState ?? "未知"}</span><span>连续失败 {policy.consecutiveFailures}</span><Button variant="outline" size="sm" onClick={() => updatePolicyMutation.mutate(!policy.expected)} disabled={updatePolicyMutation.isPending}>切换监控</Button></div> : null}
        {health ? <div className="space-y-1 text-xs"><p>最近 {health.total} 个样本，连通率 <span className="font-semibold">{health.connectivityPercent.toFixed(1)}%</span></p><div className="flex h-6 items-end gap-0.5 overflow-hidden rounded bg-muted/40 p-1">{health.items.slice().reverse().map((sample) => <span key={sample.id} title={`${sample.sampledAt} ${sample.state}`} className={sample.connected ? "h-full min-w-1 flex-1 bg-success" : "h-full min-w-1 flex-1 bg-destructive"} />)}</div></div> : null}
      </section>

      <AlertDialog open={pending !== null} onOpenChange={(open) => { if (!open) setPending(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认高风险客户机操作？</AlertDialogTitle>
            <AlertDialogDescription>
              {pending?.kind === "signal" ? `将向 PID ${pending.process.pid} (${pending.process.command}) 发送 SIGTERM。` : "该操作会修改客户机状态，并记录脱敏审计与任务。请确认目标虚拟机与用户无误。"}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel><Button variant="destructive" onClick={confirmPending}>确认执行</Button></AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

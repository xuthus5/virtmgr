import { lazy, Suspense, useEffect, useMemo, useState } from "react";
import {
  AlertTriangle,
  CheckCircle2,
  Code2,
  Copy,
  FileCode2,
  Package,
  Settings2,
  Terminal,
  UserRound,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  cloudInitByteLength,
  cloudInitDraftIssues,
  emptyCloudInitDraft,
  maxCloudInitBytes,
  parseCloudInitUserData,
  serializeCloudInit,
  type CloudInitDraft,
} from "@/features/cloud-init/cloud-init-model";
import { cn } from "@/lib/utils";

const YAMLCodeEditor = lazy(() =>
  import("@/components/code/xml-code-editor").then((module) => ({ default: module.XMLCodeEditor })),
);

type CloudInitEditorProps = {
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  prefix?: string;
  className?: string;
};

function initialEditorState(value: string): { mode: "visual" | "source"; draft: CloudInitDraft } {
  const parsed = parseCloudInitUserData(value);
  const visualMode = parsed.visualizable && parsed.unsupported.length === 0;
  return {
    mode: visualMode ? "visual" : "source",
    draft: parsed.visualizable ? parsed.draft : { ...emptyCloudInitDraft },
  };
}

export function CloudInitEditor({ value, onChange, disabled = false, prefix = "cloud-init", className }: CloudInitEditorProps) {
  const initialState = useMemo(() => initialEditorState(value), []); // Initialize once; external edits are synchronized below.
  const [mode, setMode] = useState<"visual" | "source">(initialState.mode);
  const [draft, setDraft] = useState<CloudInitDraft>(initialState.draft);
  const [modeNotice, setModeNotice] = useState("");
  const [copied, setCopied] = useState(false);
  const parsed = useMemo(() => parseCloudInitUserData(value), [value]);
  const preview = useMemo(() => serializeCloudInit(draft), [draft]);
  const issues = cloudInitDraftIssues(draft);
  const bytes = cloudInitByteLength(value);
  const tooLarge = bytes > maxCloudInitBytes;

  useEffect(() => {
    if (parsed.visualizable) setDraft(parsed.draft);
  }, [parsed]);

  function updateDraft<K extends keyof CloudInitDraft>(key: K, nextValue: CloudInitDraft[K]) {
    const next = { ...draft, [key]: nextValue };
    setDraft(next);
    onChange(serializeCloudInit(next));
    setModeNotice("");
  }

  function showVisualEditor() {
    if (disabled) return;
    if (!parsed.visualizable) {
      setModeNotice(parsed.error || "当前 user-data 无法转换为可视化配置，请先在源码模式修正。");
      return;
    }
    setDraft(parsed.draft);
    setMode("visual");
    setModeNotice(parsed.unsupported.length
      ? `源码包含高级字段（${parsed.unsupported.join("、")}）。可视化模式只生成下方字段，修改后会移除这些高级字段。`
      : "");
  }

  async function copyYAML() {
    const content = mode === "visual" ? preview : value;
    try {
      await navigator.clipboard.writeText(content);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  }

  const toggleClass = "flex items-center gap-2 rounded-md border bg-card px-3 py-2 text-xs text-foreground";

  return (
    <section className={cn("overflow-hidden rounded-xl border bg-card shadow-sm", className)} data-testid={`${prefix}-editor`}>
      <div className="flex flex-col gap-3 border-b bg-muted/25 p-3 sm:flex-row sm:flex-wrap sm:items-start sm:p-4">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <FileCode2 className="h-4 w-4 text-primary" />
            <h4 className="text-sm font-semibold">Cloud-init 配置</h4>
            <Badge variant="outline">NoCloud user-data</Badge>
            <Badge variant={bytes > maxCloudInitBytes ? "destructive" : "secondary"}>{bytes} / 64 KiB</Badge>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">用表单填写常用初始化项并实时生成标准 <code className="font-mono">#cloud-config</code> YAML；高级场景可切换到源码模式。</p>
        </div>
        <div className="grid w-full grid-cols-2 rounded-lg border bg-card p-0.5 sm:inline-flex sm:w-auto" role="tablist" aria-label="Cloud-init 编辑模式">
          <button
            type="button"
            role="tab"
            aria-selected={mode === "visual"}
            data-testid={`${prefix}-mode-visual`}
            onClick={showVisualEditor}
            disabled={disabled}
            className={cn("inline-flex min-w-0 items-center justify-center gap-1.5 whitespace-nowrap rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors disabled:opacity-50", mode === "visual" ? "bg-primary text-primary-foreground shadow-sm" : "text-muted-foreground hover:text-foreground")}
          >
            <Settings2 className="h-3.5 w-3.5" />可视化
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={mode === "source"}
            data-testid={`${prefix}-mode-source`}
            onClick={() => { setMode("source"); setModeNotice(""); }}
            disabled={disabled}
            className={cn("inline-flex min-w-0 items-center justify-center gap-1.5 whitespace-nowrap rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors disabled:opacity-50", mode === "source" ? "bg-primary text-primary-foreground shadow-sm" : "text-muted-foreground hover:text-foreground")}
          >
            <Code2 className="h-3.5 w-3.5" />YAML 源码
          </button>
        </div>
      </div>

      {modeNotice ? (
        <div className="flex items-start gap-2 border-b border-warning/25 bg-warning/10 px-4 py-2.5 text-xs text-warning" role="status" data-testid={`${prefix}-mode-notice`}>
          <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          <span>{modeNotice}</span>
        </div>
      ) : null}

      {mode === "visual" ? (
        <div className="grid gap-4 p-3 sm:p-4 xl:grid-cols-[minmax(0,1.08fr)_minmax(20rem,0.92fr)]">
          <div className="space-y-4">
            <div className="rounded-lg border p-3.5">
              <div className="mb-3 flex items-center gap-2">
                <Package className="h-4 w-4 text-primary" />
                <div>
                  <p className="text-sm font-medium">软件包与系统更新</p>
                  <p className="text-xs text-muted-foreground">首次启动时更新软件源并安装所需组件。</p>
                </div>
              </div>
              <div className="mb-3 flex flex-wrap gap-2">
                <label className={toggleClass}>
                  <input aria-label="Cloud-init 更新软件源" type="checkbox" className="h-3.5 w-3.5" checked={draft.packageUpdate} onChange={(event) => updateDraft("packageUpdate", event.target.checked)} disabled={disabled} />
                  更新软件源
                </label>
                <label className={toggleClass}>
                  <input aria-label="Cloud-init 升级已安装软件" type="checkbox" className="h-3.5 w-3.5" checked={draft.packageUpgrade} onChange={(event) => updateDraft("packageUpgrade", event.target.checked)} disabled={disabled} />
                  升级已安装软件
                </label>
                <label className={toggleClass}>
                  <input aria-label="Cloud-init 需要时重启" type="checkbox" className="h-3.5 w-3.5" checked={draft.packageRebootIfRequired} onChange={(event) => updateDraft("packageRebootIfRequired", event.target.checked)} disabled={disabled} />
                  内核更新后重启
                </label>
              </div>
              <div className="space-y-2">
                <Label htmlFor={`${prefix}-packages`}>安装软件包</Label>
                <Textarea
                  id={`${prefix}-packages`}
                  data-testid={`${prefix}-packages`}
                  value={draft.packages}
                  onChange={(event) => updateDraft("packages", event.target.value)}
                  placeholder={"nginx\ncurl\ntmux"}
                  rows={3}
                  spellCheck={false}
                  disabled={disabled}
                  className="font-mono text-xs"
                />
                <p className="text-xs text-muted-foreground">每行一个包名，也可用逗号分隔；生成到 <code className="font-mono">packages</code>。</p>
              </div>
            </div>

            <div className="rounded-lg border p-3.5">
              <div className="mb-3 flex items-center gap-2">
                <Terminal className="h-4 w-4 text-primary" />
                <div>
                  <p className="text-sm font-medium">首次启动命令</p>
                  <p className="text-xs text-muted-foreground">命令按填写顺序执行，适合初始化服务、写日志或拉取配置。</p>
                </div>
              </div>
              <Textarea
                aria-label="Cloud-init 启动命令"
                data-testid={`${prefix}-runcmd`}
                value={draft.runcmd}
                onChange={(event) => updateDraft("runcmd", event.target.value)}
                placeholder={"systemctl enable --now nginx\necho \"cloud-init completed\" | tee /var/log/virtmgr-cloud-init.log"}
                rows={5}
                spellCheck={false}
                disabled={disabled}
                className="font-mono text-xs"
              />
            </div>

            <div className="rounded-lg border p-3.5">
              <div className="mb-3 flex items-center gap-2">
                <Settings2 className="h-4 w-4 text-primary" />
                <div>
                  <p className="text-sm font-medium">区域与语言</p>
                  <p className="text-xs text-muted-foreground">留空时不写入，仅设置你明确需要的项。</p>
                </div>
              </div>
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor={`${prefix}-timezone`}>时区</Label>
                  <Input id={`${prefix}-timezone`} data-testid={`${prefix}-timezone`} value={draft.timezone} onChange={(event) => updateDraft("timezone", event.target.value)} placeholder="Asia/Shanghai" disabled={disabled} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor={`${prefix}-locale`}>Locale</Label>
                  <Input id={`${prefix}-locale`} data-testid={`${prefix}-locale`} value={draft.locale} onChange={(event) => updateDraft("locale", event.target.value)} placeholder="zh_CN.UTF-8" disabled={disabled} />
                </div>
              </div>
            </div>

            <div className="rounded-lg border p-3.5">
              <label className="flex items-start gap-3 text-sm">
                <input
                  aria-label="Cloud-init 创建 sudo 用户"
                  data-testid={`${prefix}-create-user`}
                  type="checkbox"
                  className="mt-0.5 h-4 w-4"
                  checked={draft.createUser}
                  onChange={(event) => updateDraft("createUser", event.target.checked)}
                  disabled={disabled}
                />
                <span>
                  <span className="flex items-center gap-1.5 font-medium"><UserRound className="h-4 w-4 text-primary" />创建 sudo 用户</span>
                  <span className="mt-0.5 block text-xs text-muted-foreground">密码默认锁定，使用 SSH 公钥登录，并生成免密 sudo 规则。</span>
                </span>
              </label>
              {draft.createUser ? (
                <div className="mt-3 grid gap-3 border-t pt-3 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor={`${prefix}-username`}>用户名</Label>
                    <Input id={`${prefix}-username`} data-testid={`${prefix}-username`} value={draft.username} onChange={(event) => updateDraft("username", event.target.value)} placeholder="deploy" disabled={disabled} />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor={`${prefix}-groups`}>附加组</Label>
                    <Input id={`${prefix}-groups`} data-testid={`${prefix}-groups`} value={draft.groups} onChange={(event) => updateDraft("groups", event.target.value)} placeholder="sudo, docker" disabled={disabled} />
                  </div>
                  <div className="space-y-2 sm:col-span-2">
                    <Label htmlFor={`${prefix}-shell`}>Shell</Label>
                    <Input id={`${prefix}-shell`} data-testid={`${prefix}-shell`} value={draft.shell} onChange={(event) => updateDraft("shell", event.target.value)} placeholder="/bin/bash" disabled={disabled} />
                  </div>
                  <div className="space-y-2 sm:col-span-2">
                    <Label htmlFor={`${prefix}-ssh-keys`}>SSH 公钥</Label>
                    <Textarea
                      id={`${prefix}-ssh-keys`}
                      data-testid={`${prefix}-ssh-keys`}
                      value={draft.sshAuthorizedKeys}
                      onChange={(event) => updateDraft("sshAuthorizedKeys", event.target.value)}
                      placeholder="ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA... user@example"
                      rows={3}
                      spellCheck={false}
                      disabled={disabled}
                      className="font-mono text-xs"
                    />
                    <p className="text-xs text-muted-foreground">每行一个公钥，生成到 <code className="font-mono">users[0].ssh_authorized_keys</code>。</p>
                  </div>
                </div>
              ) : null}
            </div>

            {issues.length ? (
              <div className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert">
                {issues.map((issue) => <p key={issue}>{issue}</p>)}
              </div>
            ) : null}
          </div>

          <aside className="flex min-h-[18rem] flex-col overflow-hidden rounded-lg border border-slate-800 bg-slate-950 text-slate-100 sm:min-h-[26rem]">
            <div className="flex items-center gap-2 border-b border-slate-800 px-3 py-2">
              <Code2 className="h-4 w-4 text-sky-300" />
              <p className="text-xs font-medium">实时 YAML 预览</p>
              <Badge variant="outline" className="ml-auto border-slate-700 text-slate-300">cloud-config</Badge>
              <Button type="button" variant="ghost" size="icon-sm" className="text-slate-300 hover:bg-slate-800 hover:text-white" title="复制 YAML" aria-label="复制 Cloud-init YAML" onClick={() => void copyYAML()}>
                {copied ? <CheckCircle2 className="h-3.5 w-3.5 text-emerald-300" /> : <Copy className="h-3.5 w-3.5" />}
              </Button>
            </div>
            <pre data-testid={`${prefix}-preview`} className="min-h-0 flex-1 overflow-auto p-3 font-mono text-[11px] leading-relaxed text-slate-200">{preview}</pre>
          </aside>
        </div>
      ) : (
        <div className="space-y-3 p-3 sm:p-4">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant={parsed.cloudConfig ? "secondary" : "outline"}>{parsed.cloudConfig ? "#cloud-config" : "自定义 user-data"}</Badge>
            {parsed.cloudConfig && parsed.syntaxValid ? <Badge variant="success"><CheckCircle2 className="h-3 w-3" />YAML 语法有效</Badge> : null}
            {parsed.cloudConfig && !parsed.syntaxValid ? <Badge variant="destructive"><AlertTriangle className="h-3 w-3" />YAML 语法错误</Badge> : null}
            {!parsed.cloudConfig ? <span className="text-xs text-muted-foreground">脚本或自定义数据会原样写入 seed ISO。</span> : null}
            <Button type="button" variant="ghost" size="sm" className="ml-auto" onClick={() => void copyYAML()}>
              {copied ? <CheckCircle2 className="h-3.5 w-3.5 text-success" /> : <Copy className="h-3.5 w-3.5" />}复制
            </Button>
          </div>
          <div className="h-[22rem] overflow-hidden rounded-lg border sm:h-[28rem]" data-testid={`${prefix}-source-shell`}>
            <Suspense fallback={<div className="grid h-full place-items-center text-xs text-muted-foreground">正在加载 YAML 编辑器…</div>}>
              <YAMLCodeEditor
                value={value}
                onChange={onChange}
                readOnly={disabled}
                ariaLabel="Cloud-init YAML 源码"
                testId={`${prefix}-yaml-editor`}
                language="yaml"
                className="h-full"
              />
            </Suspense>
          </div>
          {parsed.error && parsed.cloudConfig ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert">{parsed.error}</p> : null}
          {parsed.unsupported.length ? (
            <p className="rounded-md border border-warning/30 bg-warning/5 px-3 py-2 text-xs text-warning" data-testid={`${prefix}-unsupported`}>
              源码包含可视化模式不管理的高级字段：<span className="font-mono">{parsed.unsupported.join(", ")}</span>。仅为查看时不会修改源码。
            </p>
          ) : null}
          <p className="text-xs text-muted-foreground">源码模式支持任意 cloud-init 键、脚本和高级 YAML 结构；切换回可视化并修改字段时，只会生成可视化支持的标准配置。</p>
        </div>
      )}

      {tooLarge ? <p className="border-t border-destructive/25 bg-destructive/5 px-4 py-2 text-xs text-destructive" role="alert">Cloud-init 用户数据超过 64 KiB 上限，请精简后保存。</p> : null}
    </section>
  );
}

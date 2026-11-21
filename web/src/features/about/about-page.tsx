import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  Archive,
  ArrowUpRight,
  BookOpen,
  CheckCircle2,
  CircleHelp,
  ExternalLink,
  Github,
  LoaderCircle,
  Network,
  RefreshCw,
  Server,
  ShieldCheck,
  TriangleAlert,
} from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { UpdateCheck } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { PageHeader } from "@/components/layout/page-header";
import { queryKeys } from "@/lib/query-keys";

const repositoryURL = "https://github.com/xuthus5/virtmgr";
const documentationURL = `${repositoryURL}/blob/main/README.zh-CN.md`;
const releasesURL = `${repositoryURL}/releases`;

const capabilities = [
  { icon: Server, title: "虚拟机生命周期", detail: "创建、导入、克隆、快照、设备热插拔、VNC/串口控制台和配置版本回滚。" },
  { icon: Archive, title: "备份与恢复", detail: "冷备份、在线一致性备份、ZIP 包导入导出、定时计划和保留策略。" },
  { icon: Network, title: "单机网络编排", detail: "NAT、隔离与外部桥接网络，IPv4/IPv6 双栈、DHCPv6、NAT66 和端口转发。" },
  { icon: ShieldCheck, title: "安全与审计", detail: "本地账号、CSRF、只读模式、API Token、写操作审计和受控 Guest Agent 能力。" },
];

type StatusMeta = {
  label: string;
  tone: "success" | "warning" | "destructive" | "secondary";
  icon: typeof CheckCircle2;
};

const updateStatusMeta: Record<UpdateCheck["status"], StatusMeta> = {
  update_available: { label: "发现新版本", tone: "warning", icon: ArrowUpRight },
  up_to_date: { label: "已是最新版", tone: "success", icon: CheckCircle2 },
  ahead: { label: "当前构建领先", tone: "secondary", icon: CircleHelp },
  unknown: { label: "无法比较版本", tone: "secondary", icon: CircleHelp },
  no_release: { label: "尚无稳定发布", tone: "secondary", icon: CircleHelp },
  unavailable: { label: "检查暂不可用", tone: "destructive", icon: TriangleAlert },
};

const timeFormatter = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});

function formatTime(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : timeFormatter.format(date);
}

function shortCommit(value?: string) {
  if (!value || value === "unknown") return value || "-";
  return value.length > 12 ? value.slice(0, 12) : value;
}

export function AboutPage() {
  const systemQuery = useQuery({ queryKey: queryKeys.systemInfo, queryFn: api.systemInfo, staleTime: 60_000 });
  const [updateResult, setUpdateResult] = useState<UpdateCheck | null>(null);
  const [updateError, setUpdateError] = useState("");
  const checkUpdate = useMutation({
    mutationFn: () => api.systemUpdate(true),
    onSuccess: (result) => {
      setUpdateResult(result);
      setUpdateError("");
    },
    onError: (cause) => {
      setUpdateError(cause instanceof ApiError ? cause.message : "更新检查请求失败，请稍后重试。");
    },
  });

  const build = systemQuery.data?.build;
  const tls = systemQuery.data?.tls;
  const statusMeta = updateResult ? updateStatusMeta[updateResult.status] : null;
  const StatusIcon = statusMeta?.icon;

  return (
    <div data-testid="about-page">
      <PageHeader
        title="关于 VirtMgr"
        breadcrumb="总览 / 关于"
        description="了解当前产品定位、运行版本和单机 KVM/libvirt 运维能力，并按需检查稳定发布版本。"
        actions={<Button asChild variant="outline" size="sm"><a href={repositoryURL} target="_blank" rel="noreferrer"><Github className="h-4 w-4" />GitHub 仓库</a></Button>}
      />

      <Card className="overflow-hidden border-primary/20 bg-gradient-to-br from-primary/10 via-card to-card" data-testid="about-hero">
        <CardContent className="p-5 sm:p-7">
          <div className="flex flex-col gap-5 sm:flex-row sm:items-start">
            <div className="grid h-14 w-14 shrink-0 place-items-center rounded-2xl bg-primary text-2xl font-bold text-primary-foreground shadow-sm">V</div>
            <div className="min-w-0 flex-1">
              <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-primary">Single-host KVM / libvirt console</p>
              <h2 className="mt-1 text-2xl font-semibold tracking-tight">VirtMgr</h2>
              <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">
                面向单台 Linux/KVM 宿主机的 Web 管理平台，以 libvirt 为基础设施事实来源，提供虚拟机、存储、网络、备份、Guest Agent、审计和 API 自动化能力。产品目标是做轻量、可自主部署、安全边界清晰的“单机精品”，而不是多集群云平台。
              </p>
              <div className="mt-4 flex flex-wrap gap-2">
                <Badge variant="secondary">单二进制部署</Badge>
                <Badge variant="secondary">libvirt 权威数据源</Badge>
                <Badge variant="secondary">IPv4 / IPv6 双栈</Badge>
                <Badge variant="secondary">Apache-2.0</Badge>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><BookOpen className="h-4 w-4 text-primary" />项目说明</CardTitle>
            <CardDescription>当前版本明确聚焦单机运维闭环，并保留清晰的扩展边界。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 text-sm leading-6 text-muted-foreground">
            <p>VirtMgr 不复制云端控制面，也不接管宿主机全部系统管理职责。虚拟机、存储池和网络状态优先从 libvirt 回读，业务元数据保存在本地 SQLite。</p>
            <ul className="space-y-2">
              <li className="flex gap-2"><span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-primary" />一个实例管理一个 libvirt 端点，强调部署简单和故障边界清晰。</li>
              <li className="flex gap-2"><span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-primary" />默认最小权限运行；Guest Agent 动作按客户机能力矩阵开放，不提供通用任意命令接口。</li>
              <li className="flex gap-2"><span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-primary" />写操作遵循角色、CSRF、运行时只读模式和审计约束。</li>
              <li className="flex gap-2"><span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-primary" />更新检查只读取 GitHub Releases 元数据，不自动下载、安装或重启服务。</li>
            </ul>
          </CardContent>
        </Card>

        <Card data-testid="about-runtime">
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><Server className="h-4 w-4 text-primary" />运行信息</CardTitle>
            <CardDescription>构建信息来自当前运行二进制，不依赖浏览器缓存。</CardDescription>
          </CardHeader>
          <CardContent>
            {systemQuery.isLoading ? <p className="text-sm text-muted-foreground">正在读取运行版本…</p> : null}
            {systemQuery.isError ? <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive"><p className="font-medium">无法读取运行信息</p><p className="mt-1">{systemQuery.error instanceof Error ? systemQuery.error.message : "请求失败"}</p><Button variant="outline" size="sm" className="mt-3" onClick={() => void systemQuery.refetch()}>重试</Button></div> : null}
            {build ? (
              <dl className="grid gap-3 text-sm sm:grid-cols-2">
                <div className="rounded-md border bg-muted/20 p-3"><dt className="text-xs text-muted-foreground">当前版本</dt><dd className="mt-1 break-all font-mono font-medium" data-testid="about-version">{build.version || "dev"}</dd></div>
                <div className="rounded-md border bg-muted/20 p-3"><dt className="text-xs text-muted-foreground">提交</dt><dd className="mt-1 break-all font-mono" title={build.commit}>{shortCommit(build.commit)}</dd></div>
                <div className="rounded-md border bg-muted/20 p-3"><dt className="text-xs text-muted-foreground">构建时间</dt><dd className="mt-1">{formatTime(build.date)}</dd></div>
                <div className="rounded-md border bg-muted/20 p-3"><dt className="text-xs text-muted-foreground">管理面 TLS</dt><dd className="mt-1 flex flex-wrap items-center gap-2">{tls?.enabled ? <Badge variant="success">HTTPS 已启用</Badge> : <Badge variant="secondary">未启用</Badge>}{tls?.enabled && typeof tls.expiresInDays === "number" ? <span className="text-xs text-muted-foreground">剩余 {tls.expiresInDays} 天</span> : null}</dd></div>
              </dl>
            ) : null}
            {tls?.warnings?.length ? <p className="mt-3 rounded-md border border-warning/30 bg-warning/10 p-3 text-xs text-warning">{tls.warnings.join("；")}</p> : null}
            <div className="mt-4 flex flex-wrap gap-2 border-t pt-4">
              <Button asChild variant="outline" size="sm"><a href={documentationURL} target="_blank" rel="noreferrer"><BookOpen className="h-4 w-4" />中文文档</a></Button>
              <Button asChild variant="outline" size="sm"><a href="/api/v1/openapi.json" target="_blank" rel="noreferrer"><ExternalLink className="h-4 w-4" />OpenAPI</a></Button>
            </div>
          </CardContent>
        </Card>
      </div>

      <Card className="mt-4">
        <CardHeader>
          <CardTitle>核心能力</CardTitle>
          <CardDescription>当前版本覆盖的主要单机运维工作负载。</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          {capabilities.map(({ icon: Icon, title, detail }) => (
            <div key={title} className="rounded-lg border bg-muted/15 p-4">
              <Icon className="h-4 w-4 text-primary" />
              <p className="mt-3 text-sm font-medium">{title}</p>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">{detail}</p>
            </div>
          ))}
        </CardContent>
      </Card>

      <Card className="mt-4" data-testid="about-update-check">
        <CardHeader>
          <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-start">
            <div>
              <CardTitle className="flex items-center gap-2"><RefreshCw className="h-4 w-4 text-primary" />更新检查</CardTitle>
              <CardDescription>由服务器访问 GitHub Releases，仅返回版本和发布说明；不会自动升级，也不会修改当前实例。</CardDescription>
            </div>
            <Button data-testid="about-check-update" size="sm" disabled={checkUpdate.isPending} onClick={() => { setUpdateError(""); checkUpdate.mutate(); }}>
              {checkUpdate.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
              {checkUpdate.isPending ? "正在检查" : "检查更新"}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="rounded-md border bg-muted/20 p-3"><p className="text-xs text-muted-foreground">当前运行版本</p><p className="mt-1 break-all font-mono text-sm font-medium">{build?.version || "正在读取"}</p></div>
            <div className="rounded-md border bg-muted/20 p-3"><p className="text-xs text-muted-foreground">最近检查</p><p className="mt-1 text-sm">{updateResult ? formatTime(updateResult.checkedAt) : "尚未检查"}</p></div>
          </div>

          {updateError ? <p role="alert" className="mt-3 rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">{updateError}</p> : null}
          {!updateResult && !updateError ? <div className="mt-4 rounded-lg border border-dashed p-5 text-sm text-muted-foreground"><p className="font-medium text-foreground">尚未检查更新</p><p className="mt-1">点击“检查更新”后，VirtMgr 将读取 GitHub 最新稳定发布版本并比较当前构建。</p></div> : null}

          {updateResult && statusMeta && StatusIcon ? (
            <div className="mt-4 rounded-lg border p-4" data-testid="about-update-result">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="flex items-start gap-3">
                  <div className="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-muted"><StatusIcon className="h-4 w-4" /></div>
                  <div><div className="flex flex-wrap items-center gap-2"><p className="font-medium">{statusMeta.label}</p><Badge variant={statusMeta.tone}>{updateResult.status}</Badge>{updateResult.cached ? <Badge variant="secondary">缓存结果</Badge> : null}</div><p className="mt-1 text-sm text-muted-foreground">{updateResult.message}</p></div>
                </div>
                {updateResult.releaseUrl ? <Button asChild variant="outline" size="sm"><a href={updateResult.releaseUrl} target="_blank" rel="noreferrer" data-testid="about-release-link"><ArrowUpRight className="h-4 w-4" />打开发布页</a></Button> : null}
              </div>
              <div className="mt-4 grid gap-3 sm:grid-cols-2">
                <div className="rounded-md bg-muted/30 p-3"><p className="text-xs text-muted-foreground">最新稳定版本</p><p className="mt-1 break-all font-mono text-sm font-medium">{updateResult.latestVersion || "-"}</p></div>
                <div className="rounded-md bg-muted/30 p-3"><p className="text-xs text-muted-foreground">发布时间</p><p className="mt-1 text-sm">{formatTime(updateResult.publishedAt)}</p></div>
              </div>
              {updateResult.releaseName ? <p className="mt-4 text-sm font-medium">{updateResult.releaseName}</p> : null}
              {updateResult.releaseNotes ? <div className="mt-2 max-h-72 overflow-y-auto whitespace-pre-wrap break-words rounded-md border bg-background p-3 text-xs leading-5 text-muted-foreground">{updateResult.releaseNotes}</div> : null}
            </div>
          ) : null}

          <div className="mt-4 flex flex-wrap items-center justify-between gap-2 border-t pt-4 text-xs text-muted-foreground">
            <span>检查结果最多缓存 10 分钟；手动检查会绕过缓存。</span>
            <a href={releasesURL} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 font-medium text-primary underline-offset-4 hover:underline">查看全部 Release <ExternalLink className="h-3 w-3" /></a>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, ChevronDown, ChevronUp, LoaderCircle, PackageSearch, RefreshCw, Search, ShieldAlert } from "lucide-react";
import { api } from "@/api/client";
import type { GuestAgentPackage } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { queryKeys } from "@/lib/query-keys";
import { cn } from "@/lib/utils";

const dateTime = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false,
});

export function GuestAgentPackages({ uuid, canRead }: { uuid: string; canRead: boolean }) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [upgradableOnly, setUpgradableOnly] = useState(false);
  const query = useQuery({
    queryKey: queryKeys.vm(uuid).guestAgentPackages,
    queryFn: () => api.guestAgentPackages(uuid),
    enabled: open && canRead,
    staleTime: 30_000,
    retry: 0,
  });
  const items = query.data?.items ?? [];
  const filtered = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return items.filter((item) => {
      if (upgradableOnly && !item.upgradable) return false;
      if (!needle) return true;
      return `${item.name} ${item.version} ${item.updateVersion ?? ""} ${item.summary ?? ""}`.toLowerCase().includes(needle);
    });
  }, [items, search, upgradableOnly]);

  return <div className="rounded-md border" data-testid="guest-agent-packages">
    <button type="button" className="flex w-full items-center justify-between gap-3 bg-muted/20 px-3 py-2 text-left text-sm font-medium" aria-expanded={open} data-testid="guest-agent-packages-toggle" onClick={() => setOpen((current) => !current)}>
      <span className="flex items-center gap-2"><PackageSearch className="h-4 w-4" />软件包清单</span>
      <span className="flex items-center gap-2 text-xs font-normal text-muted-foreground">{open ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}{open ? "收起" : "只读查看已安装与可升级软件包"}</span>
    </button>
    {open ? <div className="space-y-3 border-t px-3 py-3 text-xs">
      {!canRead ? <p className="text-muted-foreground" role="status">当前账号没有读取客户机软件包清单的权限。</p> : null}
      {canRead && query.isLoading ? <p className="flex items-center gap-2 text-muted-foreground"><LoaderCircle className="h-3.5 w-3.5 animate-spin" />正在通过 Guest Agent 读取软件包清单...</p> : null}
      {canRead && query.isError ? <div className="flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-destructive" role="alert"><AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" /><span>{query.error instanceof Error ? query.error.message : "读取软件包清单失败。"}</span></div> : null}
      {query.data ? <>
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={query.data.securityUpdates > 0 ? "destructive" : "success"}>{query.data.securityUpdates} 个安全更新</Badge>
          <Badge variant={query.data.upgradable > 0 ? "warning" : "secondary"}>{query.data.upgradable} 个可升级</Badge>
          <Badge variant="outline">已安装 {query.data.total} 个</Badge>
          <Badge variant="secondary">{query.data.manager || "未知包管理器"}</Badge>
          {query.data.truncated ? <Badge variant="warning">列表已截断</Badge> : null}
          <span className="text-muted-foreground">检查于 {formatDateTime(query.data.checkedAt)}</span>
          <Button type="button" variant="outline" size="sm" className="ml-auto h-7 px-2 text-[11px]" onClick={() => void query.refetch()} disabled={query.isFetching}>{query.isFetching ? <LoaderCircle className="h-3 w-3 animate-spin" /> : <RefreshCw className="h-3 w-3" />}刷新</Button>
        </div>
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
          <label className="relative min-w-0 flex-1"><Search className="pointer-events-none absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted-foreground" /><Input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="搜索名称、版本或摘要" className="h-8 pl-8 text-xs" data-testid="guest-packages-search" /></label>
          <label className="flex items-center gap-2 whitespace-nowrap text-muted-foreground"><input type="checkbox" checked={upgradableOnly} onChange={(event) => setUpgradableOnly(event.target.checked)} />仅看可升级</label>
        </div>
        {query.data.warnings.length ? <div className="rounded-md border border-warning/30 bg-warning/10 px-3 py-2 text-warning"><p className="flex items-center gap-1.5 font-medium"><ShieldAlert className="h-3.5 w-3.5" />部分信息不可用</p><ul className="mt-1 space-y-0.5">{query.data.warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul></div> : null}
        {filtered.length ? <div className="max-h-80 overflow-auto rounded-md border">
          <table className="w-full min-w-[620px] text-left text-xs">
            <thead className="sticky top-0 z-10 bg-muted/95"><tr><th className="px-3 py-2 font-medium">软件包</th><th className="px-3 py-2 font-medium">已安装版本</th><th className="px-3 py-2 font-medium">可升级版本</th><th className="px-3 py-2 font-medium">标记</th></tr></thead>
            <tbody className="divide-y">{filtered.map((item) => <PackageRow key={`${item.name}-${item.architecture ?? ""}`} item={item} />)}</tbody>
          </table>
        </div> : <p className="rounded-md border bg-muted/20 px-3 py-4 text-center text-muted-foreground">{items.length ? "没有匹配的软件包。" : "客户机未返回软件包清单。"}</p>}
        <p className="text-[11px] text-muted-foreground">只读清单不会执行安装、升级、卸载或刷新软件源；安全更新标记由 apt/dnf/zypper 的只读元数据判断。</p>
      </> : null}
    </div> : null}
  </div>;
}

function PackageRow({ item }: { item: GuestAgentPackage }) {
  return <tr className={cn(item.securityUpdate && "bg-destructive/5")}>
    <td className="px-3 py-2"><p className="break-all font-mono text-[11px] text-foreground">{item.name}</p>{item.summary ? <p className="mt-0.5 line-clamp-1 text-muted-foreground" title={item.summary}>{item.summary}</p> : null}</td>
    <td className="break-all px-3 py-2 font-mono text-[11px] text-muted-foreground">{item.version}</td>
    <td className="break-all px-3 py-2 font-mono text-[11px]">{item.upgradable ? item.updateVersion || "可升级" : "-"}</td>
    <td className="px-3 py-2"><div className="flex flex-wrap gap-1">{item.securityUpdate ? <Badge variant="destructive">安全</Badge> : null}{item.upgradable ? <Badge variant="warning">可升级</Badge> : <Badge variant="secondary">最新</Badge>}</div></td>
  </tr>;
}

function formatDateTime(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "-" : dateTime.format(date);
}

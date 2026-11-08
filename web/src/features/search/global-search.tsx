import { useDeferredValue, useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { useNavigate } from "@tanstack/react-router";
import { Command, CornerDownLeft, HardDrive, LoaderCircle, Network, Search, Server, X } from "lucide-react";
import type { Role } from "@/api/types";
import { Button } from "@/components/ui/button";
import { useNetworksQuery } from "@/features/networks/use-networks-query";
import { buildGlobalSearchItems, type GlobalSearchItem } from "@/features/search/search-model";
import { useStoragePoolsQuery } from "@/features/storage/use-storage-query";
import { useVMsQuery } from "@/features/vms/use-vms-query";
import { cn } from "@/lib/utils";

function ResultIcon({ kind }: { kind: GlobalSearchItem["kind"] }) {
  if (kind === "vm") return <Server className="h-4 w-4" />;
  if (kind === "network") return <Network className="h-4 w-4" />;
  if (kind === "storage") return <HardDrive className="h-4 w-4" />;
  return <CornerDownLeft className="h-4 w-4" />;
}

export function GlobalSearch({ role }: { role: Role }) {
  const navigate = useNavigate();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const deferredQuery = useDeferredValue(query);
  const vmsQuery = useVMsQuery("", "", open);
  const networksQuery = useNetworksQuery(open);
  const poolsQuery = useStoragePoolsQuery(open);

  const results = useMemo(() => buildGlobalSearchItems(deferredQuery, role, {
    vms: vmsQuery.data?.items,
    networks: networksQuery.data?.items,
    storagePools: poolsQuery.data?.items,
  }), [deferredQuery, role, vmsQuery.data, networksQuery.data, poolsQuery.data]);

  const loading = open && (vmsQuery.isLoading || networksQuery.isLoading || poolsQuery.isLoading);
  const sourceError = vmsQuery.isError || networksQuery.isError || poolsQuery.isError;

  useEffect(() => {
    function onShortcut(event: globalThis.KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setOpen(true);
      }
    }
    window.addEventListener("keydown", onShortcut);
    return () => window.removeEventListener("keydown", onShortcut);
  }, []);

  useEffect(() => {
    if (!open) return;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    setActiveIndex(0);
    return () => { document.body.style.overflow = previousOverflow; };
  }, [open]);

  useEffect(() => {
    setActiveIndex((current) => Math.min(current, Math.max(0, results.length - 1)));
  }, [results.length]);

  function close() {
    setOpen(false);
    setQuery("");
    setActiveIndex(0);
    window.setTimeout(() => triggerRef.current?.focus(), 0);
  }

  function openResult(item: GlobalSearchItem) {
    close();
    if (item.kind === "vm") {
      void navigate({ to: "/vms/$uuid", params: { uuid: item.uuid } });
      return;
    }
    void navigate({ to: item.path });
  }

  function onDialogKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "Escape") {
      event.preventDefault();
      close();
      return;
    }
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActiveIndex((current) => results.length ? (current + 1) % results.length : 0);
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      setActiveIndex((current) => results.length ? (current - 1 + results.length) % results.length : 0);
      return;
    }
    if (event.key === "Enter" && results[activeIndex]) {
      event.preventDefault();
      openResult(results[activeIndex]);
    }
  }

  return (
    <>
      <Button
        ref={triggerRef}
        type="button"
        variant="outline"
        size="sm"
        className="h-9 gap-2 px-2 text-muted-foreground sm:px-3"
        data-testid="global-search-trigger"
        aria-label="全局搜索"
        aria-haspopup="dialog"
        onClick={() => setOpen(true)}
      >
        <Search className="h-4 w-4" />
        <span className="hidden sm:inline">搜索</span>
        <kbd className="hidden items-center gap-0.5 rounded border bg-muted/50 px-1.5 py-0.5 font-mono text-[10px] md:inline-flex"><Command className="h-2.5 w-2.5" />K</kbd>
      </Button>

      {open ? (
        <div
          className="fixed inset-0 z-50 flex items-start justify-center bg-slate-950/45 px-4 pt-[10vh] backdrop-blur-sm sm:pt-[14vh]"
          onMouseDown={(event) => { if (event.target === event.currentTarget) close(); }}
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-label="全局搜索"
            data-testid="global-search-dialog"
            className="w-full max-w-2xl overflow-hidden rounded-xl border bg-card shadow-2xl"
            onKeyDown={onDialogKeyDown}
          >
            <div className="flex items-center gap-3 border-b px-4">
              <Search className="h-5 w-5 shrink-0 text-muted-foreground" />
              <input
                autoFocus
                data-testid="global-search-input"
                aria-label="搜索虚拟机、网络、存储池或页面"
                value={query}
                onChange={(event) => { setQuery(event.target.value); setActiveIndex(0); }}
                placeholder="搜索虚拟机、IP、网络、存储池或页面..."
                className="h-14 min-w-0 flex-1 bg-transparent text-base outline-none placeholder:text-muted-foreground"
              />
              {loading ? <LoaderCircle className="h-4 w-4 animate-spin text-muted-foreground" /> : null}
              <button type="button" aria-label="关闭全局搜索" onClick={close} className="rounded-md p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground"><X className="h-4 w-4" /></button>
            </div>

            <div className="max-h-[min(58vh,520px)] overflow-y-auto p-2" data-testid="global-search-results">
              {loading && results.length === 0 ? <p className="px-4 py-10 text-center text-sm text-muted-foreground">正在读取可搜索资源...</p> : null}
              {!loading && deferredQuery.trim() && results.length === 0 ? (
                <div className="px-4 py-12 text-center" data-testid="global-search-empty">
                  <Search className="mx-auto h-6 w-6 text-muted-foreground" />
                  <p className="mt-3 text-sm font-medium">没有匹配结果</p>
                  <p className="mt-1 text-xs text-muted-foreground">尝试虚拟机名称、UUID、IP、网络桥或存储池路径。</p>
                </div>
              ) : null}
              {results.map((item, index) => {
                const showGroup = index === 0 || results[index - 1].group !== item.group;
                const active = index === activeIndex;
                return (
                  <div key={item.id}>
                    {showGroup ? <p className="px-3 pb-1 pt-3 text-[10px] font-medium uppercase tracking-[0.14em] text-muted-foreground">{item.group}</p> : null}
                    <button
                      type="button"
                      data-testid={item.testId}
                      className={cn("flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left transition-colors", active ? "bg-primary/10 text-foreground" : "text-foreground hover:bg-muted/60")}
                      onMouseEnter={() => setActiveIndex(index)}
                      onClick={() => openResult(item)}
                    >
                      <span className={cn("grid h-8 w-8 shrink-0 place-items-center rounded-md border", active ? "border-primary/30 bg-primary/10 text-primary" : "bg-muted/40 text-muted-foreground")}><ResultIcon kind={item.kind} /></span>
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm font-medium">{item.title}</span>
                        <span className={cn("mt-0.5 block truncate text-xs", item.description ? "text-muted-foreground" : "text-transparent")}>{item.description || "-"}</span>
                      </span>
                      {active ? <CornerDownLeft className="h-3.5 w-3.5 shrink-0 text-muted-foreground" /> : null}
                    </button>
                  </div>
                );
              })}
            </div>

            <div className="flex flex-wrap items-center gap-3 border-t bg-muted/20 px-4 py-2 text-[11px] text-muted-foreground">
              <span><kbd className="mr-1 rounded border bg-card px-1.5 py-0.5 font-mono">↑</kbd><kbd className="rounded border bg-card px-1.5 py-0.5 font-mono">↓</kbd> 选择</span>
              <span><kbd className="mr-1 rounded border bg-card px-1.5 py-0.5 font-mono">Enter</kbd> 打开</span>
              <span><kbd className="mr-1 rounded border bg-card px-1.5 py-0.5 font-mono">Esc</kbd> 关闭</span>
              {sourceError ? <span className="ml-auto text-warning">部分资源加载失败</span> : null}
            </div>
          </div>
        </div>
      ) : null}
    </>
  );
}

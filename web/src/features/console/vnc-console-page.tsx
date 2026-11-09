import { useEffect } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, ExternalLink, Monitor } from "lucide-react";
import type { ConsoleKind } from "@/api/types";
import { Button } from "@/components/ui/button";
import { useSessionQuery } from "@/features/auth/use-session";
import { VMConsolePanel } from "@/features/console/vm-console-panel";
import { useVMQuery } from "@/features/vms/use-vm-query";

export function VNCConsolePage({
  uuid,
  initialKind,
  detached = false,
}: {
  uuid: string;
  initialKind?: ConsoleKind;
  detached?: boolean;
}) {
  const session = useSessionQuery();
  const vmQuery = useVMQuery(uuid);
  const navigate = useNavigate();
  const vm = vmQuery.data;
  const readOnly = Boolean(session.data?.readOnly);
  const viewer = session.data?.user.role === "viewer";
  const canConnect = Boolean(session.data) && !readOnly && !viewer;
  const disabledReason = readOnly
    ? "只读模式下不能打开可交互控制台"
    : viewer
      ? "当前账号没有控制台权限"
      : "";

  useEffect(() => {
    if (session.isError) void navigate({ to: "/login" });
  }, [navigate, session.isError]);

  useEffect(() => {
    document.title = `${vm?.name ?? "虚拟机"} · 控制台 · VirtMgr`;
  }, [vm?.name]);

  if (!vm) {
    return (
      <div className="grid h-screen place-items-center bg-slate-950 text-sm text-slate-300" role="status">
        <div className="flex items-center gap-2"><Monitor className="h-4 w-4 animate-pulse" />正在读取虚拟机控制台信息…</div>
      </div>
    );
  }

  return (
    <div className="flex h-screen min-h-0 flex-col bg-slate-950 text-slate-100">
      <header className="flex min-h-14 shrink-0 items-center gap-2 border-b border-slate-800 bg-slate-900 px-3 sm:gap-3 sm:px-4" data-testid="vnc-console-header">
        {!detached ? (
          <Button asChild variant="ghost" size="icon" className="text-slate-200 hover:bg-slate-800 hover:text-white">
            <Link to="/vms/$uuid" params={{ uuid }} aria-label="返回虚拟机详情"><ArrowLeft className="h-5 w-5" /></Link>
          </Button>
        ) : null}
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold" data-testid="vnc-console-title">{vm.name}</p>
          <p className="truncate text-[10px] uppercase tracking-[0.16em] text-slate-400">Console · {uuid.slice(0, 8)}</p>
        </div>
        <div className="ml-auto flex items-center gap-2 text-xs text-slate-400">
          {detached ? <ExternalLink className="h-3.5 w-3.5" /> : <Link to="/vms/$uuid" params={{ uuid }} className="hover:text-white">返回虚拟机详情</Link>}
        </div>
      </header>
      <main className="min-h-0 flex-1">
        <VMConsolePanel
          uuid={uuid}
          graphics={vm.graphics}
          serials={vm.serials}
          vmState={vm.state}
          canConnect={canConnect}
          disabledReason={disabledReason}
          initialKind={initialKind}
          variant="standalone"
          detached={detached}
          autoConnect
        />
      </main>
    </div>
  );
}

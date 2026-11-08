import { useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import type RFB from "@novnc/novnc";
import { ArrowLeft, Expand, LoaderCircle, Monitor, RefreshCw, ShieldAlert } from "lucide-react";
import { ApiError, api } from "@/api/client";
import { Button } from "@/components/ui/button";
import { useSessionQuery } from "@/features/auth/use-session";
import { useVMQuery } from "@/features/vms/use-vm-query";
import { cn } from "@/lib/utils";

type ConsoleState = "waiting" | "forbidden" | "authorizing" | "connecting" | "connected" | "disconnected" | "error";

const stateText: Record<ConsoleState, string> = {
  waiting: "等待会话",
  forbidden: "无权访问",
  authorizing: "正在申请控制台凭证",
  connecting: "正在连接 VNC",
  connected: "已连接",
  disconnected: "连接已断开",
  error: "连接失败",
};

export function VNCConsolePage({ uuid }: { uuid: string }) {
  const session = useSessionQuery();
  const vmQuery = useVMQuery(uuid);
  const navigate = useNavigate();
  const screenRef = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<ConsoleState>("waiting");
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);

  const vm = vmQuery.data;
  const canConnect = Boolean(session.data && !session.data.readOnly && session.data.user.role !== "viewer");

  useEffect(() => {
    if (session.isError) void navigate({ to: "/login" });
  }, [navigate, session.isError]);

  useEffect(() => {
    if (!session.data || !vm) return;
    if (!canConnect) {
      setState("forbidden");
      return;
    }
    let disposed = false;
    let rfb: RFB | null = null;

    async function connect() {
      setError("");
      setState("authorizing");
      try {
        const ticket = await api.issueConsoleToken(uuid);
        if (disposed || !screenRef.current) return;
        setState("connecting");
        const { default: RFBClient } = await import("@novnc/novnc");
        if (disposed || !screenRef.current) return;
        const socketURL = new URL(ticket.path, window.location.href);
        socketURL.protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
        screenRef.current.replaceChildren();
        rfb = new RFBClient(screenRef.current, socketURL.toString(), { shared: true });
        rfb.scaleViewport = true;
        rfb.resizeSession = false;
        rfb.viewOnly = false;
        rfb.focusOnClick = true;
        rfb.showDotCursor = true;
        rfb.addEventListener("connect", () => { if (!disposed) setState("connected"); });
        rfb.addEventListener("disconnect", () => { if (!disposed) setState((current) => current === "connected" ? "disconnected" : current); });
        rfb.addEventListener("securityfailure", () => {
          if (!disposed) {
            setState("error");
            setError("VNC 安全协商失败，请刷新凭证后重试。");
          }
        });
      } catch (cause) {
        if (disposed) return;
        setState("error");
        setError(cause instanceof ApiError ? cause.message : cause instanceof Error ? cause.message : "无法建立控制台连接。");
      }
    }

    void connect();
    return () => {
      disposed = true;
      try {
        rfb?.disconnect();
      } catch {
        // noVNC can throw when the socket never opened; the page is being torn down anyway.
      }
    };
  }, [attempt, canConnect, session.data, uuid, vm]);

  async function toggleFullscreen() {
    try {
      if (document.fullscreenElement) await document.exitFullscreen();
      else await document.documentElement.requestFullscreen();
    } catch {
      setError("浏览器未允许进入全屏模式。");
    }
  }

  const busy = state === "authorizing" || state === "connecting";
  const failed = state === "error" || state === "forbidden" || state === "disconnected";

  return (
    <div className="flex h-screen min-h-0 flex-col bg-slate-950 text-slate-100">
      <header className="flex min-h-14 shrink-0 flex-wrap items-center gap-2 border-b border-slate-800 bg-slate-900 px-3 py-2 sm:gap-3 sm:px-4 md:flex-nowrap md:py-0" data-testid="vnc-console-header">
        <Button asChild variant="ghost" size="icon" className="text-slate-200 hover:bg-slate-800 hover:text-white">
          <Link to="/vms/$uuid" params={{ uuid }} aria-label="返回虚拟机详情"><ArrowLeft className="h-5 w-5" /></Link>
        </Button>
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold" data-testid="vnc-console-title">{vm?.name ?? "虚拟机控制台"}</p>
          <p className="truncate text-[10px] uppercase tracking-[0.16em] text-slate-400">VNC Console · {uuid.slice(0, 8)}</p>
        </div>
        <span
          data-testid="vnc-console-status"
          role="status"
          aria-live="polite"
          className={cn("ml-auto inline-flex h-7 items-center whitespace-nowrap rounded-full border px-2.5 text-xs sm:px-3",
            state === "connected" ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-300" :
              failed ? "border-rose-500/40 bg-rose-500/10 text-rose-300" :
                "border-slate-600 bg-slate-800 text-slate-300")}
        >
          {busy ? <LoaderCircle className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : <span className="mr-1.5 h-1.5 w-1.5 rounded-full bg-current" />}
          {stateText[state]}
        </span>
        <Button variant="ghost" size="icon" className="text-slate-200 hover:bg-slate-800 hover:text-white" onClick={() => void toggleFullscreen()} aria-label="全屏"><Expand className="h-4 w-4" /></Button>
        <Button variant="outline" size="sm" className="border-slate-600 bg-slate-800 text-slate-100 hover:bg-slate-700 hover:text-white" disabled={!canConnect || busy} onClick={() => setAttempt((value) => value + 1)} aria-label="重新连接控制台">
          <RefreshCw className={cn("h-4 w-4", busy && "animate-spin")} /><span className="hidden sm:inline">重新连接</span>
        </Button>
      </header>

      <main className="relative min-h-0 flex-1 overflow-hidden bg-black">
        <div ref={screenRef} data-testid="vnc-console-screen" className="absolute inset-0 flex items-center justify-center" />
        {state !== "connected" ? (
          <div className="pointer-events-none absolute inset-0 grid place-items-center bg-slate-950/70 p-6">
            <div className="pointer-events-auto w-full max-w-md rounded-xl border border-slate-700 bg-slate-900 p-6 text-center shadow-2xl">
              {state === "forbidden" ? <ShieldAlert className="mx-auto h-8 w-8 text-amber-300" /> : busy ? <LoaderCircle className="mx-auto h-8 w-8 animate-spin text-sky-300" /> : <Monitor className="mx-auto h-8 w-8 text-slate-300" />}
              <h1 className="mt-4 text-base font-semibold">{stateText[state]}</h1>
              <p className="mt-2 text-sm text-slate-400" data-testid="vnc-console-message">
                {state === "forbidden" ? (session.data?.readOnly ? "只读模式下不能打开可交互控制台。" : "当前账号没有打开虚拟机控制台的权限。") : error || (state === "connecting" ? "正在通过 libvirt 建立 VNC 通道，请稍候。" : state === "disconnected" ? "控制台连接已关闭，可重新申请一次性凭证。" : "正在准备安全控制台通道。")}
              </p>
              {failed && state !== "forbidden" ? <Button className="mt-5" onClick={() => setAttempt((value) => value + 1)}><RefreshCw className="h-4 w-4" />重新连接</Button> : null}
              <Button asChild variant="ghost" className="mt-2 text-slate-300 hover:bg-slate-800 hover:text-white"><Link to="/vms/$uuid" params={{ uuid }}>返回虚拟机详情</Link></Button>
            </div>
          </div>
        ) : null}
      </main>
    </div>
  );
}

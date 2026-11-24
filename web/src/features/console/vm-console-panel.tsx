import { useEffect, useRef, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
  AlertTriangle,
  ExternalLink,
  Expand,
  Keyboard,
  LoaderCircle,
  Maximize2,
  Monitor,
  RefreshCw,
  Send,
  Terminal,
  Unplug,
} from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { ConsoleKind, Graphics, SerialConsole, VMState } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  consoleKeyOptions,
  consolePageURL,
  consoleStateText,
  resolveConsoleKind,
  type ConsoleConnectionState,
  type VNCScaleMode,
} from "@/features/console/console-model";
import { consoleSocketOverallTimeoutMs, consoleSocketOverallTimeoutSeconds } from "@/features/console/console-socket";
import { useSerialSession } from "@/features/console/use-serial-session";
import { useVNCSession } from "@/features/console/use-vnc-session";
import { VNCInputPad } from "@/features/console/vnc-input-pad";
import { cn } from "@/lib/utils";

type ConsolePanelProps = {
  uuid: string;
  graphics: Graphics[];
  serials?: SerialConsole[];
  vmState: VMState;
  canConnect: boolean;
  disabledReason: string;
  initialKind?: ConsoleKind;
  variant?: "embedded" | "standalone";
  detached?: boolean;
  autoConnect?: boolean;
};

const EMPTY_SERIALS: SerialConsole[] = [];

function stateTone(state: ConsoleConnectionState): "success" | "warning" | "destructive" | "secondary" {
  if (state === "connected") return "success";
  if (state === "error") return "destructive";
  if (state === "authorizing" || state === "connecting") return "warning";
  return "secondary";
}

export function VMConsolePanel({
  uuid,
  graphics,
  serials,
  vmState,
  canConnect,
  disabledReason,
  initialKind,
  variant = "embedded",
  detached = false,
  autoConnect = true,
}: ConsolePanelProps) {
  const navigate = useNavigate();
  const rootRef = useRef<HTMLDivElement>(null);
  const [kind, setKind] = useState<ConsoleKind | null>(() => resolveConsoleKind(initialKind, graphics, serials));
  const [scaleMode, setScaleMode] = useState<VNCScaleMode>("fit");
  const [connectionRequested, setConnectionRequested] = useState(false);
  const [connectionStalled, setConnectionStalled] = useState(false);
  const [keyNotice, setKeyNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [inputOpen, setInputOpen] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const serialDevices = serials ?? EMPTY_SERIALS;
  const [serialDevice, setSerialDevice] = useState<string | undefined>(() => serialDevices[0]?.device);
  const activeKind = resolveConsoleKind(kind ?? undefined, graphics, serialDevices);
  const interactiveState = vmState === "running" || vmState === "blocked" || vmState === "paused";
  const canOpen = canConnect && interactiveState;
  const selectedSerial = serialDevices.find((device) => device.device === serialDevice) ?? serialDevices[0];
  const shareableConsoleURL = consolePageURL(uuid);

  const vnc = useVNCSession({
    uuid,
    enabled: connectionRequested && canOpen && activeKind === "vnc",
    scaleMode,
    screenTestId: variant === "standalone" ? "vnc-console-screen" : "vm-console-vnc-screen",
  });
  const serial = useSerialSession({
    uuid,
    device: selectedSerial?.device,
    enabled: connectionRequested && canOpen && activeKind === "serial",
  });
  const active = activeKind === "vnc" ? vnc : serial;
  const activeState = activeKind ? active.state : "idle";
  const activeError = activeKind ? active.error : "";
  const activeNote = activeKind === "vnc" ? vnc.connectionNote : "";
  const busy = activeState === "authorizing" || activeState === "connecting";
  const connected = activeState === "connected";
  const showBusy = busy && !connectionStalled;
  const statusText = connectionStalled
    ? "连接超时，未收到 WebSocket 握手"
    : activeError && activeState !== "connected"
      ? activeError
      : busy && activeNote
        ? activeNote
        : consoleStateText[activeState];

  useEffect(() => {
    if (!busy) {
      setConnectionStalled(false);
      return;
    }
    setConnectionStalled(false);
    const timeout = window.setTimeout(() => setConnectionStalled(true), consoleSocketOverallTimeoutMs + 5_000);
    return () => window.clearTimeout(timeout);
  }, [busy, activeKind, uuid]);

  useEffect(() => {
    const resolved = resolveConsoleKind(initialKind, graphics, serialDevices);
    setKind((current) => resolveConsoleKind(current ?? undefined, graphics, serialDevices) ?? resolved);
  }, [graphics, initialKind, serialDevices]);

  useEffect(() => {
    setSerialDevice((current) =>
      serialDevices.some((device) => device.device === current) ? current : serialDevices[0]?.device,
    );
  }, [serialDevices]);

  useEffect(() => {
    setConnectionRequested(canOpen && autoConnect);
  }, [autoConnect, canOpen, uuid]);

  useEffect(() => {
    function onFullscreenChange() {
      setFullscreen(Boolean(document.fullscreenElement));
    }
    document.addEventListener("fullscreenchange", onFullscreenChange);
    return () => document.removeEventListener("fullscreenchange", onFullscreenChange);
  }, []);

  useEffect(() => {
    if (activeKind !== "vnc" || !connected) setInputOpen(false);
  }, [activeKind, connected]);

  const sendKeyMutation = useMutation({
    mutationFn: (key: Parameters<typeof api.sendConsoleKey>[1]["key"]) => api.sendConsoleKey(uuid, { key }),
    onSuccess: (_result, key) => {
      const label = consoleKeyOptions.find((option) => option.value === key)?.label ?? key;
      setKeyNotice({ tone: "success", text: `已发送 ${label}` });
    },
    onError: (error) => {
      setKeyNotice({ tone: "error", text: error instanceof ApiError ? error.message : "发送按键失败" });
    },
  });

  function connect() {
    if (!activeKind) return;
    if (!connectionRequested) setConnectionRequested(true);
    else active.reconnect();
    setKeyNotice(null);
  }

  function disconnect() {
    active.disconnect();
    setKeyNotice(null);
  }

  async function toggleFullscreen() {
    try {
      if (document.fullscreenElement) await document.exitFullscreen();
      else await rootRef.current?.requestFullscreen();
    } catch {
      setKeyNotice({ tone: "error", text: "浏览器未允许进入全屏模式。" });
    }
  }

  function expandConsole() {
    if (!activeKind) return;
    void navigate({ to: "/console/vms/$uuid", params: { uuid }, search: { kind: activeKind } });
  }

  function detachConsole() {
    if (!activeKind) return;
    const params = new URLSearchParams({ kind: activeKind, detached: "true" });
    const opened = window.open(
      `/console/vms/${encodeURIComponent(uuid)}?${params.toString()}`,
      `virtmgr-console-${uuid}`,
      "popup,width=1280,height=860",
    );
    if (!opened) {
      setKeyNotice({ tone: "error", text: "独立窗口被浏览器拦截，请允许弹出窗口后重试。" });
      return;
    }
    disconnect();
  }

  const overlayText = !canOpen
    ? disabledReason || "当前虚拟机状态不支持打开控制台"
    : !activeKind
      ? "当前虚拟机没有配置图形或串口控制台"
      : activeState === "error"
        ? activeError || "控制台连接失败"
        : activeState === "disconnected"
          ? "控制台连接已断开"
          : "";

  return (
    <div
      ref={rootRef}
      className={cn("flex min-h-0 flex-col bg-slate-950 text-slate-100", variant === "standalone" ? "h-full" : "h-[25rem] sm:h-[30rem]")}
      data-testid="vm-console-panel"
      data-kind={activeKind ?? "none"}
      data-state={activeState}
      data-connection-stalled={connectionStalled}
    >
      <div className="flex flex-wrap items-center gap-2 border-b border-slate-800 bg-slate-900 px-3 py-2">
        <div className="inline-flex rounded-lg border border-slate-700 bg-slate-950 p-0.5" role="tablist" aria-label="控制台类型">
          <button
            type="button"
            role="tab"
            aria-selected={activeKind === "vnc"}
            data-testid="console-kind-vnc"
            disabled={!graphics.some((device) => device.type.toLowerCase() === "vnc")}
            onClick={() => { setKind("vnc"); setKeyNotice(null); }}
            className={cn("inline-flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-40", activeKind === "vnc" ? "bg-slate-700 text-white shadow-sm" : "text-slate-400 hover:text-slate-200")}
          >
            <Monitor className="h-3.5 w-3.5" />图形
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeKind === "serial"}
            data-testid="console-kind-serial"
            disabled={serialDevices.length === 0}
            title={serialDevices.length ? serialDevices[0].label : "当前虚拟机未配置 PTY 串口控制台"}
            onClick={() => { setKind("serial"); setKeyNotice(null); }}
            className={cn("inline-flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-40", activeKind === "serial" ? "bg-slate-700 text-white shadow-sm" : "text-slate-400 hover:text-slate-200")}
          >
            <Terminal className="h-3.5 w-3.5" />串口
          </button>
        </div>

        <Badge
          variant={connectionStalled ? "destructive" : stateTone(activeState)}
          data-testid={variant === "standalone" ? "vnc-console-status" : "vm-console-status"}
          role="status"
          aria-live="polite"
          className={cn("max-w-full truncate", activeState === "connected" && "border-emerald-500/40 bg-emerald-500/10 text-emerald-300")}
        >
          {showBusy ? <LoaderCircle className="h-3 w-3 animate-spin" /> : <span className="h-1.5 w-1.5 rounded-full bg-current" />}
          {statusText}
        </Badge>

        <div className="ml-auto flex flex-wrap items-center gap-1.5">
          {activeKind === "vnc" ? (
            <>
              <select
                aria-label="发送组合键"
                data-testid="vm-console-send-key"
                value=""
                disabled={!connected || sendKeyMutation.isPending}
                onChange={(event) => {
                  const key = event.target.value as Parameters<typeof api.sendConsoleKey>[1]["key"];
                  if (key) sendKeyMutation.mutate(key);
                }}
                className="h-8 rounded-md border border-slate-700 bg-slate-900 px-2 text-xs text-slate-200 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <option value="">发送按键</option>
                {consoleKeyOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
              <select
                aria-label="缩放模式"
                data-testid="vm-console-scale"
                value={scaleMode}
                disabled={!connected}
                onChange={(event) => setScaleMode(event.target.value as VNCScaleMode)}
                className="h-8 rounded-md border border-slate-700 bg-slate-900 px-2 text-xs text-slate-200 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <option value="fit">自适应</option>
                <option value="actual">原始大小</option>
                <option value="remote">远端调整</option>
              </select>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="text-slate-300 hover:bg-slate-800 hover:text-white"
                data-testid="vm-console-input-open"
                aria-label="打开 VNC 输入板"
                aria-haspopup="dialog"
                title="输入板：粘贴命令并逐字键入客户机"
                disabled={!connected}
                onClick={() => setInputOpen(true)}
              >
                <Keyboard className="h-3.5 w-3.5" /><span className="hidden sm:inline">输入板</span>
              </Button>
            </>
          ) : null}
          {activeKind === "serial" && serialDevices.length > 1 ? (
            <select
              aria-label="串口设备"
              data-testid="vm-console-serial-device"
              value={selectedSerial?.device ?? ""}
              onChange={(event) => {
                setSerialDevice(event.target.value);
                setKeyNotice(null);
              }}
              className="h-8 rounded-md border border-slate-700 bg-slate-900 px-2 text-xs text-slate-200"
            >
              {serialDevices.map((device, index) => (
                <option key={device.device ?? `serial-${index}`} value={device.device ?? ""}>
                  {device.label}
                </option>
              ))}
            </select>
          ) : null}
          {connectionRequested && (connected || busy) ? (
            <Button type="button" variant="outline" size="sm" data-testid="vm-console-disconnect" className="border-slate-700 bg-slate-900 text-slate-200 hover:bg-slate-800 hover:text-white" onClick={disconnect}>
              <Unplug className="h-3.5 w-3.5" />断开
            </Button>
          ) : (
            <Button type="button" size="sm" data-testid="vm-console-connect" disabled={!canOpen || !activeKind} onClick={connect}>
              <Send className="h-3.5 w-3.5" />连接
            </Button>
          )}
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label="重新连接控制台"
            title="重新连接控制台"
            disabled={!canOpen || !activeKind || busy}
            className="text-slate-300 hover:bg-slate-800 hover:text-white"
            onClick={() => { setConnectionRequested(true); active.reconnect(); }}
          >
            <RefreshCw className={cn("h-3.5 w-3.5", busy && "animate-spin")} />
          </Button>
          <Button type="button" variant="ghost" size="icon-sm" aria-label={fullscreen ? "退出全屏" : "全屏"} title={fullscreen ? "退出全屏" : "全屏"} className="text-slate-300 hover:bg-slate-800 hover:text-white" onClick={() => void toggleFullscreen()}>
            <Maximize2 className="h-3.5 w-3.5" />
          </Button>
          {variant === "embedded" ? (
            <Button type="button" variant="ghost" size="sm" className="text-slate-300 hover:bg-slate-800 hover:text-white" onClick={expandConsole} disabled={!activeKind}>
              <Expand className="h-3.5 w-3.5" />展开
            </Button>
          ) : null}
          {!detached && activeKind ? (
            <Button type="button" variant="ghost" size="sm" className="text-slate-300 hover:bg-slate-800 hover:text-white" onClick={detachConsole}>
              <ExternalLink className="h-3.5 w-3.5" />分离
            </Button>
          ) : null}
          {detached ? (
            <Button type="button" variant="ghost" size="sm" className="text-slate-300 hover:bg-slate-800 hover:text-white" onClick={() => window.close()}>
              关闭窗口
            </Button>
          ) : null}
        </div>
      </div>

      {keyNotice ? (
        <div className={cn("border-b px-3 py-1.5 text-xs", keyNotice.tone === "success" ? "border-emerald-500/20 bg-emerald-500/10 text-emerald-300" : "border-rose-500/20 bg-rose-500/10 text-rose-300")} role="status">
          {keyNotice.text}
        </div>
      ) : null}

      <div className="relative min-h-0 flex-1 overflow-hidden bg-black">
        {activeKind === "vnc" ? (
          <div ref={vnc.screenRef} data-testid={variant === "standalone" ? "vnc-console-screen" : "vm-console-vnc-screen"} className="absolute inset-0 flex items-center justify-center" />
        ) : null}
        {activeKind === "serial" ? (
          <div className="absolute inset-0 bg-slate-950 p-2">
            <div ref={serial.containerRef} className="h-full w-full" data-testid="vm-console-serial-screen" />
          </div>
        ) : null}

        {!connected ? (
          <div className="pointer-events-none absolute inset-0 grid place-items-center bg-slate-950/80 p-5">
            <div className="pointer-events-auto w-full max-w-md rounded-xl border border-slate-700 bg-slate-900/95 p-5 text-center shadow-2xl">
              {showBusy ? <LoaderCircle className="mx-auto h-7 w-7 animate-spin text-sky-300" /> : activeError || connectionStalled ? <AlertTriangle className="mx-auto h-7 w-7 text-rose-300" /> : <Monitor className="mx-auto h-7 w-7 text-slate-300" />}
              <p className="mt-3 text-sm text-slate-300" data-testid="vm-console-message">
                {showBusy
                  ? activeNote || `正在准备${activeKind === "serial" ? "串口" : "图形"}控制台，请稍候。`
                  : connectionStalled
                    ? "控制台 WebSocket 连接已超时。公网代理或 Cloudflare Tunnel 链路可能短暂抖动，请稍后重新连接。"
                    : overlayText || "控制台尚未连接。"}
              </p>
              {showBusy ? (
                <p className="mt-2 text-xs text-slate-500">链路较慢时会自动并行增加连接尝试，整体最多等待 {consoleSocketOverallTimeoutSeconds} 秒。</p>
              ) : null}
              {canOpen && activeKind && !showBusy ? (
                <Button type="button" size="sm" className="mt-4" onClick={connect}>
                  <RefreshCw className="h-3.5 w-3.5" />{activeState === "idle" ? "连接控制台" : "重新连接"}
                </Button>
              ) : null}
              {!interactiveState && canConnect ? <p className="mt-2 text-xs text-slate-500">虚拟机启动后即可连接控制台。</p> : null}
            </div>
          </div>
        ) : null}

        {activeKind === "vnc" && inputOpen ? (
          <VNCInputPad
            connected={connected}
            onClose={() => setInputOpen(false)}
            onType={vnc.typeText}
            onSendClipboard={vnc.copyToClipboard}
          />
        ) : null}
      </div>

      {activeKind === "vnc" && graphics.length ? (
        <details className="border-t border-slate-800 bg-slate-900/80 px-3 py-1.5 text-[11px] text-slate-400">
          <summary className="cursor-pointer select-none">连接信息</summary>
          <div className="mt-2 space-y-2">
            <div>
              <p className="text-slate-500">Web 控制台（远程访问使用）</p>
              <a href={shareableConsoleURL} target="_blank" rel="noreferrer" className="break-all font-mono text-sky-300 hover:text-sky-200 hover:underline">
                {shareableConsoleURL}
              </a>
            </div>
            <div>
              <p className="text-slate-500">宿主机 VNC 监听（仅供本机、VirtMgr 代理或 SSH 隧道）</p>
              <div className="mt-1 flex flex-wrap gap-3 font-mono text-slate-300">
                {graphics.filter((device) => device.type.toLowerCase() === "vnc").map((device, index) => (
                  <span key={index}>{device.listen || "*"}:{device.port || "-"}</span>
                ))}
              </div>
              <p className="mt-1 text-slate-500">远程客户端请使用上方 Web 控制台；如使用原生 VNC，请通过 SSH 隧道转发此地址，不要直接暴露到公网。</p>
            </div>
          </div>
        </details>
      ) : null}
    </div>
  );
}

import { useCallback, useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { ApiError, api } from "@/api/client";
import type { ConsoleConnectionState } from "@/features/console/console-model";
import { consoleWebSocketURL } from "@/features/console/console-model";
import { consoleSocketTimeoutMs } from "@/features/console/console-socket";

export function useSerialSession({
  uuid,
  device,
  enabled,
}: {
  uuid: string;
  device?: string;
  enabled: boolean;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const socketRef = useRef<WebSocket | null>(null);
  const [state, setState] = useState<ConsoleConnectionState>("idle");
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);

  const reconnect = useCallback(() => setAttempt((value) => value + 1), []);
  const disconnect = useCallback(() => {
    socketRef.current?.close(1000, "user disconnected");
    socketRef.current = null;
    setState("disconnected");
  }, []);

  useEffect(() => {
    if (!enabled) {
      setState("idle");
      setError("");
      return;
    }
    const container = containerRef.current;
    if (!container) return;
    container.replaceChildren();
    const terminal = new Terminal({
      cursorBlink: true,
      convertEol: true,
      fontFamily: '"JetBrains Mono", "SFMono-Regular", Consolas, monospace',
      fontSize: 13,
      lineHeight: 1.15,
      scrollback: 5000,
      theme: {
        background: "#020617",
        foreground: "#e2e8f0",
        cursor: "#38bdf8",
        selectionBackground: "#1e3a5f",
        black: "#0f172a",
        brightBlack: "#64748b",
      },
    });
    const fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);
    terminal.open(container);
    terminalRef.current = terminal;
    const resize = () => {
      try {
        fitAddon.fit();
      } catch {
        // The terminal can be disposed while a resize observer callback is queued.
      }
    };
    const observer = new ResizeObserver(resize);
    observer.observe(container);
    requestAnimationFrame(resize);

    let disposed = false;
    let socket: WebSocket | null = null;
    const inputDisposable = terminal.onData((data) => {
      if (socket?.readyState === WebSocket.OPEN) {
        socket.send(new TextEncoder().encode(data));
      }
    });

    async function connect() {
      setError("");
      setState("authorizing");
      try {
        const ticket = await api.issueConsoleToken(uuid, { kind: "serial", device });
        if (disposed) return;
        setState("connecting");
        socket = new WebSocket(consoleWebSocketURL(ticket.path));
        socket.binaryType = "arraybuffer";
        socketRef.current = socket;
        let opened = false;
        const timeout = window.setTimeout(() => {
          if (disposed || opened) return;
          setState("error");
          setError("串口控制台 WebSocket 连接超时。若当前地址是 IP 且使用自签名证书，浏览器可能只允许页面访问却拦截 wss；请改用受信任的 HTTPS 域名，刷新页面后重试。");
          socket?.close();
        }, consoleSocketTimeoutMs);
        socket.addEventListener("open", () => {
          if (disposed) return;
          opened = true;
          window.clearTimeout(timeout);
          setState("connected");
          terminal.focus();
          resize();
        });
        socket.addEventListener("message", (event) => {
          if (disposed) return;
          if (typeof event.data === "string") terminal.write(event.data);
          else terminal.write(new Uint8Array(event.data as ArrayBuffer));
        });
        socket.addEventListener("close", () => {
          if (disposed) return;
          window.clearTimeout(timeout);
          setState("disconnected");
          socketRef.current = null;
        });
        socket.addEventListener("error", () => {
          if (disposed) return;
          window.clearTimeout(timeout);
          setState("error");
          setError("串口控制台 WebSocket 建连失败。请确认浏览器信任当前 HTTPS 证书、网络或反向代理允许 WebSocket，设备也可能正被其他会话占用。");
        });
      } catch (cause) {
        if (disposed) return;
        setState("error");
        setError(cause instanceof ApiError ? cause.message : cause instanceof Error ? cause.message : "无法建立串口连接。");
      }
    }

    void connect();
    return () => {
      disposed = true;
      socket?.close();
      if (socketRef.current === socket) socketRef.current = null;
      inputDisposable.dispose();
      observer.disconnect();
      terminalRef.current = null;
      terminal.dispose();
    };
  }, [attempt, device, enabled, uuid]);

  return { containerRef, state, error, reconnect, disconnect };
}

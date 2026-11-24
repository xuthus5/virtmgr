import { useCallback, useEffect, useRef, useState } from "react";
import type RFB from "@novnc/novnc";
import { ApiError, api } from "@/api/client";
import { analyzeConsoleTextInput, CONSOLE_SHIFT_KEYSYM } from "@/features/console/console-input";
import type { ConsoleConnectionState, VNCScaleMode } from "@/features/console/console-model";
import { consoleWebSocketURL } from "@/features/console/console-model";
import { openConsoleSocket } from "@/features/console/console-socket";

type VNCStateEvent = CustomEvent<{ clean?: boolean }>;
type VNCSecurityEvent = CustomEvent<{ reason?: string }>;

const DEFAULT_TYPE_DELAY_MS = 8;

type TypeTextOptions = {
  signal?: AbortSignal;
  delayMs?: number;
  onProgress?: (sent: number, total: number) => void;
};

function typeDelay(ms: number) {
  return new Promise<void>((resolve) => window.setTimeout(resolve, ms));
}

function typeAbortError() {
  return new DOMException("已停止键入", "AbortError");
}

export function useVNCSession({
  uuid,
  enabled,
  scaleMode,
  screenTestId,
}: {
  uuid: string;
  enabled: boolean;
  scaleMode: VNCScaleMode;
  screenTestId: string;
}) {
  const screenRef = useRef<HTMLDivElement>(null);
  const rfbRef = useRef<RFB | null>(null);
  const connectedRef = useRef(false);
  const typingRef = useRef(false);
  const scaleModeRef = useRef(scaleMode);
  scaleModeRef.current = scaleMode;
  const [state, setState] = useState<ConsoleConnectionState>("idle");
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);

  const reconnect = useCallback(() => setAttempt((value) => value + 1), []);
  const disconnect = useCallback(() => {
    connectedRef.current = false;
    try {
      rfbRef.current?.disconnect();
    } catch {
      // The socket may already be closed.
    }
    rfbRef.current = null;
    setState("disconnected");
  }, []);

  const typeText = useCallback(async (text: string, options: TypeTextOptions = {}) => {
    const rfb = rfbRef.current;
    if (!rfb || !connectedRef.current) throw new Error("VNC 控制台尚未连接。");
    if (typingRef.current) throw new Error("已有键入任务正在进行。");
    const analysis = analyzeConsoleTextInput(text);
    if (analysis.unsupported.length > 0) throw new Error("键入内容包含非 ASCII 字符。");
    if (analysis.keystrokes.length === 0) return 0;

    typingRef.current = true;
    const delayMs = Math.max(0, options.delayMs ?? DEFAULT_TYPE_DELAY_MS);
    try {
      for (let index = 0; index < analysis.keystrokes.length; index += 1) {
        if (options.signal?.aborted) throw typeAbortError();
        if (!connectedRef.current) throw new Error("VNC 控制台连接已断开。");
        const keystroke = analysis.keystrokes[index];
        if (keystroke.shift) rfb.sendKey(CONSOLE_SHIFT_KEYSYM, "ShiftLeft", true);
        rfb.sendKey(keystroke.keysym, keystroke.code, true);
        rfb.sendKey(keystroke.keysym, keystroke.code, false);
        if (keystroke.shift) rfb.sendKey(CONSOLE_SHIFT_KEYSYM, "ShiftLeft", false);
        options.onProgress?.(index + 1, analysis.keystrokes.length);
        if (delayMs > 0 && index < analysis.keystrokes.length - 1) await typeDelay(delayMs);
      }
      rfb.focus();
      return analysis.keystrokes.length;
    } finally {
      typingRef.current = false;
    }
  }, []);

  const copyToClipboard = useCallback((text: string) => {
    const rfb = rfbRef.current;
    if (!rfb || !connectedRef.current) return false;
    rfb.clipboardPasteFrom(text);
    rfb.focus();
    return true;
  }, []);

  useEffect(() => {
    if (!enabled) {
      connectedRef.current = false;
      setState("idle");
      setError("");
      return;
    }
    let disposed = false;
    let rfb: RFB | null = null;

    async function connect() {
      connectedRef.current = false;
      setError("");
      setState("authorizing");
      try {
        const ticket = await api.issueConsoleToken(uuid, { kind: "vnc" });
        if (disposed || !screenRef.current) return;
        setState("connecting");
        const { default: RFBClient } = await import("@novnc/novnc");
        if (disposed || !screenRef.current) return;
        const socket = await openConsoleSocket(consoleWebSocketURL(ticket.path));
        if (disposed || !screenRef.current) {
          socket.close();
          return;
        }
        screenRef.current.replaceChildren();
        rfb = new RFBClient(screenRef.current, socket, { shared: true });
        rfb.scaleViewport = scaleModeRef.current === "fit";
        rfb.resizeSession = scaleModeRef.current === "remote";
        rfb.clipViewport = scaleModeRef.current !== "fit";
        rfb.dragViewport = scaleModeRef.current !== "fit";
        rfb.viewOnly = false;
        rfb.focusOnClick = true;
        rfb.showDotCursor = true;
        rfb.addEventListener("connect", () => {
          connectedRef.current = true;
          if (!disposed) setState("connected");
        });
        rfb.addEventListener("disconnect", (_event: Event) => {
          connectedRef.current = false;
          const event = _event as VNCStateEvent;
          if (disposed) return;
          setState("disconnected");
          if (!event.detail.clean) setError("控制台连接已中断，请重新连接。");
        });
        rfb.addEventListener("securityfailure", (event: Event) => {
          connectedRef.current = false;
          if (disposed) return;
          const failure = event as VNCSecurityEvent;
          setState("error");
          setError(failure.detail.reason || "VNC 安全协商失败，请重新连接。");
        });
        rfbRef.current = rfb;
      } catch (cause) {
        connectedRef.current = false;
        if (disposed) return;
        setState("error");
        setError(cause instanceof ApiError ? cause.message : cause instanceof Error ? cause.message : "无法建立 VNC 连接。");
      }
    }

    void connect();
    return () => {
      disposed = true;
      connectedRef.current = false;
      try {
        rfb?.disconnect();
      } catch {
        // noVNC can throw when the socket never opened.
      }
      if (rfbRef.current === rfb) rfbRef.current = null;
      screenRef.current?.replaceChildren();
    };
  }, [attempt, enabled, uuid]);

  useEffect(() => {
    if (!rfbRef.current) return;
    rfbRef.current.scaleViewport = scaleMode === "fit";
    rfbRef.current.resizeSession = scaleMode === "remote";
    rfbRef.current.clipViewport = scaleMode !== "fit";
    rfbRef.current.dragViewport = scaleMode !== "fit";
  }, [scaleMode]);

  return { screenRef, state, error, reconnect, disconnect, typeText, copyToClipboard, screenTestId };
}

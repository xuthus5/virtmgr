import { useCallback, useEffect, useRef, useState } from "react";
import type RFB from "@novnc/novnc";
import { ApiError, api } from "@/api/client";
import type { ConsoleConnectionState, VNCScaleMode } from "@/features/console/console-model";
import { consoleWebSocketURL } from "@/features/console/console-model";
import { openConsoleSocket } from "@/features/console/console-socket";

type VNCStateEvent = CustomEvent<{ clean?: boolean }>;
type VNCSecurityEvent = CustomEvent<{ reason?: string }>;

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
  const scaleModeRef = useRef(scaleMode);
  scaleModeRef.current = scaleMode;
  const [state, setState] = useState<ConsoleConnectionState>("idle");
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);

  const reconnect = useCallback(() => setAttempt((value) => value + 1), []);
  const disconnect = useCallback(() => {
    try {
      rfbRef.current?.disconnect();
    } catch {
      // The socket may already be closed.
    }
    rfbRef.current = null;
    setState("disconnected");
  }, []);

  useEffect(() => {
    if (!enabled) {
      setState("idle");
      setError("");
      return;
    }
    let disposed = false;
    let rfb: RFB | null = null;

    async function connect() {
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
          if (!disposed) setState("connected");
        });
        rfb.addEventListener("disconnect", (_event: Event) => {
          const event = _event as VNCStateEvent;
          if (disposed) return;
          setState("disconnected");
          if (!event.detail.clean) setError("控制台连接已中断，请重新连接。");
        });
        rfb.addEventListener("securityfailure", (event: Event) => {
          if (disposed) return;
          const failure = event as VNCSecurityEvent;
          setState("error");
          setError(failure.detail.reason || "VNC 安全协商失败，请重新连接。");
        });
        rfbRef.current = rfb;
      } catch (cause) {
        if (disposed) return;
        setState("error");
        setError(cause instanceof ApiError ? cause.message : cause instanceof Error ? cause.message : "无法建立 VNC 连接。");
      }
    }

    void connect();
    return () => {
      disposed = true;
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

  return { screenRef, state, error, reconnect, disconnect, screenTestId };
}

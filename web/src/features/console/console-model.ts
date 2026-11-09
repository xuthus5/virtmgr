import type { ConsoleKeyAction, ConsoleKind, Graphics, SerialConsole } from "@/api/types";

export type ConsoleConnectionState = "idle" | "authorizing" | "connecting" | "connected" | "disconnected" | "error";

export type VNCScaleMode = "fit" | "actual" | "remote";

export const consoleStateText: Record<ConsoleConnectionState, string> = {
  idle: "未连接",
  authorizing: "正在申请凭证",
  connecting: "正在连接",
  connected: "已连接",
  disconnected: "已断开",
  error: "连接失败",
};

export const consoleKeyOptions: Array<{ value: ConsoleKeyAction; label: string }> = [
  { value: "ctrl-alt-del", label: "Ctrl + Alt + Del" },
  { value: "ctrl-alt-backspace", label: "Ctrl + Alt + Backspace" },
  ...Array.from({ length: 12 }, (_, index) => ({
    value: `ctrl-alt-f${index + 1}` as ConsoleKeyAction,
    label: `Ctrl + Alt + F${index + 1}`,
  })),
];

export function hasVNCConsole(graphics: Graphics[] | undefined): boolean {
  return Boolean(graphics?.some((device) => device.type.toLowerCase() === "vnc"));
}

export function defaultConsoleKind(graphics: Graphics[] | undefined, serials: SerialConsole[] | undefined): ConsoleKind | null {
  if (hasVNCConsole(graphics)) return "vnc";
  if (serials?.length) return "serial";
  return null;
}

export function resolveConsoleKind(
  requested: ConsoleKind | undefined,
  graphics: Graphics[] | undefined,
  serials: SerialConsole[] | undefined,
): ConsoleKind | null {
  if (requested === "vnc" && hasVNCConsole(graphics)) return "vnc";
  if (requested === "serial" && serials?.length) return "serial";
  return defaultConsoleKind(graphics, serials);
}

export function consoleWebSocketURL(path: string): string {
  const socketURL = new URL(path, window.location.href);
  socketURL.protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return socketURL.toString();
}

export function consoleKindLabel(kind: ConsoleKind): string {
  return kind === "vnc" ? "图形控制台" : "串口控制台";
}

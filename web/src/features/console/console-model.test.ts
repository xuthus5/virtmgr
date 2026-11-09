import { describe, expect, it } from "vitest";
import { consoleKeyOptions, consoleWebSocketURL, defaultConsoleKind, hasVNCConsole, resolveConsoleKind } from "@/features/console/console-model";

describe("console model", () => {
  it("prefers VNC and falls back to a serial console", () => {
    const serials = [{ device: "serial0", label: "Serial 1", targetPort: 0 }];
    expect(hasVNCConsole([{ type: "spice" }])).toBe(false);
    expect(defaultConsoleKind([{ type: "vnc" }], serials)).toBe("vnc");
    expect(defaultConsoleKind([{ type: "spice" }], serials)).toBe("serial");
    expect(defaultConsoleKind([], [])).toBeNull();
    expect(resolveConsoleKind("serial", [{ type: "vnc" }], serials)).toBe("serial");
    expect(resolveConsoleKind("serial", [{ type: "vnc" }], [])).toBe("vnc");
  });

  it("builds a WebSocket URL from the current page protocol", () => {
    const url = consoleWebSocketURL("/api/v1/vms/demo/console?token=test");
    expect(url.startsWith("ws://")).toBe(true);
    expect(url).toContain("/api/v1/vms/demo/console?token=test");
  });

  it("exposes Cockpit-style key combinations", () => {
    expect(consoleKeyOptions[0]).toEqual({ value: "ctrl-alt-del", label: "Ctrl + Alt + Del" });
    expect(consoleKeyOptions).toHaveLength(14);
    expect(consoleKeyOptions.at(-1)?.value).toBe("ctrl-alt-f12");
  });
});

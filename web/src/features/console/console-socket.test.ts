import { afterEach, describe, expect, it, vi } from "vitest";
import { ConsoleSocketError, openConsoleSocket } from "@/features/console/console-socket";

class FakeSocket extends EventTarget {
  binaryType: BinaryType = "blob";
  readyState = 0;
  close = vi.fn(() => {
    this.readyState = 3;
  });

  open() {
    this.readyState = 1;
    this.dispatchEvent(new Event("open"));
  }

  error() {
    this.dispatchEvent(new Event("error"));
  }
}

function fakeSocket() {
  const socket = new FakeSocket();
  return { socket, createSocket: () => socket as unknown as WebSocket };
}

afterEach(() => {
  vi.useRealTimers();
});

describe("openConsoleSocket", () => {
  it("resolves once the WebSocket opens", async () => {
    const { socket, createSocket } = fakeSocket();
    const pending = openConsoleSocket("wss://example.test/console", { createSocket });
    socket.open();
    await expect(pending).resolves.toBe(socket);
  });

  it("turns a stalled handshake into an actionable timeout", async () => {
    vi.useFakeTimers();
    const { socket, createSocket } = fakeSocket();
    const pending = openConsoleSocket("wss://example.test/console", { timeoutMs: 1_000, createSocket });
    const expectation = expect(pending).rejects.toEqual(expect.any(ConsoleSocketError));
    const message = expect(pending).rejects.toThrow("自签名证书");
    await vi.advanceTimersByTimeAsync(1_000);
    await expectation;
    await message;
    expect(socket.close).toHaveBeenCalledOnce();
  });

  it("reports WebSocket errors before the socket opens", async () => {
    const { socket, createSocket } = fakeSocket();
    const pending = openConsoleSocket("wss://example.test/console", { createSocket });
    const expectation = expect(pending).rejects.toThrow("WebSocket 建连失败");
    socket.error();
    await expectation;
    expect(socket.close).toHaveBeenCalledOnce();
  });
});

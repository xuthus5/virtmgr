import { afterEach, describe, expect, it, vi } from "vitest";
import { ConsoleSocketError, openConsoleSocket, openHedgedConsoleSocket } from "@/features/console/console-socket";

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

  it("keeps waiting beyond 15 seconds for a slow public tunnel handshake", async () => {
    vi.useFakeTimers();
    const { socket, createSocket } = fakeSocket();
    const pending = openConsoleSocket("wss://example.test/console", { createSocket });
    await vi.advanceTimersByTimeAsync(16_000);
    socket.open();
    await expect(pending).resolves.toBe(socket);
  });

  it("turns a stalled handshake into an actionable timeout", async () => {
    vi.useFakeTimers();
    const { socket, createSocket } = fakeSocket();
    const pending = openConsoleSocket("wss://example.test/console", { timeoutMs: 1_000, createSocket });
    const expectation = expect(pending).rejects.toEqual(expect.any(ConsoleSocketError));
    const message = expect(pending).rejects.toThrow("Cloudflare Tunnel 链路可能短暂抖动");
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

describe("openHedgedConsoleSocket", () => {
  it("does not start a second attempt when the first socket opens quickly", async () => {
    vi.useFakeTimers();
    const { socket, createSocket } = fakeSocket();
    const factory = vi.fn(async () => createSocket());
    const pending = openHedgedConsoleSocket(factory, {
      hedgeDelaysMs: [0, 1_000],
      timeoutMs: 5_000,
      overallTimeoutMs: 10_000,
    });
    await vi.advanceTimersByTimeAsync(0);
    socket.open();
    await expect(pending).resolves.toBe(socket);
    await vi.advanceTimersByTimeAsync(1_000);
    expect(factory).toHaveBeenCalledOnce();
  });

  it("hedges a stalled handshake and keeps the first socket that opens", async () => {
    vi.useFakeTimers();
    const first = new FakeSocket();
    const second = new FakeSocket();
    const sockets = [first, second];
    const factory = vi.fn(async () => sockets.shift() as unknown as WebSocket);
    const pending = openHedgedConsoleSocket(factory, {
      hedgeDelaysMs: [0, 2_000],
      timeoutMs: 10_000,
      overallTimeoutMs: 20_000,
    });
    await vi.advanceTimersByTimeAsync(2_000);
    expect(factory).toHaveBeenCalledTimes(2);
    second.open();
    await expect(pending).resolves.toBe(second);
    expect(first.close).toHaveBeenCalledOnce();
  });

  it("reports a distinct overall timeout after all attempts stall", async () => {
    vi.useFakeTimers();
    const first = new FakeSocket();
    const second = new FakeSocket();
    const sockets = [first, second];
    const factory = vi.fn(async () => sockets.shift() as unknown as WebSocket);
    const pending = openHedgedConsoleSocket(factory, {
      hedgeDelaysMs: [0, 2_000],
      timeoutMs: 20_000,
      overallTimeoutMs: 5_000,
    });
    const expectation = expect(pending).rejects.toThrow("5 秒内尝试了 2 条连接");
    await vi.advanceTimersByTimeAsync(5_000);
    await expectation;
    expect(first.close).toHaveBeenCalledOnce();
    expect(second.close).toHaveBeenCalledOnce();
  });

  it("stops waiting as soon as the caller aborts", async () => {
    vi.useFakeTimers();
    const { socket, createSocket } = fakeSocket();
    const controller = new AbortController();
    const pending = openHedgedConsoleSocket(async () => createSocket(), {
      hedgeDelaysMs: [0, 2_000],
      timeoutMs: 10_000,
      overallTimeoutMs: 20_000,
      signal: controller.signal,
    });
    await vi.advanceTimersByTimeAsync(0);
    controller.abort();
    await expect(pending).rejects.toThrow("控制台连接已取消");
    expect(socket.close).toHaveBeenCalledOnce();
  });
});

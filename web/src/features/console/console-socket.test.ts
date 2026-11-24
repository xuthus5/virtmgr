import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/api/client";
import { ConsoleSocketError, openConsoleSocket, openRetryingConsoleSocket } from "@/features/console/console-socket";

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

describe("openRetryingConsoleSocket", () => {
  it("does not start a second attempt when the first socket opens quickly", async () => {
    vi.useFakeTimers();
    const { socket, createSocket } = fakeSocket();
    const factory = vi.fn(async () => createSocket());
    const pending = openRetryingConsoleSocket(factory, {
      maxAttempts: 2,
      retryDelaysMs: [1_000],
      attemptTimeoutMs: 5_000,
      overallTimeoutMs: 10_000,
      random: () => 0.5,
    });
    await vi.advanceTimersByTimeAsync(0);
    socket.open();
    await expect(pending).resolves.toBe(socket);
    await vi.advanceTimersByTimeAsync(1_000);
    expect(factory).toHaveBeenCalledOnce();
  });

  it("retries sequentially after a retryable handshake failure", async () => {
    vi.useFakeTimers();
    const first = new FakeSocket();
    const second = new FakeSocket();
    const sockets = [first, second];
    const factory = vi.fn(async () => sockets.shift() as unknown as WebSocket);
    const onRetry = vi.fn();
    const pending = openRetryingConsoleSocket(factory, {
      maxAttempts: 2,
      retryDelaysMs: [2_000],
      attemptTimeoutMs: 10_000,
      overallTimeoutMs: 20_000,
      random: () => 0.5,
      onRetry,
    });
    await vi.advanceTimersByTimeAsync(0);
    first.error();
    await vi.advanceTimersByTimeAsync(2_000);
    expect(factory).toHaveBeenCalledTimes(2);
    expect(onRetry).toHaveBeenCalledWith(1, 2, 2_000, expect.any(ConsoleSocketError));
    second.open();
    await expect(pending).resolves.toBe(second);
    expect(first.close).toHaveBeenCalledOnce();
  });

  it("does not retry deterministic API errors", async () => {
    vi.useFakeTimers();
    const factory = vi.fn(async () => {
      throw new ApiError(403, "FORBIDDEN", "没有控制台权限");
    });
    const pending = openRetryingConsoleSocket(factory, {
      maxAttempts: 2,
      retryDelaysMs: [2_000],
      attemptTimeoutMs: 10_000,
      overallTimeoutMs: 20_000,
      random: () => 0.5,
    });
    await expect(pending).rejects.toThrow("没有控制台权限");
    expect(factory).toHaveBeenCalledOnce();
  });

  it("reports the attempt count after retry exhaustion", async () => {
    vi.useFakeTimers();
    const first = new FakeSocket();
    const second = new FakeSocket();
    const sockets = [first, second];
    const factory = vi.fn(async () => sockets.shift() as unknown as WebSocket);
    const pending = openRetryingConsoleSocket(factory, {
      maxAttempts: 2,
      retryDelaysMs: [2_000],
      attemptTimeoutMs: 10_000,
      overallTimeoutMs: 20_000,
      random: () => 0.5,
    });
    const expectation = expect(pending).rejects.toThrow("已尝试 2 次");
    await vi.advanceTimersByTimeAsync(0);
    first.error();
    await vi.advanceTimersByTimeAsync(2_000);
    second.error();
    await expectation;
    expect(factory).toHaveBeenCalledTimes(2);
  });

  it("reports a distinct overall timeout while an attempt is still pending", async () => {
    vi.useFakeTimers();
    const first = new FakeSocket();
    const second = new FakeSocket();
    const sockets = [first, second];
    const factory = vi.fn(async () => sockets.shift() as unknown as WebSocket);
    const pending = openRetryingConsoleSocket(factory, {
      maxAttempts: 2,
      retryDelaysMs: [2_000],
      attemptTimeoutMs: 20_000,
      overallTimeoutMs: 5_000,
      random: () => 0.5,
    });
    const expectation = expect(pending).rejects.toThrow("5 秒内未能完成 2 次尝试");
    await vi.advanceTimersByTimeAsync(5_000);
    await expectation;
    expect(first.close).toHaveBeenCalledOnce();
  });

  it("stops waiting as soon as the caller aborts", async () => {
    vi.useFakeTimers();
    const { socket, createSocket } = fakeSocket();
    const controller = new AbortController();
    const pending = openRetryingConsoleSocket(async () => createSocket(), {
      maxAttempts: 2,
      retryDelaysMs: [2_000],
      attemptTimeoutMs: 10_000,
      overallTimeoutMs: 20_000,
      signal: controller.signal,
    });
    await vi.advanceTimersByTimeAsync(0);
    controller.abort();
    await expect(pending).rejects.toThrow("控制台连接已取消");
    expect(socket.close).toHaveBeenCalledOnce();
  });
});

import { ApiError } from "@/api/client";

const consoleSocketAttemptTimeoutMs = 45_000;
export const consoleSocketOverallTimeoutMs = 100_000;
export const consoleSocketOverallTimeoutSeconds = consoleSocketOverallTimeoutMs / 1_000;
const consoleSocketMaxAttempts = 2;
const consoleSocketRetryDelaysMs = [2_000] as const;
const consoleSocketRetryJitterRatio = 0.2;

type ConsoleSocketOptions = {
  timeoutMs?: number;
  createSocket?: (url: string) => WebSocket;
};

type RetryingConsoleSocketOptions = {
  attemptTimeoutMs?: number;
  overallTimeoutMs?: number;
  maxAttempts?: number;
  retryDelaysMs?: readonly number[];
  signal?: AbortSignal;
  random?: () => number;
  onAttempt?: (attempt: number, total: number) => void;
  onRetry?: (failedAttempt: number, total: number, delayMs: number, error: unknown) => void;
};

type ConsoleSocketErrorOptions = ErrorOptions & { retryable?: boolean };

export class ConsoleSocketError extends Error {
  readonly retryable: boolean;

  constructor(message: string, options: ConsoleSocketErrorOptions = {}) {
    super(message, options);
    this.name = "ConsoleSocketError";
    this.retryable = options.retryable ?? false;
  }
}

export function openConsoleSocket(url: string, options: ConsoleSocketOptions = {}): Promise<WebSocket> {
  const createSocket = options.createSocket ?? ((target: string) => new WebSocket(target));
  let socket: WebSocket;
  try {
    socket = createSocket(url);
  } catch (cause) {
    return Promise.reject(new ConsoleSocketError("无法创建控制台 WebSocket 连接。", { cause }));
  }
  return waitForConsoleSocket(socket, options.timeoutMs ?? consoleSocketAttemptTimeoutMs);
}

function waitForConsoleSocket(socket: WebSocket, timeoutMs: number): Promise<WebSocket> {
  socket.binaryType = "arraybuffer";

  return new Promise((resolve, reject) => {
    let settled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const cleanup = () => {
      if (timer) clearTimeout(timer);
      socket.removeEventListener("open", handleOpen);
      socket.removeEventListener("error", handleError);
      socket.removeEventListener("close", handleClose);
    };
    const fail = (message: string) => {
      if (settled) return;
      settled = true;
      cleanup();
      try {
        socket.close();
      } catch {
        // The socket may already be closed or never have opened.
      }
      reject(new ConsoleSocketError(message, { retryable: true }));
    };
    const handleOpen = () => {
      if (settled) return;
      settled = true;
      cleanup();
      resolve(socket);
    };
    const handleError = () => {
      fail("控制台 WebSocket 建连失败。请确认网络或反向代理允许 WebSocket；若当前地址是 IP 且使用自签名证书，也请改用受信任的 HTTPS 域名。");
    };
    const handleClose = (event: CloseEvent) => {
      const detail = event.code ? `（code ${event.code}${event.reason ? `，${event.reason}` : ""}）` : "";
      fail(`控制台 WebSocket 在握手前关闭${detail}，请刷新后重试。`);
    };

    socket.addEventListener("open", handleOpen);
    socket.addEventListener("error", handleError);
    socket.addEventListener("close", handleClose);
    timer = setTimeout(() => {
      fail(`控制台 WebSocket 连接超时（${timeoutMs / 1_000} 秒内未完成握手）。公网代理或 Cloudflare Tunnel 链路可能短暂抖动，请稍后重试。`);
    }, timeoutMs);

    if (socket.readyState === WebSocket.OPEN) {
      handleOpen();
    } else if (socket.readyState !== WebSocket.CONNECTING) {
      fail("控制台 WebSocket 已关闭，无法建立连接。请刷新页面后重试。");
    }
  });
}

export function openRetryingConsoleSocket(
  createSocket: (attempt: number) => WebSocket | Promise<WebSocket>,
  options: RetryingConsoleSocketOptions = {},
): Promise<WebSocket> {
  const timeoutMs = options.attemptTimeoutMs ?? consoleSocketAttemptTimeoutMs;
  const overallTimeoutMs = options.overallTimeoutMs ?? consoleSocketOverallTimeoutMs;
  const total = Math.max(1, options.maxAttempts ?? consoleSocketMaxAttempts);
  const retryDelaysMs = options.retryDelaysMs ?? consoleSocketRetryDelaysMs;

  return new Promise((resolve, reject) => {
    let settled = false;
    let currentSocket: WebSocket | null = null;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    const random = options.random ?? Math.random;
    const overallTimer = setTimeout(() => {
      fail(new ConsoleSocketError(
        `控制台 WebSocket 连接超时：${overallTimeoutMs / 1_000} 秒内未能完成 ${total} 次尝试。当前到公网入口的链路不稳定，请切换 Wi-Fi/移动网络或稍后重试。`,
      ));
    }, overallTimeoutMs);

    const cleanup = () => {
      clearTimeout(overallTimer);
      if (retryTimer) clearTimeout(retryTimer);
      options.signal?.removeEventListener("abort", handleAbort);
    };

    const finish = (socket: WebSocket) => {
      if (settled) {
        socket.close();
        return;
      }
      settled = true;
      currentSocket = null;
      cleanup();
      resolve(socket);
    };

    const fail = (error: unknown) => {
      if (settled) return;
      settled = true;
      cleanup();
      if (currentSocket) {
        try {
          currentSocket.close();
        } catch {
          // Ignore a socket that is already closed.
        }
        currentSocket = null;
      }
      reject(error instanceof Error ? error : new ConsoleSocketError("无法建立控制台 WebSocket 连接。", { cause: error }));
    };

    const toError = (error: unknown) => error instanceof Error
      ? error
      : new ConsoleSocketError("无法建立控制台 WebSocket 连接。", { cause: error, retryable: true });

    const wait = (delayMs: number) => new Promise<void>((done) => {
      retryTimer = setTimeout(() => {
        retryTimer = undefined;
        done();
      }, delayMs);
    });

    async function run() {
      for (let attempt = 0; attempt < total && !settled; attempt += 1) {
        options.onAttempt?.(attempt + 1, total);
        try {
          const socket = await createSocket(attempt);
          if (settled) {
            socket.close();
            return;
          }
          currentSocket = socket;
          const opened = await waitForConsoleSocket(socket, timeoutMs);
          currentSocket = null;
          if (settled) {
            opened.close();
            return;
          }
          finish(opened);
          return;
        } catch (cause) {
          currentSocket = null;
          if (settled) return;
          const error = toError(cause);
          if (!isRetryableConsoleError(error)) {
            fail(error);
            return;
          }
          if (attempt + 1 >= total) {
            fail(new ConsoleSocketError(`${error.message}（已尝试 ${attempt + 1} 次）`, { cause: error }));
            return;
          }
          const delayMs = consoleSocketRetryDelayMs(attempt, retryDelaysMs, random);
          options.onRetry?.(attempt + 1, total, delayMs, error);
          await wait(delayMs);
        }
      }
    }

    function handleAbort() {
      fail(new ConsoleSocketError("控制台连接已取消。"));
    }

    if (options.signal?.aborted) {
      handleAbort();
      return;
    }
    options.signal?.addEventListener("abort", handleAbort, { once: true });
    void run();
  });
}

function isRetryableConsoleError(error: unknown): boolean {
  if (error instanceof ConsoleSocketError) return error.retryable;
  if (error instanceof ApiError) {
    return error.status === 408 || error.status === 425 || error.status === 429 || error.status >= 500;
  }
  if (error instanceof DOMException && error.name === "AbortError") return false;
  return true;
}

function consoleSocketRetryDelayMs(attempt: number, delays: readonly number[], random: () => number): number {
  const base = delays[attempt] ?? delays[delays.length - 1] ?? 2_000;
  const jitter = base * consoleSocketRetryJitterRatio * (random() * 2 - 1);
  return Math.max(0, Math.round(base + jitter));
}

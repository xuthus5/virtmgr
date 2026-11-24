export const consoleSocketTimeoutMs = 75_000;
export const consoleSocketTimeoutSeconds = consoleSocketTimeoutMs / 1_000;
export const consoleSocketOverallTimeoutMs = 100_000;
export const consoleSocketOverallTimeoutSeconds = consoleSocketOverallTimeoutMs / 1_000;
export const consoleSocketHedgeDelaysMs = [0, 6_000, 15_000] as const;

type ConsoleSocketOptions = {
  timeoutMs?: number;
  createSocket?: (url: string) => WebSocket;
};

type HedgedConsoleSocketOptions = {
  timeoutMs?: number;
  overallTimeoutMs?: number;
  hedgeDelaysMs?: readonly number[];
  signal?: AbortSignal;
  onAttempt?: (attempt: number, total: number) => void;
};

export class ConsoleSocketError extends Error {
  constructor(message: string, options?: ErrorOptions) {
    super(message, options);
    this.name = "ConsoleSocketError";
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
  return waitForConsoleSocket(socket, options.timeoutMs ?? consoleSocketTimeoutMs);
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
      reject(new ConsoleSocketError(message));
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

export function openHedgedConsoleSocket(
  createSocket: (attempt: number) => WebSocket | Promise<WebSocket>,
  options: HedgedConsoleSocketOptions = {},
): Promise<WebSocket> {
  const hedgeDelaysMs = options.hedgeDelaysMs ?? consoleSocketHedgeDelaysMs;
  const timeoutMs = options.timeoutMs ?? consoleSocketTimeoutMs;
  const overallTimeoutMs = options.overallTimeoutMs ?? consoleSocketOverallTimeoutMs;
  const total = hedgeDelaysMs.length;

  return new Promise((resolve, reject) => {
    let settled = false;
    let failedAttempts = 0;
    let lastError: unknown;
    const pending = new Set<WebSocket>();
    const startTimers = new Set<ReturnType<typeof setTimeout>>();
    const overallTimer = setTimeout(() => {
      fail(new ConsoleSocketError(
        `控制台 WebSocket 连接超时：${overallTimeoutMs / 1_000} 秒内尝试了 ${total} 条连接，仍未完成握手。当前到公网入口的链路不稳定，请切换 Wi-Fi/移动网络或稍后重试。`,
        { cause: lastError },
      ));
    }, overallTimeoutMs);

    const cleanup = () => {
      clearTimeout(overallTimer);
      for (const timer of startTimers) clearTimeout(timer);
      startTimers.clear();
      options.signal?.removeEventListener("abort", handleAbort);
    };

    const closePending = () => {
      for (const socket of pending) {
        try {
          socket.close();
        } catch {
          // Ignore sockets that are already closed or never opened.
        }
      }
      pending.clear();
    };

    const finish = (socket: WebSocket) => {
      if (settled) {
        try {
          socket.close();
        } catch {
          // The winning socket may have closed immediately after resolving.
        }
        return;
      }
      settled = true;
      cleanup();
      pending.delete(socket);
      closePending();
      resolve(socket);
    };

    const fail = (error: unknown) => {
      if (settled) return;
      settled = true;
      cleanup();
      closePending();
      reject(error instanceof Error ? error : new ConsoleSocketError("无法建立控制台 WebSocket 连接。", { cause: error }));
    };

    const noteFailure = (error: unknown) => {
      failedAttempts += 1;
      lastError = error;
      if (failedAttempts >= total) {
        fail(error);
      }
    };

    const start = (attempt: number) => {
      if (settled) return;
      options.onAttempt?.(attempt + 1, total);
      Promise.resolve()
        .then(() => createSocket(attempt))
        .then((socket) => {
          if (settled) {
            socket.close();
            return;
          }
          pending.add(socket);
          void waitForConsoleSocket(socket, timeoutMs).then(
            (opened) => {
              pending.delete(opened);
              finish(opened);
            },
            (error) => {
              pending.delete(socket);
              noteFailure(error);
            },
          );
        })
        .catch(noteFailure);
    };

    function handleAbort() {
      fail(new ConsoleSocketError("控制台连接已取消。"));
    }

    if (options.signal?.aborted) {
      handleAbort();
      return;
    }
    options.signal?.addEventListener("abort", handleAbort, { once: true });

    hedgeDelaysMs.forEach((delay, attempt) => {
      if (delay <= 0) {
        start(attempt);
        return;
      }
      const timer = setTimeout(() => {
        startTimers.delete(timer);
        start(attempt);
      }, delay);
      startTimers.add(timer);
    });
  });
}

export const consoleSocketTimeoutMs = 15_000;

type ConsoleSocketOptions = {
  timeoutMs?: number;
  createSocket?: (url: string) => WebSocket;
};

export class ConsoleSocketError extends Error {
  constructor(message: string, options?: ErrorOptions) {
    super(message, options);
    this.name = "ConsoleSocketError";
  }
}

export function openConsoleSocket(url: string, options: ConsoleSocketOptions = {}): Promise<WebSocket> {
  const timeoutMs = options.timeoutMs ?? consoleSocketTimeoutMs;
  const createSocket = options.createSocket ?? ((target: string) => new WebSocket(target));
  let socket: WebSocket;
  try {
    socket = createSocket(url);
  } catch (cause) {
    return Promise.reject(new ConsoleSocketError("无法创建控制台 WebSocket 连接。", { cause }));
  }
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
      fail("控制台 WebSocket 建连失败。若当前地址是 IP 且使用自签名证书，浏览器可能只允许页面访问却拦截 wss；请改用受信任的 HTTPS 域名，并确认网络或反向代理允许 WebSocket。");
    };
    const handleClose = (event: CloseEvent) => {
      const detail = event.code ? `（code ${event.code}${event.reason ? `，${event.reason}` : ""}）` : "";
      fail(`控制台 WebSocket 在握手前关闭${detail}，请刷新后重试。`);
    };

    socket.addEventListener("open", handleOpen);
    socket.addEventListener("error", handleError);
    socket.addEventListener("close", handleClose);
    timer = setTimeout(() => {
      fail("控制台 WebSocket 连接超时。若当前地址是 IP 且使用自签名证书，浏览器可能只允许页面访问却拦截 wss；请改用受信任的 HTTPS 域名，刷新页面后重试。");
    }, timeoutMs);

    if (socket.readyState === WebSocket.OPEN) {
      handleOpen();
    } else if (socket.readyState !== WebSocket.CONNECTING) {
      fail("控制台 WebSocket 已关闭，无法建立连接。请刷新页面后重试。");
    }
  });
}

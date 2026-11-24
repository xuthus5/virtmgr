import { useEffect, useId, useMemo, useRef, useState } from "react";
import { AlertTriangle, ClipboardPaste, Keyboard, LoaderCircle, Send, Square, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import {
  analyzeConsoleTextInput,
  CONSOLE_INPUT_MAX_LENGTH,
  describeConsoleInputCharacter,
} from "@/features/console/console-input";
import { trapTabFocus } from "@/lib/focus-trap";
import { cn } from "@/lib/utils";

type InputNotice = { tone: "success" | "error"; text: string };

type VNCInputPadProps = {
  connected: boolean;
  onClose: () => void;
  onType: (text: string, options: { signal: AbortSignal; onProgress: (sent: number, total: number) => void }) => Promise<number>;
  onSendClipboard: (text: string) => boolean;
};

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === "AbortError";
}

export function VNCInputPad({ connected, onClose, onType, onSendClipboard }: VNCInputPadProps) {
  const titleId = useId();
  const descriptionId = useId();
  const panelRef = useRef<HTMLElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const abortRef = useRef<AbortController | null>(null);
  const mountedRef = useRef(true);
  const onCloseRef = useRef(onClose);
  const [text, setText] = useState("");
  const [appendEnter, setAppendEnter] = useState(false);
  const [sending, setSending] = useState(false);
  const [progress, setProgress] = useState<{ sent: number; total: number } | null>(null);
  const [notice, setNotice] = useState<InputNotice | null>(null);

  onCloseRef.current = onClose;

  const payload = appendEnter && text ? `${text}\n` : text;
  const analysis = useMemo(() => analyzeConsoleTextInput(payload), [payload]);
  const unsupportedPreview = analysis.unsupported.slice(0, 8).map(describeConsoleInputCharacter).join("、");
  const unsupportedSuffix = analysis.unsupported.length > 8 ? " 等" : "";
  const canType = connected && !sending && text.length > 0 && analysis.unsupported.length === 0;
  const canSendClipboard = connected && !sending && text.length > 0;
  const progressPercent = progress && progress.total > 0 ? Math.round((progress.sent / progress.total) * 100) : 0;

  useEffect(() => {
    mountedRef.current = true;
    const previouslyFocused = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    textareaRef.current?.focus();
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.preventDefault();
        onCloseRef.current();
        return;
      }
      trapTabFocus(event, panelRef.current);
    }
    document.addEventListener("keydown", onKeyDown);
    return () => {
      mountedRef.current = false;
      abortRef.current?.abort();
      document.removeEventListener("keydown", onKeyDown);
      if (previouslyFocused?.isConnected) previouslyFocused.focus();
    };
  }, []);

  function updateText(value: string) {
    setText(value);
    setNotice(null);
  }

  async function handleType() {
    if (!canType) return;
    const controller = new AbortController();
    abortRef.current = controller;
    setNotice(null);
    setSending(true);
    setProgress({ sent: 0, total: analysis.keystrokes.length });
    try {
      const count = await onType(payload, {
        signal: controller.signal,
        onProgress: (sent, total) => {
          if (mountedRef.current) setProgress({ sent, total });
        },
      });
      if (mountedRef.current) setNotice({ tone: "success", text: `已键入 ${count} 个字符。` });
    } catch (error) {
      if (!mountedRef.current) return;
      setNotice(isAbortError(error)
        ? { tone: "error", text: "已停止键入。" }
        : { tone: "error", text: error instanceof Error ? error.message : "键入失败。" });
    } finally {
      if (abortRef.current === controller) abortRef.current = null;
      if (mountedRef.current) {
        setSending(false);
        setProgress(null);
      }
    }
  }

  function handleSendClipboard() {
    if (!canSendClipboard) return;
    const sent = onSendClipboard(text);
    setNotice(sent
      ? { tone: "success", text: "已写入远端剪贴板。请在客户机内按 Ctrl+V；该能力取决于客户机剪贴板服务。" }
      : { tone: "error", text: "VNC 控制台尚未连接，无法发送剪贴板。" });
  }

  return (
    <div
      className="absolute inset-0 z-20 flex justify-end bg-slate-950/55"
      role="presentation"
      onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}
    >
      <section
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={descriptionId}
        data-testid="vnc-input-pad"
        className="flex h-full w-full max-w-md flex-col border-l border-slate-700 bg-slate-900 text-slate-100 shadow-2xl"
      >
        <header className="flex items-start gap-3 border-b border-slate-800 p-4">
          <div className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-primary text-primary-foreground shadow-sm">
            <Keyboard className="h-4 w-4" />
          </div>
          <div className="min-w-0 flex-1">
            <h2 id={titleId} className="text-sm font-semibold">VNC 输入板</h2>
            <p id={descriptionId} className="mt-1 text-xs leading-5 text-slate-400">
              将命令粘贴到下方文本框，再逐字键入客户机。键入模式不依赖客户机剪贴板服务。
            </p>
          </div>
          <Button type="button" variant="ghost" size="icon-sm" className="text-slate-300 hover:bg-slate-800 hover:text-white" aria-label="关闭输入板" onClick={onClose}>
            <X className="h-4 w-4" />
          </Button>
        </header>

        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto p-4">
          {!connected ? (
            <div className="rounded-md border border-rose-500/30 bg-rose-500/10 px-3 py-2 text-xs text-rose-200" role="alert">
              VNC 控制台未连接，连接后才能发送内容。
            </div>
          ) : null}

          {analysis.unsupported.length > 0 ? (
            <div className="flex gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs leading-5 text-amber-200" role="alert" data-testid="vnc-input-unsupported">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <p>逐字键入仅支持 ASCII。发现 {analysis.unsupported.length} 种不支持字符：<span className="font-mono">{unsupportedPreview}{unsupportedSuffix}</span>。可改用“放入远端剪贴板”。</p>
            </div>
          ) : null}

          <div className="space-y-1.5">
            <div className="flex items-center justify-between gap-3">
              <label htmlFor="vnc-input-text" className="text-xs font-medium text-slate-300">待输入内容</label>
              <button type="button" className="text-[11px] text-slate-500 transition-colors hover:text-slate-300" onClick={() => updateText("")} disabled={!text || sending}>清空</button>
            </div>
            <textarea
              id="vnc-input-text"
              ref={textareaRef}
              value={text}
              maxLength={CONSOLE_INPUT_MAX_LENGTH}
              spellCheck={false}
              autoCapitalize="off"
              autoCorrect="off"
              data-testid="vnc-input-textarea"
              className="min-h-40 w-full resize-y rounded-md border border-slate-700 bg-slate-950 px-3 py-2 font-mono text-xs leading-5 text-slate-100 outline-none transition-colors placeholder:text-slate-600 focus:border-sky-500 focus:ring-2 focus:ring-sky-500/20 disabled:cursor-not-allowed disabled:opacity-60"
              placeholder="直接 Ctrl+V 粘贴命令或脚本…"
              onChange={(event) => updateText(event.target.value)}
              onKeyDown={(event) => {
                if ((event.ctrlKey || event.metaKey) && event.key === "Enter") {
                  event.preventDefault();
                  void handleType();
                }
              }}
              disabled={sending}
            />
            <div className="flex items-center justify-between gap-3 text-[11px] text-slate-500">
              <span>{text.length} / {CONSOLE_INPUT_MAX_LENGTH} 字符 · Ctrl+Enter 键入</span>
              <span>{analysis.keystrokes.length} 次按键</span>
            </div>
          </div>

          <label className="flex cursor-pointer items-center gap-2 text-xs text-slate-300">
            <input type="checkbox" className="h-3.5 w-3.5 accent-sky-500" checked={appendEnter} onChange={(event) => { setAppendEnter(event.target.checked); setNotice(null); }} disabled={sending} />
            键入完成后追加回车
          </label>

          <p className="text-[11px] leading-5 text-slate-500">
            逐字键入按 US/QWERTY 键位模拟键盘，适合安装器、TTY、GRUB 和无 Guest Agent 的客户机。中文、Emoji 或客户机使用其他键盘布局时，请优先使用远端剪贴板并在客户机内粘贴。
          </p>

        </div>

        {progress || notice ? (
          <div className="space-y-2 border-t border-slate-800 px-4 py-2.5">
            {progress ? (
              <div className="space-y-1.5" data-testid="vnc-input-progress">
                <div className="flex items-center justify-between text-[11px] text-slate-400"><span>正在键入</span><span>{progress.sent} / {progress.total}</span></div>
                <Progress value={progressPercent} className="bg-slate-800" />
              </div>
            ) : null}
            {notice ? (
              <div
                role="status"
                data-testid="vnc-input-notice"
                className={cn("rounded-md border px-3 py-2 text-xs leading-5", notice.tone === "success" ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-200" : "border-rose-500/30 bg-rose-500/10 text-rose-200")}
              >
                {notice.text}
              </div>
            ) : null}
          </div>
        ) : null}

        <footer className="flex flex-wrap items-center justify-end gap-2 border-t border-slate-800 bg-slate-900/95 p-3">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="border-slate-700 bg-slate-800 text-slate-100 hover:bg-slate-700 hover:text-white"
            data-testid="vnc-input-send-clipboard"
            disabled={!canSendClipboard}
            onClick={handleSendClipboard}
          >
            <ClipboardPaste className="h-3.5 w-3.5" />放入远端剪贴板
          </Button>
          {sending ? (
            <Button type="button" variant="destructive" size="sm" data-testid="vnc-input-stop" onClick={() => abortRef.current?.abort()}>
              <Square className="h-3.5 w-3.5" />停止
            </Button>
          ) : (
            <Button type="button" size="sm" data-testid="vnc-input-type" disabled={!canType} onClick={() => void handleType()}>
              {connected ? <Send className="h-3.5 w-3.5" /> : <LoaderCircle className="h-3.5 w-3.5" />}逐字键入
            </Button>
          )}
        </footer>
      </section>
    </div>
  );
}

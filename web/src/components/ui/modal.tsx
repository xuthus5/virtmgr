import { useEffect, useId, useRef, type ReactNode } from "react";
import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export function Modal({ open, onClose, title, description, children, footer, className, testId }: {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string;
  children: ReactNode;
  footer?: ReactNode;
  className?: string;
  testId?: string;
}) {
  const titleID = useId();
  const descriptionID = useId();
  const closeRef = useRef<HTMLButtonElement>(null);
  const onCloseRef = useRef(onClose);

  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  useEffect(() => {
    if (!open) return;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    closeRef.current?.focus();
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.preventDefault();
        onCloseRef.current();
      }
    }
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  if (!open) return null;
  return (
    <div className="fixed inset-0 z-[70] flex items-end justify-center bg-slate-950/45 p-0 sm:items-center sm:p-6" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleID}
        aria-describedby={description ? descriptionID : undefined}
        data-testid={testId}
        className={cn("flex max-h-[92vh] w-full max-w-2xl flex-col overflow-hidden rounded-t-xl border bg-card shadow-2xl sm:rounded-xl", className)}
      >
        <header className="flex items-start gap-4 border-b p-5">
          <div className="min-w-0 flex-1">
            <h2 id={titleID} className="text-base font-semibold">{title}</h2>
            {description ? <p id={descriptionID} className="mt-1 text-sm text-muted-foreground">{description}</p> : null}
          </div>
          <Button ref={closeRef} type="button" variant="ghost" size="icon-sm" aria-label="关闭对话框" onClick={onClose}><X className="h-4 w-4" /></Button>
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto p-5">{children}</div>
        {footer ? <footer className="flex flex-wrap items-center justify-end gap-2 border-t bg-muted/20 p-4">{footer}</footer> : null}
      </section>
    </div>
  );
}

import { useEffect, type ReactNode } from "react";
import { cn } from "@/lib/utils";

export function PageHeader({ title, documentTitle, description, breadcrumb, actions, className }: {
  title: ReactNode;
  documentTitle?: string;
  description?: ReactNode;
  breadcrumb?: ReactNode;
  actions?: ReactNode;
  className?: string;
}) {
  const resolvedTitle = typeof title === "string" ? title : documentTitle;

  useEffect(() => {
    document.title = resolvedTitle ? `${resolvedTitle} · VirtMgr` : "VirtMgr";
  }, [resolvedTitle]);

  return (
    <div className={cn("mb-5 flex flex-col gap-3", className)}>
      {breadcrumb ? <div className="text-xs text-muted-foreground">{breadcrumb}</div> : null}
      <div className="flex flex-col justify-between gap-3 md:flex-row md:items-start">
        <div className="min-w-0">
          <h1 className="truncate text-xl font-semibold tracking-tight text-foreground">{title}</h1>
          {description ? <div className="mt-1 text-sm text-muted-foreground">{description}</div> : null}
        </div>
        {actions ? <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div> : null}
      </div>
    </div>
  );
}

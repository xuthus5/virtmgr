import { AlertTriangle, LoaderCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export function LoadingState({ label = "正在加载", className }: { label?: string; className?: string }) {
  return (
    <div className={cn("grid place-items-center py-16 text-sm text-muted-foreground", className)}>
      <LoaderCircle className="mb-3 h-5 w-5 animate-spin text-primary" />
      {label}
    </div>
  );
}

export function ErrorState({ message, onRetry, className }: { message: string; onRetry?: () => void; className?: string }) {
  return (
    <div className={cn("rounded-lg border border-destructive/30 bg-destructive/5 p-6 text-center", className)}>
      <AlertTriangle className="mx-auto h-5 w-5 text-destructive" />
      <p className="mt-3 text-sm font-medium">加载失败</p>
      <p className="mt-1 text-xs text-muted-foreground">{message}</p>
      {onRetry ? <Button variant="outline" size="sm" className="mt-4" onClick={onRetry}>重试</Button> : null}
    </div>
  );
}

import { Progress } from "@/components/ui/progress";
import { cn, percent } from "@/lib/utils";

export function ResourceBar({ label, used, total, displayUsed, displayTotal, className }: {
  label: string;
  used: number;
  total: number;
  displayUsed: string;
  displayTotal: string;
  className?: string;
}) {
  const value = percent(used, total);
  return (
    <div className={cn("space-y-1.5", className)}>
      <div className="flex items-center justify-between text-xs">
        <span className="text-muted-foreground">{label}</span>
        <span className="font-medium tabular-nums">{displayUsed} / {displayTotal}</span>
      </div>
      <Progress value={value} className="h-1.5" aria-label={`${label}使用率`} />
    </div>
  );
}

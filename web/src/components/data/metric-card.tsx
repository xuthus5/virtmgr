import type { LucideIcon } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { cn } from "@/lib/utils";

export function MetricCard({ label, value, hint, icon: Icon, tone = "primary", testId, className }: {
  label: string;
  value: string | number;
  hint?: string;
  icon: LucideIcon;
  tone?: "primary" | "success" | "warning" | "danger" | "neutral";
  testId?: string;
  className?: string;
}) {
  return (
    <Card className={cn("overflow-hidden", className)}>
      <CardContent className="flex items-start justify-between p-5">
        <div className="min-w-0">
          <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{label}</p>
          <p className="mt-2 text-2xl font-semibold tabular-nums tracking-tight" data-testid={testId}>{value}</p>
          {hint ? <p className="mt-1 truncate text-xs text-muted-foreground">{hint}</p> : null}
        </div>
        <div className={cn("rounded-lg p-2.5", {
          "bg-primary/10 text-primary": tone === "primary",
          "bg-success/10 text-success": tone === "success",
          "bg-warning/10 text-warning": tone === "warning",
          "bg-destructive/10 text-destructive": tone === "danger",
          "bg-muted text-muted-foreground": tone === "neutral",
        })}>
          <Icon className="h-5 w-5" />
        </div>
      </CardContent>
    </Card>
  );
}

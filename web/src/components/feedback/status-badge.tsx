import { Badge } from "@/components/ui/badge";
import type { VMState } from "@/api/types";
import { cn } from "@/lib/utils";

const labels: Record<VMState, string> = {
  running: "运行中",
  blocked: "阻塞",
  paused: "已暂停",
  "shutting-down": "关机中",
  crashed: "异常",
  suspended: "已挂起",
  shutoff: "已关机",
  unknown: "未知",
};

function variant(state: VMState): "success" | "warning" | "destructive" | "secondary" | "default" {
  switch (state) {
    case "running":
      return "success";
    case "paused":
    case "suspended":
    case "shutting-down":
      return "warning";
    case "crashed":
      return "destructive";
    case "shutoff":
      return "secondary";
    default:
      return "default";
  }
}

export function StatusBadge({ state, className }: { state: VMState; className?: string }) {
  const tone = variant(state);
  return (
    <Badge variant={tone} className={cn("whitespace-nowrap", className)}>
      <span className={cn("h-1.5 w-1.5 rounded-full", {
        "bg-success": tone === "success",
        "bg-warning": tone === "warning",
        "bg-destructive": tone === "destructive",
        "bg-muted-foreground": tone === "secondary",
        "bg-primary": tone === "default",
      })} />
      {labels[state]}
    </Badge>
  );
}

export function stateLabel(state: VMState): string {
  return labels[state];
}

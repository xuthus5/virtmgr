import type { Graphics, SerialConsole, VMState } from "@/api/types";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { VMConsolePanel } from "@/features/console/vm-console-panel";

export function VMConsoleCard({
  uuid,
  graphics,
  serials,
  state,
  canConnect,
  disabledReason,
}: {
  uuid: string;
  graphics: Graphics[];
  serials?: SerialConsole[];
  state: VMState;
  canConnect: boolean;
  disabledReason: string;
}) {
  return (
    <Card className="mt-4 overflow-hidden" data-testid="vm-console-card">
      <CardHeader>
        <CardTitle>控制台</CardTitle>
        <CardDescription>直接在页面内访问 VNC 图形控制台或 PTY 串口控制台；支持发送组合键、断开、缩放、展开和独立窗口。</CardDescription>
      </CardHeader>
      <CardContent className="p-0">
        <VMConsolePanel
          uuid={uuid}
          graphics={graphics}
          serials={serials}
          vmState={state}
          canConnect={canConnect}
          disabledReason={disabledReason}
          autoConnect
        />
      </CardContent>
    </Card>
  );
}

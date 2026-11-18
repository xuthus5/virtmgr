import type { ComponentProps } from "react";
import { Download, LoaderCircle, Trash2 } from "lucide-react";
import type { VM } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { VMCloneCard } from "@/features/vms/vm-clone-card";
import { VMXMLCard } from "@/features/vms/vm-xml-card";
import { cn } from "@/lib/utils";

export function VMDetailAdvancedTab({ vm, canMutate, exportNotice, exportPending, onExport, onDelete, cloneProps, xmlProps }: {
  vm: VM;
  canMutate: boolean;
  exportNotice: { tone: "success" | "error"; text: string } | null;
  exportPending: boolean;
  onExport: () => void;
  onDelete: () => void;
  cloneProps: ComponentProps<typeof VMCloneCard>;
  xmlProps: ComponentProps<typeof VMXMLCard>;
}) {
  return <div role="tabpanel" id="vm-tabpanel-advanced" aria-labelledby="vm-tab-advanced" data-testid="vm-tabpanel-advanced">
    <div className="mt-4 flex flex-wrap items-start justify-between gap-3 rounded-lg border bg-card px-4 py-3">
      <div>
        <p className="text-sm font-medium">导出虚拟机配置</p>
        <p className="mt-0.5 text-xs text-muted-foreground">下载域定义、XML 与快照元数据；不包含磁盘内容。</p>
      </div>
      <Button variant="outline" size="sm" data-testid="vm-export-button" disabled={exportPending} title="下载包含虚拟机定义、XML 与快照元数据的 JSON 文件" onClick={onExport}>
        {exportPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}导出配置
      </Button>
    </div>
    {exportNotice ? (
      <p data-testid="vm-export-notice" role="status" className={cn("mt-4 rounded-md border px-3 py-2 text-xs", exportNotice.tone === "success" ? "border-success/30 bg-success/10 text-success" : "border-destructive/30 bg-destructive/5 text-destructive")}>{exportNotice.text}</p>
    ) : null}

    <VMCloneCard {...cloneProps} />
    <VMXMLCard {...xmlProps} />

    {canMutate ? (
      <Card className="mt-4 border-destructive/30">
        <CardHeader>
          <CardTitle>危险操作</CardTitle>
          <CardDescription>删除只移除 libvirt 域定义，磁盘文件会保留。此操作不可撤销。</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 sm:flex-row sm:items-center">
          <Button variant="destructive" data-testid="vm-delete-button" disabled={vm.state !== "shutoff" && vm.state !== "crashed"} title={vm.state !== "shutoff" && vm.state !== "crashed" ? "请先关闭虚拟机" : "删除虚拟机域定义"} onClick={onDelete}>
            <Trash2 className="h-4 w-4" />删除虚拟机
          </Button>
          <p className="text-xs text-muted-foreground">{vm.state === "shutoff" || vm.state === "crashed" ? "删除前需要输入虚拟机名称确认。" : "运行中的虚拟机不能删除，请先执行正常关机或强制下电。"}</p>
        </CardContent>
      </Card>
    ) : null}
  </div>;
}

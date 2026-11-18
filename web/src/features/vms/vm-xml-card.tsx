import { lazy, Suspense } from "react";
import { Check, Clipboard, History, LoaderCircle, Pencil, Save, X } from "lucide-react";
import type { Role, VMConfigVersion, VMXML, VMXMLFormat } from "@/api/types";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { VMConfigVersionsDialog } from "@/features/vms/vm-config-versions-dialog";

const XMLCodeEditor = lazy(() => import("@/components/code/xml-code-editor").then((module) => ({ default: module.XMLCodeEditor })));

export function VMXMLCard({ vmName, uuid, role, readOnly, format, xml, loading, error, fetching, editing, draft, copyNotice, editNotice, editError, canEdit, saving, versionsOpen, versionsTotal, currentXml, onFormatChange, onCopy, onBeginEdit, onCancel, onDraftChange, onRequestSave, onRetry, onVersionsOpenChange, onRolledBack }: {
  vmName: string;
  uuid: string;
  role: Role | undefined;
  readOnly: boolean;
  format: VMXMLFormat;
  xml?: VMXML;
  loading: boolean;
  error?: string;
  fetching: boolean;
  editing: boolean;
  draft: string;
  copyNotice: "copied" | "error" | "";
  editNotice: string;
  editError: string;
  canEdit: boolean;
  saving: boolean;
  versionsOpen: boolean;
  versionsTotal?: number;
  currentXml?: string;
  onFormatChange: (format: VMXMLFormat) => void;
  onCopy: () => void;
  onBeginEdit: () => void;
  onCancel: () => void;
  onDraftChange: (value: string) => void;
  onRequestSave: () => void;
  onRetry: () => void;
  onVersionsOpenChange: (open: boolean) => void;
  onRolledBack: (version: VMConfigVersion) => void;
}) {
  return <>
    <Card className="mt-4">
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle>域 XML</CardTitle>
            <CardDescription>{editing ? "编辑持久配置 XML。名称和 UUID 必须保持不变；仅更新定义，不启动虚拟机。" : "查看 libvirt 域定义。运行态包含当前运行时覆盖，持久配置为 inactive XML。"}</CardDescription>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" size="sm" data-testid="vm-config-versions-open" title="查看持久域 XML 的自动归档版本" onClick={() => onVersionsOpenChange(true)}>
              <History className="h-4 w-4" />版本历史{versionsTotal !== undefined ? ` · ${versionsTotal}` : ""}
            </Button>
            <select aria-label="XML 类型" data-testid="vm-xml-format" value={format} disabled={editing} onChange={(event) => onFormatChange(event.target.value as VMXMLFormat)} className="h-8 rounded-md border border-input bg-card px-3 text-xs shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50">
              <option value="live">运行态 XML</option>
              <option value="config">持久配置 XML</option>
            </select>
            <Button variant="outline" size="sm" data-testid="vm-xml-copy" disabled={!xml?.xml || fetching} onClick={onCopy}>
              {copyNotice === "copied" ? <Check className="h-4 w-4" /> : <Clipboard className="h-4 w-4" />}复制 XML
            </Button>
            {canEdit && !editing ? (
              <Button variant="outline" size="sm" data-testid="vm-xml-edit" disabled={!xml?.xml || fetching} onClick={onBeginEdit}>
                <Pencil className="h-4 w-4" />编辑 XML
              </Button>
            ) : null}
            {editing ? (
              <>
                <Button variant="ghost" size="sm" data-testid="vm-xml-cancel" disabled={saving} onClick={onCancel}><X className="h-4 w-4" />取消</Button>
                <Button size="sm" data-testid="vm-xml-save" disabled={saving} onClick={onRequestSave}>
                  {saving ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}保存定义
                </Button>
              </>
            ) : null}
          </div>
        </div>
      </CardHeader>
      <CardContent className="p-0">
        <div className="flex flex-wrap items-center gap-3 border-b px-4 py-2 text-xs text-muted-foreground">
          <Badge variant="outline" data-testid="vm-xml-format-badge">{xml?.format === "config" ? "持久配置" : "运行态"}</Badge>
          <span>{editing ? "编辑模式" : "只读视图"}</span>
          {xml ? <span>{xml.xml.length.toLocaleString()} 字符</span> : null}
          {copyNotice === "copied" ? <span className="text-success" data-testid="vm-xml-copy-notice">已复制到剪贴板</span> : null}
          {copyNotice === "error" ? <span className="text-destructive" data-testid="vm-xml-copy-notice">复制失败，请手动选择 XML</span> : null}
          {editNotice ? <span className="text-success" data-testid="vm-xml-edit-notice">{editNotice}</span> : null}
          {editError ? <span className="text-destructive" data-testid="vm-xml-edit-error" role="alert">{editError}</span> : null}
        </div>
        {loading ? <LoadingState label="正在读取域 XML" /> : null}
        {error ? <ErrorState message={error} onRetry={onRetry} className="m-4" /> : null}
        {xml && editing ? (
          <div className="h-[32rem] overflow-hidden">
            <Suspense fallback={<LoadingState label="正在加载 XML 编辑器" />}>
              <XMLCodeEditor value={draft} onChange={onDraftChange} ariaLabel="域 XML 编辑器" testId="vm-xml-editor" className="h-full" />
            </Suspense>
          </div>
        ) : xml ? (
          <div className="h-[32rem] overflow-hidden">
            <Suspense fallback={<LoadingState label="正在加载 XML 编辑器" />}>
              <XMLCodeEditor value={xml.xml} readOnly ariaLabel="域 XML 只读内容" testId="vm-xml-content" format={xml.format} className="h-full" />
            </Suspense>
          </div>
        ) : null}
      </CardContent>
    </Card>

    <VMConfigVersionsDialog uuid={uuid} vmName={vmName} open={versionsOpen} onOpenChange={onVersionsOpenChange} currentXml={currentXml} role={role} readOnly={readOnly} onRolledBack={onRolledBack} />
  </>;
}

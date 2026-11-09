import { lazy, Suspense, useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  Check,
  Clipboard,
  Download,
  History,
  LoaderCircle,
  Pin,
  PinOff,
  RotateCcw,
} from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { Role, VMConfigVersion, VMConfigVersionDetail } from "@/api/types";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Modal } from "@/components/ui/modal";
import { overviewQueryKey } from "@/features/overview/use-overview-query";
import { vmXMLQueryKey } from "@/features/vms/use-vm-query";
import {
  vmConfigVersionQueryKey,
  vmConfigVersionsQueryKey,
  useVMConfigVersionQuery,
  useVMConfigVersionsQuery,
} from "@/features/vms/use-vm-config-versions-query";
import {
  configVersionFilename,
  configVersionRollbackReason,
  configVersionSourceLabel,
  shortConfigVersionSHA,
} from "@/features/vms/vm-config-version-model";
import { formatBytes } from "@/lib/utils";

const XMLCodeEditor = lazy(() =>
  import("@/components/code/xml-code-editor").then((module) => ({ default: module.XMLCodeEditor })),
);

const RETAINED_CONFIG_VERSIONS = 100;

const timeFormatter = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});

type Props = {
  uuid: string;
  vmName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  currentXml?: string;
  role?: Role;
  readOnly: boolean;
  onRolledBack?: (version: VMConfigVersionDetail) => void;
};

type Notice = { tone: "success" | "error"; text: string } | null;

export function VMConfigVersionsDialog({ uuid, vmName, open, onOpenChange, currentXml, role, readOnly, onRolledBack }: Props) {
  const queryClient = useQueryClient();
  const versionsQuery = useVMConfigVersionsQuery(uuid);
  const [selectedVersion, setSelectedVersion] = useState<number | null>(null);
  const [notice, setNotice] = useState<Notice>(null);
  const [rollbackOpen, setRollbackOpen] = useState(false);
  const [rollbackNote, setRollbackNote] = useState("");
  const [copyNotice, setCopyNotice] = useState<"copied" | "error" | "">("");
  const detailQuery = useVMConfigVersionQuery(uuid, selectedVersion, open);

  const versions = versionsQuery.data?.items ?? [];
  const selected = versions.find((item) => item.versionNo === selectedVersion);

  useEffect(() => {
    if (!open) {
      setNotice(null);
      setRollbackOpen(false);
      setRollbackNote("");
      setCopyNotice("");
      return;
    }
    void versionsQuery.refetch();
  }, [open, versionsQuery.refetch]);

  useEffect(() => {
    if (!open) return;
    if (!versionsQuery.data) return;
    setSelectedVersion((current) => {
      if (current !== null && versionsQuery.data.items.some((item) => item.versionNo === current)) return current;
      return versionsQuery.data.items[0]?.versionNo ?? null;
    });
  }, [open, versionsQuery.data]);

  const pinMutation = useMutation({
    mutationFn: (version: VMConfigVersion) =>
      api.protectVMConfigVersion(uuid, version.versionNo, { protected: !version.protected }),
    onSuccess: (updated) => {
      setNotice({ tone: "success", text: `版本 v${updated.versionNo} 已${updated.protected ? "固定" : "取消固定"}。` });
      void queryClient.invalidateQueries({ queryKey: vmConfigVersionsQueryKey(uuid) });
      void queryClient.invalidateQueries({ queryKey: vmConfigVersionQueryKey(uuid, updated.versionNo) });
    },
    onError: (error) => {
      setNotice({ tone: "error", text: error instanceof ApiError ? error.message : "更新版本保护状态失败。" });
    },
  });

  const rollbackMutation = useMutation({
    mutationFn: (version: VMConfigVersion) =>
      api.rollbackVMConfigVersion(uuid, version.versionNo, {
        xmlSha256: versionsQuery.data?.currentSha256 ?? "",
        note: rollbackNote,
      }),
    onSuccess: async (result) => {
      setRollbackOpen(false);
      setRollbackNote("");
      setNotice({ tone: "success", text: `已回滚到 v${result.sourceVersion ?? "?"}，并自动创建版本 v${result.versionNo}。` });
      queryClient.setQueryData(vmXMLQueryKey(uuid, "config"), { uuid, name: vmName, format: "config", xml: result.xml });
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: vmConfigVersionsQueryKey(uuid) }),
        queryClient.invalidateQueries({ queryKey: ["vm", uuid] }),
        queryClient.invalidateQueries({ queryKey: ["vms"] }),
        queryClient.invalidateQueries({ queryKey: overviewQueryKey }),
      ]);
      setSelectedVersion(result.versionNo);
      onRolledBack?.(result);
    },
    onError: (error) => {
      setRollbackOpen(false);
      if (error instanceof ApiError && error.status === 409) {
        setNotice({ tone: "error", text: "当前持久配置已被其他操作修改。请关闭历史窗口并刷新后重新选择版本。" });
        void versionsQuery.refetch();
        return;
      }
      setNotice({ tone: "error", text: error instanceof ApiError ? error.message : "回滚域 XML 失败。" });
    },
  });

  async function copySelectedXML() {
    if (!detailQuery.data?.xml) return;
    try {
      await navigator.clipboard.writeText(detailQuery.data.xml);
      setCopyNotice("copied");
    } catch {
      setCopyNotice("error");
    }
  }

  function downloadSelectedXML() {
    if (!detailQuery.data?.xml || !selected) return;
    const url = URL.createObjectURL(new Blob([detailQuery.data.xml], { type: "application/xml" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = configVersionFilename(vmName, selected.versionNo);
    link.click();
    URL.revokeObjectURL(url);
  }

  const rollbackReason = selected
    ? configVersionRollbackReason({ role, readOnly, isCurrent: selected.isCurrent })
    : "请选择一个版本";

  return (
    <>
      <Modal
        open={open}
        onClose={() => onOpenChange(false)}
        title="域 XML 版本历史"
        description="仅归档持久域定义，不包含磁盘、内存或快照数据。回滚后需重启运行中的虚拟机才会生效。"
        testId="vm-config-versions-dialog"
        className="max-w-6xl"
        footer={<Button variant="outline" onClick={() => onOpenChange(false)}>关闭</Button>}
      >
        <div className="space-y-4">
          <div className="rounded-lg border border-warning/35 bg-warning/10 px-4 py-3 text-xs text-warning">
            <div className="flex items-start gap-2">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <p>版本回滚只恢复 libvirt 持久配置 XML，不会恢复磁盘内容、网络、直通设备或快照。系统会保留每台虚拟机最近 {RETAINED_CONFIG_VERSIONS} 个未固定版本。</p>
            </div>
          </div>

          {notice ? (
            <p
              role="status"
              data-testid="vm-config-version-notice"
              className={`rounded-md border px-3 py-2 text-xs ${notice.tone === "success" ? "border-success/30 bg-success/10 text-success" : "border-destructive/30 bg-destructive/5 text-destructive"}`}
            >
              {notice.text}
            </p>
          ) : null}

          {versionsQuery.isLoading ? <LoadingState label="正在读取配置版本" /> : null}
          {versionsQuery.isError ? (
            <ErrorState
              message={versionsQuery.error instanceof Error ? versionsQuery.error.message : "无法读取配置版本"}
              onRetry={() => void versionsQuery.refetch()}
            />
          ) : null}
          {!versionsQuery.isLoading && !versionsQuery.isError && versions.length === 0 ? (
            <div className="rounded-lg border border-dashed px-5 py-12 text-center" data-testid="vm-config-versions-empty">
              <History className="mx-auto h-6 w-6 text-muted-foreground" />
              <p className="mt-3 text-sm font-medium">还没有配置版本</p>
              <p className="mt-1 text-xs text-muted-foreground">下一次配置变更或外部 XML 修改会被自动归档。</p>
            </div>
          ) : null}

          {versions.length > 0 ? (
            <div className="grid gap-4 lg:grid-cols-[minmax(16rem,20rem)_minmax(0,1fr)]">
              <div className="max-h-[38rem] space-y-2 overflow-y-auto pr-1" data-testid="vm-config-version-list">
                {versions.map((version) => (
                  <button
                    key={version.versionNo}
                    type="button"
                    data-testid={`vm-config-version-item-${version.versionNo}`}
                    onClick={() => { setSelectedVersion(version.versionNo); setCopyNotice(""); }}
                    className={`w-full rounded-lg border p-3 text-left transition-colors ${selectedVersion === version.versionNo ? "border-primary bg-primary/5" : "bg-card hover:bg-accent/50"}`}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-mono text-sm font-semibold">v{version.versionNo}</span>
                      <div className="flex items-center gap-1">
                        {version.protected ? <Badge variant="outline"><Pin className="h-3 w-3" />已固定</Badge> : null}
                        {version.isCurrent ? <Badge variant="success">当前</Badge> : null}
                      </div>
                    </div>
                    <p className="mt-1.5 text-xs font-medium">{configVersionSourceLabel(version.source)}{version.sourceVersion ? ` · 自 v${version.sourceVersion}` : ""}</p>
                    <p className="mt-1 truncate text-[11px] text-muted-foreground">{timeFormatter.format(new Date(version.createdAt))}</p>
                    <div className="mt-2 flex items-center justify-between gap-2 text-[11px] text-muted-foreground">
                      <span className="truncate">{version.actor || "system"}</span>
                      <span className="font-mono">{shortConfigVersionSHA(version.xmlSha256)}</span>
                    </div>
                  </button>
                ))}
              </div>

              <div className="min-w-0 space-y-3">
                {!versionsQuery.data?.currentCaptured ? (
                  <div className="rounded-md border border-warning/35 bg-warning/10 px-3 py-2 text-xs text-warning" data-testid="vm-config-version-unarchived">
                    当前持久 XML 尚未出现在历史中，可能刚被外部工具修改。回滚前系统仍会自动存档当前配置。
                  </div>
                ) : null}
                {detailQuery.isLoading ? <LoadingState label="正在读取版本 XML" /> : null}
                {detailQuery.isError ? (
                  <ErrorState
                    message={detailQuery.error instanceof Error ? detailQuery.error.message : "无法读取该版本 XML"}
                    onRetry={() => void detailQuery.refetch()}
                  />
                ) : null}
                {selected && detailQuery.data ? (
                  <>
                    <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border bg-muted/20 px-3 py-2.5">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <Badge variant={selected.isCurrent ? "success" : "outline"}>v{selected.versionNo}</Badge>
                          <span className="text-xs font-medium">{configVersionSourceLabel(selected.source)}</span>
                          {selected.protected ? <Badge variant="default"><Pin className="h-3 w-3" />已固定</Badge> : null}
                        </div>
                        <p className="mt-1 text-[11px] text-muted-foreground">
                          {selected.actor || "system"} · {timeFormatter.format(new Date(selected.createdAt))} · {formatBytes(selected.sizeBytes)} · SHA-256 <span className="font-mono">{shortConfigVersionSHA(selected.xmlSha256)}</span>
                        </p>
                        {selected.note ? <p className="mt-1 text-[11px] text-muted-foreground">{selected.note}</p> : null}
                      </div>
                      <div className="flex flex-wrap items-center gap-2">
                        <Button variant="outline" size="sm" data-testid="vm-config-version-copy" onClick={() => void copySelectedXML()}>
                          {copyNotice === "copied" ? <Check className="h-3.5 w-3.5" /> : <Clipboard className="h-3.5 w-3.5" />}复制
                        </Button>
                        <Button variant="outline" size="sm" data-testid="vm-config-version-download" onClick={downloadSelectedXML}>
                          <Download className="h-3.5 w-3.5" />下载
                        </Button>
                        {role === "admin" && !readOnly ? (
                          <Button
                            variant="outline"
                            size="sm"
                            data-testid="vm-config-version-pin"
                            disabled={pinMutation.isPending}
                            onClick={() => pinMutation.mutate(selected)}
                          >
                            {selected.protected ? <PinOff className="h-3.5 w-3.5" /> : <Pin className="h-3.5 w-3.5" />}
                            {selected.protected ? "取消固定" : "固定版本"}
                          </Button>
                        ) : null}
                        <Button
                          size="sm"
                          variant={selected.isCurrent ? "outline" : "destructive"}
                          data-testid="vm-config-version-rollback"
                          disabled={Boolean(rollbackReason) || rollbackMutation.isPending}
                          title={rollbackReason || `将持久配置回滚到 v${selected.versionNo}`}
                          onClick={() => { setNotice(null); setRollbackOpen(true); }}
                        >
                          {rollbackMutation.isPending ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" /> : <RotateCcw className="h-3.5 w-3.5" />}回滚到此版本
                        </Button>
                      </div>
                    </div>
                    {copyNotice === "error" ? <p className="text-xs text-destructive">复制失败，请下载 XML 后手动保存。</p> : null}
                    <div className="grid gap-3 xl:grid-cols-2">
                      <section className="overflow-hidden rounded-lg border">
                        <header className="border-b bg-muted/30 px-3 py-2 text-xs font-medium">当前持久配置</header>
                        {currentXml ? (
                          <div className="h-[24rem] overflow-hidden">
                            <Suspense fallback={<LoadingState label="正在加载 XML 视图" />}>
                              <XMLCodeEditor value={currentXml} readOnly ariaLabel="当前持久配置 XML" testId="vm-config-version-current-xml" format="config" className="h-full" />
                            </Suspense>
                          </div>
                        ) : <LoadingState label="正在加载当前持久配置" />}
                      </section>
                      <section className="overflow-hidden rounded-lg border">
                        <header className="border-b bg-muted/30 px-3 py-2 text-xs font-medium">历史版本 v{selected.versionNo}</header>
                        <div className="h-[24rem] overflow-hidden">
                          <Suspense fallback={<LoadingState label="正在加载 XML 视图" />}>
                            <XMLCodeEditor value={detailQuery.data.xml} readOnly ariaLabel={`历史版本 v${selected.versionNo} XML`} testId="vm-config-version-xml" format="config" className="h-full" />
                          </Suspense>
                        </div>
                      </section>
                    </div>
                  </>
                ) : null}
              </div>
            </div>
          ) : null}
        </div>
      </Modal>

      <Modal
        open={rollbackOpen}
        onClose={() => { if (!rollbackMutation.isPending) setRollbackOpen(false); }}
        title={`回滚持久配置到 v${selected?.versionNo ?? "?"}？`}
        description="回滚会覆盖当前持久域 XML，并先自动归档当前配置。磁盘、快照和运行时内存不会恢复。"
        testId="vm-config-version-rollback-dialog"
        className="max-w-lg"
        footer={
          <>
            <Button variant="outline" disabled={rollbackMutation.isPending} onClick={() => setRollbackOpen(false)}>取消</Button>
            <Button
              variant="destructive"
              data-testid="vm-config-version-rollback-confirm"
              disabled={rollbackMutation.isPending || !selected}
              onClick={() => { if (selected) rollbackMutation.mutate(selected); }}
            >
              {rollbackMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <RotateCcw className="h-4 w-4" />}确认回滚
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">
            运行中的虚拟机只会更新持久定义，需重启后生效。回滚不会自动重启虚拟机。
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="vm-config-rollback-note">操作备注（可选）</Label>
            <Input
              id="vm-config-rollback-note"
              data-testid="vm-config-version-rollback-note"
              maxLength={1024}
              value={rollbackNote}
              onChange={(event) => setRollbackNote(event.target.value)}
              placeholder="例如：修复错误的内存配置"
            />
          </div>
          {rollbackMutation.isError ? <p className="text-xs text-destructive">{rollbackMutation.error instanceof Error ? rollbackMutation.error.message : "回滚失败"}</p> : null}
        </div>
      </Modal>
    </>
  );
}

import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, LoaderCircle, Tags, TriangleAlert } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { VM } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { vmQueryKey } from "@/features/vms/use-vm-query";

function tagText(tags?: string[]) {
  return (tags ?? []).join(", ");
}

export function VMMetadataCard({ vm, canMutate, disabledReason }: {
  vm: VM;
  canMutate: boolean;
  disabledReason: string;
}) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [tags, setTags] = useState(tagText(vm.metadata?.tags));
  const [note, setNote] = useState(vm.metadata?.note ?? "");
  const [owner, setOwner] = useState(vm.metadata?.owner ?? "");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  useEffect(() => {
    if (editing) return;
    setTags(tagText(vm.metadata?.tags));
    setNote(vm.metadata?.note ?? "");
    setOwner(vm.metadata?.owner ?? "");
  }, [editing, vm.metadata]);

  const mutation = useMutation({
    mutationFn: () => api.updateVMMetadata(vm.uuid, {
      tags: tags.split(",").map((item) => item.trim()).filter(Boolean),
      note: note.trim(),
      owner: owner.trim(),
    }),
    onSuccess: (metadata) => {
      queryClient.setQueryData<VM>(vmQueryKey(vm.uuid), (current) => current ? { ...current, metadata } : current);
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
      setEditing(false);
      setError("");
      setNotice("标签与备注已保存。");
    },
    onError: (cause) => setError(cause instanceof ApiError ? cause.message : "保存虚拟机标签失败，请稍后重试。"),
  });

  return (
    <Card className="mt-4" data-testid="vm-metadata-card">
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2"><Tags className="h-4 w-4 text-primary" />标签与备注</CardTitle>
            <CardDescription>用于分组、检索和记录虚拟机负责人；元数据保存在 VirtMgr 数据库中。</CardDescription>
          </div>
          <div className="flex items-center gap-2">
            {vm.metadata?.updatedBy ? <Badge variant="outline">最近更新 {vm.metadata.updatedBy}</Badge> : null}
            {!editing ? <Button size="sm" variant="outline" disabled={!canMutate} title={canMutate ? "编辑标签与备注" : disabledReason} onClick={() => { setNotice(""); setError(""); setEditing(true); }}>编辑</Button> : null}
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {editing ? (
          <div className="space-y-3">
            <div className="space-y-2">
              <Label htmlFor="vm-metadata-tags">标签</Label>
              <Input id="vm-metadata-tags" data-testid="vm-metadata-tags" value={tags} maxLength={1024} placeholder="prod, database, 华东" onChange={(event) => setTags(event.target.value)} />
              <p className="text-xs text-muted-foreground">使用逗号分隔，最多 20 个；自动去重和忽略空白。</p>
            </div>
            <div className="grid gap-3 md:grid-cols-2">
              <div className="space-y-2"><Label htmlFor="vm-metadata-owner">负责人</Label><Input id="vm-metadata-owner" data-testid="vm-metadata-owner" value={owner} maxLength={128} placeholder="例如 platform-team" onChange={(event) => setOwner(event.target.value)} /></div>
              <div className="space-y-2"><Label htmlFor="vm-metadata-note">备注</Label><Textarea id="vm-metadata-note" data-testid="vm-metadata-note" value={note} maxLength={2000} rows={3} placeholder="用途、维护窗口或联系信息" onChange={(event) => setNote(event.target.value)} /></div>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Button size="sm" data-testid="vm-metadata-save" disabled={mutation.isPending} onClick={() => mutation.mutate()}>
                {mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <CheckCircle2 className="h-4 w-4" />}
                {mutation.isPending ? "保存中..." : "保存"}
              </Button>
              <Button size="sm" variant="ghost" disabled={mutation.isPending} onClick={() => { setEditing(false); setError(""); }}>取消</Button>
            </div>
          </div>
        ) : (
          <div className="space-y-3">
            <div className="flex flex-wrap gap-1.5">
              {vm.metadata?.tags?.length ? vm.metadata.tags.map((tag) => <Badge key={tag} variant="secondary">{tag}</Badge>) : <span className="text-sm text-muted-foreground">未设置标签</span>}
            </div>
            <div className="grid gap-3 text-sm md:grid-cols-2">
              <div><p className="text-xs text-muted-foreground">负责人</p><p className="mt-1">{vm.metadata?.owner || "-"}</p></div>
              <div><p className="text-xs text-muted-foreground">备注</p><p className="mt-1 whitespace-pre-wrap">{vm.metadata?.note || "-"}</p></div>
            </div>
          </div>
        )}
        {error ? <p role="alert" className="flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive"><TriangleAlert className="h-3.5 w-3.5" />{error}</p> : null}
        {notice ? <p role="status" className="rounded-md border border-success/30 bg-success/10 px-3 py-2 text-xs text-success">{notice}</p> : null}
      </CardContent>
    </Card>
  );
}

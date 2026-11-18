import { useEffect, useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { Archive, LoaderCircle, RotateCcw, ShieldCheck } from "lucide-react";
import { api } from "@/api/client";
import type { Backup, BackupRepository, VMRestoreInput } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Modal } from "@/components/ui/modal";
import { backupName, errorText, formatBytes, type BackupNotice } from "@/features/backups/backup-ui";


export function CreateBackupModal({ open, onClose, repositories, vms, onCreated, onNotice }: {
  open: boolean;
  onClose: () => void;
  repositories: BackupRepository[];
  vms: Array<{ uuid: string; name: string; state: string }>;
  onCreated: (taskID: string) => void;
  onNotice: (notice: BackupNotice) => void;
}) {
  const [vmUuid, setVMUuid] = useState("");
  const [repositoryID, setRepositoryID] = useState("");
  const [name, setName] = useState("");
  const [online, setOnline] = useState(false);
  const [quiesce, setQuiesce] = useState(false);
  const [error, setError] = useState("");
  const repo = repositories.find((item) => item.id === repositoryID);

  useEffect(() => {
    if (!open) return;
    const vm = vms[0];
    const repository = repositories.find((item) => item.isDefault) ?? repositories[0];
    setVMUuid(vm?.uuid ?? "");
    setName(vm ? backupName(vm.name) : "");
    setRepositoryID(repository?.id ?? "");
    setOnline(vm?.state === "running");
    setQuiesce(false);
    setError("");
  }, [open, repositories, vms]);

  const mutation = useMutation({ mutationFn: api.createBackup });
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (mutation.isPending) return;
    if (!vmUuid || !repo || !name.trim()) { setError("请选择虚拟机、备份名称和存储库。"); return; }
    const selectedVM = vms.find((item) => item.uuid === vmUuid);
    if (selectedVM?.state === "running" && !online) { setError("运行中的虚拟机必须启用在线一致性备份。"); return; }
    try {
      const task = await mutation.mutateAsync({ vmUuid, name: name.trim(), repositoryId: repo.id, targetPool: repo.poolName, online: selectedVM?.state === "running", quiesce: selectedVM?.state === "running" && quiesce });
      onCreated(task.id);
    } catch (cause) {
      setError(errorText(cause, "创建备份失败"));
      onNotice({ tone: "error", text: errorText(cause, "创建备份失败") });
    }
  }
  function close() { if (!mutation.isPending) onClose(); }
  return <Modal open={open} onClose={close} title="创建备份" description="关机虚拟机执行冷备份；运行中虚拟机通过外部磁盘快照执行在线一致性备份。" testId="backup-create-modal" footer={<><Button variant="ghost" disabled={mutation.isPending} onClick={close}>取消</Button><Button form="backup-create-form" type="submit" disabled={mutation.isPending || !vms.length || !repositories.length}>{mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Archive className="h-4 w-4" />}{mutation.isPending ? "正在提交" : "开始备份"}</Button></>}>
    <form id="backup-create-form" className="space-y-4" onSubmit={(event) => void submit(event)}>
      <div className="space-y-2"><Label htmlFor="backup-vm">虚拟机</Label><select id="backup-vm" data-testid="backup-create-vm" value={vmUuid} onChange={(event) => { const vm = vms.find((item) => item.uuid === event.target.value); setVMUuid(event.target.value); if (vm) setName(backupName(vm.name)); setOnline(vm?.state === "running"); if (vm?.state !== "running") setQuiesce(false); }} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">
        {vms.length ? vms.map((vm) => <option key={vm.uuid} value={vm.uuid}>{vm.name} · {vm.state === "crashed" ? "异常停止" : vm.state === "running" ? "运行中" : "已关机"}</option>) : <option value="">没有可备份的虚拟机</option>}
      </select><p className="text-xs text-muted-foreground">运行中虚拟机将使用外部磁盘快照，备份期间保持运行；备份不包含内存状态。</p></div>
      <div className="space-y-2"><Label htmlFor="backup-name">备份名称</Label><Input id="backup-name" data-testid="backup-create-name" value={name} onChange={(event) => setName(event.target.value)} maxLength={64} /></div>
      <div className="space-y-2"><Label htmlFor="backup-repository">备份存储库</Label><select id="backup-repository" data-testid="backup-create-repository" value={repositoryID} onChange={(event) => setRepositoryID(event.target.value)} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">{repositories.map((repository) => <option key={repository.id} value={repository.id}>{repository.name} · {repository.poolName}{repository.isDefault ? " · 默认" : ""}</option>)}</select></div>
      <div className="grid gap-2 sm:grid-cols-2">
        <label className="flex items-center gap-2 rounded-md border p-3 text-sm"><input type="checkbox" checked={online} onChange={(event) => setOnline(event.target.checked)} disabled={vms.find((item) => item.uuid === vmUuid)?.state !== "running"} />运行中在线备份</label>
        <label className="flex items-center gap-2 rounded-md border p-3 text-sm"><input type="checkbox" checked={quiesce} onChange={(event) => setQuiesce(event.target.checked)} disabled={!online} />Guest Agent 静默</label>
      </div>
      <p className="rounded-md border border-warning/30 bg-warning/10 p-3 text-xs text-warning">静默需要客户机已安装并运行 QEMU Guest Agent；未启用时仍会取得崩溃一致性的磁盘快照。vTPM 状态不会进入备份，恢复后需使用 BitLocker 恢复密钥。</p>
      {error ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{error}</p> : null}
    </form>
  </Modal>;
}

export function VerifyBackupModal({ target, onClose, onStarted, onNotice }: {
  target: Backup | null;
  onClose: () => void;
  onStarted: (taskID: string) => void;
  onNotice: (notice: BackupNotice) => void;
}) {
  const [deep, setDeep] = useState(false);
  const mutation = useMutation({ mutationFn: ({ id, deep }: { id: string; deep: boolean }) => api.verifyBackup(id, deep) });
  useEffect(() => { if (target) setDeep(false); }, [target]);
  function close() { if (!mutation.isPending) onClose(); }
  async function execute() {
    if (!target || mutation.isPending) return;
    try {
      const task = await mutation.mutateAsync({ id: target.id, deep });
      onStarted(task.id);
    } catch (cause) {
      onNotice({ tone: "error", text: errorText(cause, "启动校验失败") });
    }
  }
  return <Modal open={Boolean(target)} onClose={close} title="校验备份完整性" description={target ? `${target.name} · ${target.vmName}` : ""} testId="backup-verify-modal" footer={<><Button variant="ghost" disabled={mutation.isPending} onClick={close}>取消</Button><Button disabled={mutation.isPending} onClick={() => void execute()}>{mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <ShieldCheck className="h-4 w-4" />}{mutation.isPending ? "正在启动" : "开始校验"}</Button></>}>
    <div className="space-y-3 text-sm">
      <label className="flex items-start gap-3 rounded-md border p-3"><input type="checkbox" className="mt-1" checked={deep} onChange={(event) => setDeep(event.target.checked)} /><span><span className="font-medium">深度校验</span><span className="mt-1 block text-xs text-muted-foreground">额外检查每个磁盘产物是否存在、容量和格式是否匹配。不会计算整块磁盘 SHA-256，避免大磁盘产生持续 IO。</span></span></label>
      <p className="rounded-md bg-muted/40 p-3 text-xs text-muted-foreground">快速校验会检查 manifest、配置 XML 的 SHA-256、以及全部磁盘产物是否存在。</p>
    </div>
  </Modal>;
}

export function RestoreBackupModal({ target, repositories, onClose, onStarted, onNotice }: {
  target: Backup | null;
  repositories: BackupRepository[];
  onClose: () => void;
  onStarted: (taskID: string) => void;
  onNotice: (notice: BackupNotice) => void;
}) {
  const [form, setForm] = useState<VMRestoreInput>({ name: "", targetPool: "", autostart: false, start: false, preservePassthrough: false });
  const [error, setError] = useState("");
  const [confirmOpen, setConfirmOpen] = useState(false);
  const mutation = useMutation({ mutationFn: ({ id, input }: { id: string; input: VMRestoreInput }) => api.restoreBackup(id, input) });
  useEffect(() => {
    if (!target) return;
    setForm({ name: `${target.vmName}-restored`.slice(0, 64), targetPool: target.targetPool, autostart: false, start: false, preservePassthrough: false });
    setError("");
    setConfirmOpen(false);
  }, [target]);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!target || !form.name.trim() || !form.targetPool) { setError("请填写新虚拟机名称并选择目标存储池。"); return; }
    setError("");
    setConfirmOpen(true);
  }
  async function execute() {
    if (!target || mutation.isPending) return;
    try {
      const task = await mutation.mutateAsync({ id: target.id, input: { ...form, name: form.name.trim() } });
      onStarted(task.id);
    } catch (cause) {
      setError(errorText(cause, "恢复虚拟机失败"));
      onNotice({ tone: "error", text: errorText(cause, "恢复虚拟机失败") });
    }
  }
  function close() { if (!mutation.isPending) onClose(); }
  return <Modal open={Boolean(target)} onClose={close} title="恢复为新虚拟机" description="原备份和原虚拟机不会被修改；恢复会生成新的 UUID、MAC 和独立磁盘。" testId="backup-restore-modal" footer={confirmOpen ? <><Button variant="ghost" disabled={mutation.isPending} onClick={() => setConfirmOpen(false)}>返回修改</Button><Button disabled={mutation.isPending} onClick={() => void execute()}>{mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <RotateCcw className="h-4 w-4" />}{mutation.isPending ? "正在恢复" : "确认恢复"}</Button></> : <><Button variant="ghost" disabled={mutation.isPending} onClick={close}>取消</Button><Button form="backup-restore-form" type="submit"><RotateCcw className="h-4 w-4" />预检并恢复</Button></>}>
    {target ? <form id="backup-restore-form" className="space-y-4" onSubmit={(event) => void submit(event)}>
      <div className="rounded-md border bg-muted/20 p-3 text-sm"><p className="font-medium">{target.name}</p><p className="mt-1 text-xs text-muted-foreground">来源 {target.vmName} · {formatBytes(target.totalBytes)} · {target.diskCount} 块磁盘</p></div>
      <div className="space-y-2"><Label htmlFor="restore-name">新虚拟机名称</Label><Input id="restore-name" data-testid="restore-name" value={form.name} maxLength={64} onChange={(event) => setForm({ ...form, name: event.target.value })} /></div>
      <div className="space-y-2"><Label htmlFor="restore-pool">目标存储池</Label><select id="restore-pool" data-testid="restore-pool" value={form.targetPool} onChange={(event) => setForm({ ...form, targetPool: event.target.value })} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">{[...new Set([target.targetPool, ...repositories.map((item) => item.poolName)])].map((pool) => <option key={pool} value={pool}>{pool}</option>)}</select></div>
      <div className="grid gap-2 sm:grid-cols-3">
        <label className="flex items-center gap-2 rounded-md border p-3 text-sm"><input type="checkbox" checked={form.autostart} onChange={(event) => setForm({ ...form, autostart: event.target.checked })} />开机自启动</label>
        <label className="flex items-center gap-2 rounded-md border p-3 text-sm"><input type="checkbox" checked={form.start} onChange={(event) => setForm({ ...form, start: event.target.checked })} />恢复后启动</label>
        <label className="flex items-center gap-2 rounded-md border p-3 text-sm"><input type="checkbox" checked={form.preservePassthrough} onChange={(event) => setForm({ ...form, preservePassthrough: event.target.checked })} />保留 PCI/USB 直通</label>
      </div>
      <p className="rounded-md border border-warning/30 bg-warning/10 p-3 text-xs text-warning">默认移除 PCI/USB 直通设备，避免与仍在运行的虚拟机争用设备。恢复前会检查名称冲突、存储池容量和备份产物。</p>
      {confirmOpen ? <div className="rounded-md border border-primary/30 bg-primary/5 p-3 text-sm">即将创建 <span className="font-semibold">{form.name}</span>，目标存储池为 <span className="font-mono">{form.targetPool}</span>。此操作会复制全部磁盘，可能耗时较长。</div> : null}
      {error ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{error}</p> : null}
    </form> : null}
  </Modal>;
}

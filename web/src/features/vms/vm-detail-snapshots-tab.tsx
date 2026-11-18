import type { Role, VM } from "@/api/types";
import { VMBackupCard } from "@/features/vms/vm-backup-card";
import { VMRecentBackupsCard } from "@/features/vms/vm-recent-backups-card";
import { VMSnapshotPoliciesCard } from "@/features/vms/vm-snapshot-policies-card";
import { VMSnapshotsCard } from "@/features/vms/vm-snapshots-card";

export function VMDetailSnapshotsTab({ vm, role, readOnly, canMutate, disabledReason, backupDisabledReason }: {
  vm: VM;
  role: Role | undefined;
  readOnly: boolean;
  canMutate: boolean;
  disabledReason: string;
  backupDisabledReason: string;
}) {
  return <div role="tabpanel" id="vm-tabpanel-snapshots" aria-labelledby="vm-tab-snapshots" data-testid="vm-tabpanel-snapshots">
    <VMSnapshotPoliciesCard uuid={vm.uuid} canMutate={canMutate} disabledReason={disabledReason} />
    <VMSnapshotsCard uuid={vm.uuid} canMutate={canMutate} disabledReason={disabledReason} />
    <VMBackupCard uuid={vm.uuid} vmName={vm.name} state={vm.state} persistent={vm.persistent} role={role} readOnly={readOnly} disabledReason={backupDisabledReason} />
    <VMRecentBackupsCard uuid={vm.uuid} />
  </div>;
}

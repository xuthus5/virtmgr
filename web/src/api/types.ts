export type Role = "viewer" | "operator" | "admin";

export type User = {
  id: number;
  username: string;
  role: Role;
  disabled: boolean;
  createdAt: string;
};

export type Session = {
  user: User;
  authMode: string;
  readOnly: boolean;
  csrfToken: string;
  expiresAt: string;
};

export type Settings = {
  readOnly: boolean;
};

export type SettingsUpdateInput = {
  readOnly: boolean;
};

export type UserList = {
  items: User[];
  total: number;
};

export type UserCreateInput = {
  username: string;
  password: string;
  role: Role;
};

export type UserUpdateInput = {
  role: Role;
  disabled: boolean;
};

export type VMState = "running" | "blocked" | "paused" | "shutting-down" | "crashed" | "suspended" | "shutoff" | "unknown";

export type Disk = {
  type?: string;
  device: string;
  target: string;
  bus: string;
  format?: string;
  source?: string;
  readOnly: boolean;
  capacityBytes: number;
  allocationBytes: number;
};

export type NetworkInterface = {
  type: string;
  mac?: string;
  source?: string;
  model?: string;
  address?: string;
};

export type Graphics = {
  type: string;
  listen?: string;
  port?: number;
};

export type PassthroughDevice = {
  name: string;
  type: "pci" | "usb";
  vendor?: string;
  vendorId?: string;
  product?: string;
  productId?: string;
  address?: string;
  driver?: string;
};

export type VMDeviceInput = {
  device: string;
};

export type VMAction = "start" | "shutdown" | "reboot" | "reset" | "pause" | "resume" | "destroy";

export type VMAutostartInput = {
  enabled: boolean;
};

export type VMResourcesInput = {
  vcpus: number;
  memoryMiB: number;
};

export type VMDiskResizeInput = {
  capacityBytes: number;
};

export type VMFirmware = "bios" | "uefi";

export type CloudInitInput = {
  userData: string;
  pool: string;
};

export type VMBatchActionInput = {
  action: VMAction;
  uuids: string[];
};

export type VMBatchActionItem = {
  uuid: string;
  name?: string;
  success: boolean;
  error?: string;
  vm?: VM;
};

export type VMBatchActionResult = {
  action: VMAction;
  items: VMBatchActionItem[];
  succeeded: number;
  failed: number;
};

export type VMCreateInput = {
  name: string;
  vcpus: number;
  memoryMiB: number;
  diskPath: string;
  diskFormat: string;
  network: string;
  isoPath?: string;
  firmware: VMFirmware;
  secureBoot: boolean;
  cloudInit?: CloudInitInput;
  autostart: boolean;
  start: boolean;
};

export type VMTemplateInput = {
  name: string;
  description: string;
  vcpus: number;
  memoryMiB: number;
  diskFormat: string;
  network: string;
  isoPath?: string;
  firmware: VMFirmware;
  secureBoot: boolean;
  cloudInit: boolean;
  cloudInitUserData?: string;
  autostart: boolean;
  start: boolean;
};

export type VMTemplate = VMTemplateInput & {
  id: number;
  createdAt: string;
  updatedAt: string;
};

export type VMTemplateList = {
  items: VMTemplate[];
  total: number;
};

export type VMCloneInput = {
  name: string;
  targetPool?: string;
  autostart: boolean;
  start: boolean;
};

export type VMBackupInput = {
  name: string;
  targetPool: string;
};

export type VMBackupDisk = {
  sourceVolume: string;
  backupVolume: string;
  format: string;
  capacityBytes: number;
};

export type VMBackup = {
  formatVersion: string;
  createdAt: string;
  vmUuid: string;
  vmName: string;
  targetPool: string;
  name: string;
  manifestVolume: string;
  configVolume: string;
  configBytes: number;
  disks: VMBackupDisk[];
  totalBytes: number;
};

export type BackupStatus = "running" | "completed" | "failed" | "interrupted" | "deleting" | "delete_failed";

export type BackupVerifyStatus = "unknown" | "verified" | "failed";

export type BackupDisk = {
  sequence: number;
  target?: string;
  sourceVolume: string;
  artifact: string;
  format: string;
  capacityBytes: number;
  sha256?: string;
};

export type Backup = {
  id: string;
  vmUuid: string;
  vmName: string;
  repositoryId: string;
  repositoryName?: string;
  targetPool: string;
  name: string;
  status: BackupStatus;
  formatVersion: string;
  manifestVolume: string;
  configVolume: string;
  configBytes: number;
  totalBytes: number;
  diskCount: number;
  pinned: boolean;
  verifyStatus: BackupVerifyStatus;
  verifiedAt?: string;
  verifyMessage?: string;
  error?: string;
  createdBy: string;
  createdAt: string;
  completedAt?: string;
  disks?: BackupDisk[];
};

export type BackupList = {
  items: Backup[];
  total: number;
  page: number;
  pageSize: number;
};

export type BackupSummary = {
  total: number;
  completed: number;
  failed: number;
  running: number;
  pinned: number;
  totalBytes: number;
  lastCompletedAt?: string;
};

export type BackupFilters = {
  vmUuid?: string;
  status?: BackupStatus | "";
  repositoryId?: string;
  page?: number;
  pageSize?: number;
};

export type BackupCreateInput = {
  vmUuid: string;
  name: string;
  targetPool: string;
  repositoryId?: string;
};

export type VMRestoreInput = {
  name: string;
  targetPool: string;
  autostart: boolean;
  start: boolean;
  preservePassthrough: boolean;
};

export type BackupPinInput = {
  pinned: boolean;
};

export type BackupArtifactCheck = {
  name: string;
  kind: "manifest" | "config" | "disk";
  exists: boolean;
  format?: string;
  capacityBytes?: number;
  expectedBytes?: number;
  message?: string;
};

export type BackupVerification = {
  backupId: string;
  status: BackupVerifyStatus;
  deep: boolean;
  checkedAt: string;
  message: string;
  artifacts: BackupArtifactCheck[];
};

export type BackupRepository = {
  id: string;
  name: string;
  type: "libvirt-pool";
  poolName: string;
  isDefault: boolean;
  createdAt: string;
  updatedAt: string;
};

export type BackupRepositoryList = {
  items: BackupRepository[];
  total: number;
};

export type BackupRepositoryInput = {
  name: string;
  poolName: string;
  isDefault: boolean;
};

export type BackupSchedule = {
  id: number;
  vmUuid: string;
  vmName?: string;
  repositoryId: string;
  repositoryName?: string;
  targetPool: string;
  name: string;
  intervalHours: number;
  retentionCount: number;
  retentionDays: number;
  enabled: boolean;
  lastRunAt?: string;
  nextRunAt: string;
  lastBackupId?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
};

export type BackupScheduleInput = {
  vmUuid: string;
  repositoryId: string;
  name: string;
  intervalHours: number;
  retentionCount: number;
  retentionDays: number;
  enabled: boolean;
};

export type BackupScheduleList = {
  items: BackupSchedule[];
  total: number;
};

export type VM = {
  uuid: string;
  name: string;
  state: VMState;
  stateReason: number;
  persistent: boolean;
  autostart: boolean;
  vcpus: number;
  maxVcpus: number;
  memoryBytes: number;
  maxMemoryBytes: number;
  cpuTimeNanos: number;
  osType?: string;
  arch?: string;
  machine?: string;
  firmware: VMFirmware;
  secureBoot: boolean;
  cloudInit: boolean;
  disks: Disk[];
  interfaces: NetworkInterface[];
  graphics: Graphics[];
  serials?: SerialConsole[];
  passthrough: PassthroughDevice[];
  ipAddresses: string[];
  hostname?: string;
  agentConnected: boolean;
  allowedActions: VMAction[];
};

export type SerialConsole = {
  device?: string;
  label: string;
  targetPort: number;
  targetType?: string;
};

export type VMXMLFormat = "live" | "config";

export type VMXML = {
  uuid: string;
  name: string;
  format: VMXMLFormat;
  xml: string;
};

export type VMXMLUpdateInput = {
  xml: string;
};

export type VMConfigVersionSource =
  | "manual_xml"
  | "resources"
  | "passthrough"
  | "snapshot_revert"
  | "external"
  | "create"
  | "clone"
  | "backup_restore"
  | "rollback"
  | "pre_rollback"
  | "disk_resize"
  | "baseline";

export type VMConfigVersion = {
  vmUuid: string;
  versionNo: number;
  xmlSha256: string;
  sizeBytes: number;
  source: VMConfigVersionSource;
  sourceVersion?: number;
  actor: string;
  role: Role;
  requestId?: string;
  sourceIp?: string;
  note?: string;
  protected: boolean;
  createdAt: string;
  isCurrent: boolean;
};

export type VMConfigVersionDetail = VMConfigVersion & {
  xml: string;
};

export type VMConfigVersionList = {
  items: VMConfigVersion[];
  total: number;
  currentSha256: string;
  currentCaptured: boolean;
};

export type VMConfigVersionRollbackInput = {
  xmlSha256: string;
  note?: string;
};

export type VMConfigVersionProtectInput = {
  protected: boolean;
};

export type VMExport = {
  formatVersion: string;
  exportedAt: string;
  vm: VM;
  configXml: VMXML;
  liveXml?: VMXML;
  snapshots: VMSnapshotList;
  disclaimer: string;
};

export type ConsoleTicket = {
  token: string;
  expiresAt: string;
  path: string;
  kind: ConsoleKind;
  device?: string;
};

export type ConsoleKind = "vnc" | "serial";

export type ConsoleTokenInput = {
  kind: ConsoleKind;
  device?: string;
};

export type ConsoleKeyAction =
  | "ctrl-alt-del"
  | "ctrl-alt-backspace"
  | "ctrl-alt-f1"
  | "ctrl-alt-f2"
  | "ctrl-alt-f3"
  | "ctrl-alt-f4"
  | "ctrl-alt-f5"
  | "ctrl-alt-f6"
  | "ctrl-alt-f7"
  | "ctrl-alt-f8"
  | "ctrl-alt-f9"
  | "ctrl-alt-f10"
  | "ctrl-alt-f11"
  | "ctrl-alt-f12";

export type SendConsoleKeyInput = {
  key: ConsoleKeyAction;
};

export type VMSnapshot = {
  name: string;
  description?: string;
  createdAt: number;
  state: VMState;
  current: boolean;
  parent?: string;
};

export type VMSnapshotCreateInput = {
  name: string;
  description?: string;
};

export type VMSnapshotList = {
  items: VMSnapshot[];
  total: number;
};

export type SnapshotPolicyInput = {
  name: string;
  intervalMinutes: number;
  retention: number;
  enabled: boolean;
};

export type SnapshotPolicy = SnapshotPolicyInput & {
  id: number;
  vmUuid: string;
  lastRunAt?: string;
  nextRunAt: string;
  lastSnapshot?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
};

export type SnapshotPolicyList = {
  items: SnapshotPolicy[];
  total: number;
};

export type SnapshotPolicyRunResult = {
  policy: SnapshotPolicy;
  snapshot?: VMSnapshot;
  deletedSnapshots?: string[];
};

export type Host = {
  name: string;
  libvirtVersion: string;
  cpuModel: string;
  cpus: number;
  cpuSockets: number;
  cpuCores: number;
  cpuThreads: number;
  cpuMhz: number;
  memoryBytes: number;
};

export type HostDetails = {
  host: Host;
  counts: VMCounts;
  allocatedVcpus: number;
  allocatedMemoryBytes: number;
  vms: VM[];
  storagePools: StoragePool[];
  networks: VirtualNetwork[];
  warnings?: string[];
};

export type ResourceMetrics = {
  timestamp: number;
  cpuPercent: number;
  memoryUsedBytes: number;
  memoryTotalBytes: number;
  networkRxBytes: number;
  networkTxBytes: number;
  networkRxBytesPerSecond: number;
  networkTxBytesPerSecond: number;
};

export type HostMetrics = ResourceMetrics & {
  hostName: string;
};

export type VMMetrics = ResourceMetrics & {
  uuid: string;
};

export type HostDevice = {
  name: string;
  type: string;
  parent?: string;
  vendor?: string;
  vendorId?: string;
  product?: string;
  productId?: string;
  address?: string;
  driver?: string;
  active: boolean;
  persistent: boolean;
  capabilities: string[];
};

export type HostDeviceList = {
  items: HostDevice[];
  total: number;
};

export type EventSeverity = "info" | "warning" | "critical";

export type SystemEvent = {
  id: number;
  occurredAt: string;
  source: string;
  kind: string;
  severity: EventSeverity;
  targetKind: string;
  targetId: string;
  targetName?: string;
  message: string;
  detail?: Record<string, unknown>;
};

export type VMCounts = {
  total: number;
  running: number;
  paused: number;
  shutoff: number;
  failed: number;
};

export type Overview = {
  host: Host;
  counts: VMCounts;
  allocatedVcpus: number;
  allocatedMemoryBytes: number;
  vms: VM[];
  warnings?: string[];
};

export type VMSummaryList = {
  items: VM[];
  total: number;
};

export type StoragePoolState = "active" | "inactive" | "degraded" | "inaccessible" | "unknown";

export type StoragePoolAction = "start" | "stop" | "refresh";

export type StoragePool = {
  name: string;
  uuid: string;
  type: string;
  state: StoragePoolState;
  autostart: boolean;
  persistent: boolean;
  targetPath?: string;
  capacityBytes: number;
  allocationBytes: number;
  availableBytes: number;
  volumeCount: number;
};

export type StoragePoolCreateInput = {
  name: string;
  type: "dir";
  targetPath: string;
  autostart: boolean;
  start: boolean;
};

export type StorageVolume = {
  name: string;
  path?: string;
  type: string;
  format?: string;
  capacityBytes: number;
  allocationBytes: number;
};

export type StorageVolumeFormat = "qcow2" | "raw";

export type StorageVolumeCreateInput = {
  name: string;
  format: StorageVolumeFormat;
  capacityBytes: number;
};

export type StoragePoolList = {
  items: StoragePool[];
  total: number;
};

export type StorageVolumeList = {
  pool: string;
  items: StorageVolume[];
  total: number;
};

export type NetworkAddress = {
  family: string;
  address: string;
  prefix: number;
  dhcpEnabled?: boolean;
  dhcpStart?: string;
  dhcpEnd?: string;
  hosts?: NetworkDHCPHost[];
};

export type NetworkDHCPHost = {
  mac: string;
  ip: string;
  name?: string;
};

export type NetworkDHCPHostInput = {
  mac: string;
  ip: string;
  name?: string;
};

export type VirtualNetwork = {
  name: string;
  uuid?: string;
  bridge?: string;
  active: boolean;
  persistent: boolean;
  autostart: boolean;
  forwardMode?: string;
  addresses: NetworkAddress[];
};

export type VirtualNetworkList = {
  items: VirtualNetwork[];
  total: number;
};

export type DHCPLease = {
  interface?: string;
  expiresAt?: string;
  family: string;
  mac: string;
  ipAddress: string;
  prefix: number;
  hostname?: string;
  clientId?: string;
  iaid?: string;
};

export type DHCPLeaseList = {
  network: string;
  items: DHCPLease[];
  total: number;
};

export type VirtualNetworkAction = "start" | "stop";

export type VirtualNetworkCreateInput = {
  name: string;
  mode: "nat" | "isolated";
  subnet: string;
  dhcpStart?: string;
  dhcpEnd?: string;
  autostart: boolean;
  start: boolean;
};

export type VirtualNetworkUpdateInput = {
  mode: "nat" | "isolated";
  subnet: string;
  dhcpStart?: string;
  dhcpEnd?: string;
  autostart: boolean;
};

export type AuditResult = "success" | "failure";

export type AuditEvent = {
  id: number;
  occurredAt: string;
  requestId: string;
  actor: string;
  role: Role;
  action: string;
  targetKind: string;
  targetId: string;
  sourceIp: string;
  result: AuditResult;
  detail?: Record<string, unknown>;
};

export type AuditEventList = {
  items: AuditEvent[];
  total: number;
  page: number;
  pageSize: number;
};

export type AuditEventFilters = {
  actor?: string;
  action?: string;
  targetKind?: string;
  result?: AuditResult;
  page?: number;
  pageSize?: number;
};

export type TaskStatus = "queued" | "running" | "succeeded" | "failed" | "interrupted";

export type Task = {
  id: string;
  kind: string;
  targetKind: string;
  targetId: string;
  status: TaskStatus;
  progress: number;
  step: string;
  error?: string;
  actor: string;
  createdAt: string;
  updatedAt: string;
};

export type TaskList = {
  items: Task[];
  total: number;
  page: number;
  pageSize: number;
};

export type TaskFilters = {
  status?: TaskStatus;
  page?: number;
  pageSize?: number;
};

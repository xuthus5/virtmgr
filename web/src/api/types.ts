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
  auditRetentionDays: number;
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

export type ChangePasswordInput = {
  currentPassword: string;
  newPassword: string;
};

export type UserSession = {
  id: string;
  createdAt: string;
  expiresAt: string;
  current: boolean;
};

export type UserSessionList = {
  items: UserSession[];
  total: number;
};

export type APITokenScope = "read" | "write";

export type APIToken = {
  id: string;
  name: string;
  scopes: APITokenScope[];
  createdAt: string;
  expiresAt?: string;
  lastUsedAt?: string;
  revokedAt?: string;
  expired: boolean;
};

export type APITokenCreated = {
  token: string;
  apiToken: APIToken;
};

export type APITokenList = {
  items: APIToken[];
  total: number;
};

export type APITokenCreateInput = {
  name: string;
  scopes: APITokenScope[];
  expiresInDays: number;
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
  qos?: DiskQoS;
};

export type DiskQoS = {
  totalBytesSec?: number;
  readBytesSec?: number;
  writeBytesSec?: number;
  totalIops?: number;
  readIops?: number;
  writeIops?: number;
};

export type NetworkInterface = {
  type: string;
  mac?: string;
  source?: string;
  model?: string;
  address?: string;
  qos?: NetworkQoS;
};

export type NetworkQoS = {
  inboundAverageBytes?: number;
  inboundPeakBytes?: number;
  inboundBurstBytes?: number;
  outboundAverageBytes?: number;
  outboundPeakBytes?: number;
  outboundBurstBytes?: number;
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

export type VMNUMACell = {
  id: number;
  cpuset: string;
  memoryMiB: number;
};

export type VMCpuPin = {
  vcpu: number;
  cpuset: string;
};

export type VMPlacement = {
  numaCells: VMNUMACell[];
  hugePageSize?: string;
  vcpuPins: VMCpuPin[];
  emulatorPin?: string;
};

export type VMPlacementInput = VMPlacement;

export type VMDiskResizeInput = {
  capacityBytes: number;
};

export type VMDiskMigrateInput = {
  targetPool: string;
  targetName: string;
  targetFormat: "qcow2" | "raw";
};

export type VMDiskAttachInput = {
  pool: string;
  volume: string;
  target?: string;
  bus?: VMDiskBus;
};

export type VMISOAttachInput = {
  pool: string;
  volume: string;
};

export type VMNetworkAttachInput = {
  network: string;
  model?: VMNICModel;
};

export type VMFirmware = "bios" | "uefi";
export type VMDiskBus = "virtio" | "sata" | "scsi";
export type VMNICModel = "virtio" | "e1000";
export type VMBootOrder = "auto" | "disk-first" | "cdrom-first";

export type CloudInitInput = {
  userData: string;
  networkConfig?: string;
  vendorData?: string;
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
  architecture?: string;
  machine?: string;
  vcpus: number;
  memoryMiB: number;
  diskPath: string;
  diskFormat: string;
  imageId?: number;
  imageCloneMode?: VMImageCloneMode;
  targetPool?: string;
  diskBus?: VMDiskBus;
  network: string;
  nicModel?: VMNICModel;
  isoPath?: string;
  bootOrder?: VMBootOrder;
  firmware: VMFirmware;
  secureBoot: boolean;
  tpm?: boolean;
  cloudInit?: CloudInitInput;
  guestAgent?: boolean;
  placement?: VMPlacementInput;
  autostart: boolean;
  start: boolean;
};

export type VMImportInput = {
  name: string;
  architecture?: string;
  machine?: string;
  sourcePool: string;
  sourceVolume: string;
  targetPool: string;
  vcpus: number;
  memoryMiB: number;
  diskCapacityBytes?: number;
  diskBus: VMDiskBus;
  network: string;
  nicModel: VMNICModel;
  firmware: VMFirmware;
  secureBoot: boolean;
  tpm?: boolean;
  guestAgent: boolean;
  autostart: boolean;
  start: boolean;
};

export type VMTemplateInput = {
  name: string;
  description: string;
  vcpus: number;
  memoryMiB: number;
  diskFormat: string;
  imageId?: number;
  network: string;
  isoPath?: string;
  firmware: VMFirmware;
  secureBoot: boolean;
  cloudInit: boolean;
  cloudInitUserData?: string;
  cloudInitNetworkConfig?: string;
  cloudInitVendorData?: string;
  guestAgent?: boolean;
  autostart: boolean;
  start: boolean;
};

export type VMTemplate = VMTemplateInput & {
  id: number;
  createdAt: string;
  updatedAt: string;
};

export type VMImage = {
  id: number;
  name: string;
  description: string;
  pool: string;
  volume: string;
  format: "qcow2" | "raw";
  osFamily: string;
  osName: string;
  architecture: string;
  capacityBytes: number;
  allocationBytes: number;
  sha256?: string;
  createdAt: string;
  updatedAt: string;
};

export type VMImageCloneMode = "full" | "linked";

export type VMImageInput = {
  name: string;
  description: string;
  pool: string;
  volume: string;
  osFamily: string;
  osName: string;
  architecture: string;
};

export type VMImageList = {
  items: VMImage[];
  total: number;
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
  online?: boolean;
  quiesce?: boolean;
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
  online: boolean;
  quiesced: boolean;
  consistent: boolean;
  containsTpm: boolean;
  tpmStateSaved: boolean;
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
  online: boolean;
  quiesced: boolean;
  consistent: boolean;
  containsTpm: boolean;
  tpmStateSaved: boolean;
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
  online?: boolean;
  quiesce?: boolean;
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
  scheduleMode: ScheduleMode;
  dailyTime: string;
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
  scheduleMode: ScheduleMode;
  dailyTime: string;
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
  bootDevices?: VMBootDevice[];
  cpuModel?: string;
  cpuSockets?: number;
  cpuCores?: number;
  cpuThreads?: number;
  firmware: VMFirmware;
  secureBoot: boolean;
  tpm: boolean;
  cloudInit: boolean;
  disks: Disk[];
  interfaces: NetworkInterface[];
  graphics: Graphics[];
  serials?: SerialConsole[];
  passthrough: PassthroughDevice[];
  ipAddresses: string[];
  hostname?: string;
  agentConnected: boolean;
  agentConfigured?: boolean;
  agentState?: GuestAgentState;
  allowedActions: VMAction[];
  metadata?: VMMetadata;
  placement?: VMPlacement;
};

export type VMBootDevice = "hd" | "cdrom" | "network";

export type VMBootInput = {
  devices: VMBootDevice[];
};

export type VMCPUUpdateInput = {
  model: string;
  sockets: number;
  cores: number;
  threads: number;
};

export type VMDiskQoSInput = {
  totalBytesSec: number;
  readBytesSec: number;
  writeBytesSec: number;
  totalIops: number;
  readIops: number;
  writeIops: number;
};

export type VMNetworkQoSInput = {
  inboundAverageBytes: number;
  inboundPeakBytes: number;
  inboundBurstBytes: number;
  outboundAverageBytes: number;
  outboundPeakBytes: number;
  outboundBurstBytes: number;
};

export type VMNetworkModelInput = {
  model: VMNICModel;
};

export type VMMetadata = {
  vmUuid: string;
  tags: string[];
  note: string;
  owner: string;
  folder: string;
  resourceGroup: string;
  updatedBy: string;
  updatedAt: string;
};

export type VMMetadataInput = {
  tags: string[];
  note: string;
  owner: string;
  folder: string;
  resourceGroup: string;
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
  | "import"
  | "backup_restore"
  | "rollback"
  | "pre_rollback"
  | "disk_resize"
  | "iso_eject"
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
  quiesced?: boolean;
};

export type VMSnapshotCreateInput = {
  name: string;
  description?: string;
  quiesce?: boolean;
};

export type GuestAgentState =
  | "unknown"
  | "not_configured"
  | "disconnected"
  | "connected"
  | "unresponsive"
  | "error";

export type GuestAgentOSInfo = {
  id?: string;
  name?: string;
  prettyName?: string;
  version?: string;
  kernelRelease?: string;
  arch?: string;
  hostname?: string;
};

export type GuestAgentIPAddress = {
  address: string;
  family: "ipv4" | "ipv6";
  prefix?: number;
};

export type GuestAgentNetworkInterface = {
  name: string;
  mac?: string;
  addresses: GuestAgentIPAddress[];
};

export type GuestAgentFilesystem = {
  device?: string;
  mountPoint: string;
  type?: string;
  totalBytes?: number;
  usedBytes?: number;
  freeBytes?: number;
};

export type GuestAgentCapabilities = {
  hostname: boolean;
  network: boolean;
  osInfo: boolean;
  fsInfo: boolean;
  shutdown: boolean;
  reboot: boolean;
  timeSync: boolean;
  trim: boolean;
  quiesce: boolean;
  fileAccess: boolean;
  passwordReset: boolean;
  sshKeys: boolean;
  processes: boolean;
  users: boolean;
  timezone: boolean;
  networkRoutes: boolean;
  filesystemResize?: boolean;
  packages: boolean;
};

export type GuestAgentPackage = {
  name: string;
  version: string;
  architecture?: string;
  summary?: string;
  upgradable: boolean;
  updateVersion?: string;
  securityUpdate: boolean;
};

export type GuestAgentPackageInventory = {
  manager: string;
  items: GuestAgentPackage[];
  total: number;
  upgradable: number;
  securityUpdates: number;
  truncated: boolean;
  warnings: string[];
  checkedAt: string;
};

export type GuestAgentRoute = {
  destination?: string;
  gateway?: string;
  interface?: string;
  family?: string;
  metric?: number;
};

export type GuestAgentInfo = {
  state: GuestAgentState;
  connected: boolean;
  channelConfigured: boolean;
  version?: string;
  hostname?: string;
  lastSeenAt?: string;
  lastError?: string;
  stale: boolean;
  os: GuestAgentOSInfo;
  interfaces: GuestAgentNetworkInterface[];
  filesystems: GuestAgentFilesystem[];
  timezone?: string;
  routes: GuestAgentRoute[];
  capabilities: GuestAgentCapabilities;
  diagnosticCode?: string;
  diagnosticHint?: string;
  probedAt: string;
};

export type GuestAgentCloudInit = {
  installed: boolean;
  state: "done" | "running" | "error" | "disabled" | "not_run" | "unknown";
  status?: string;
  extendedStatus?: string;
  stage?: string;
  datasource?: string;
  lastUpdate?: string;
  errors: string[];
  recoverableErrors: string[];
  log?: string;
  logPath?: string;
  logTruncated: boolean;
  checkedAt: string;
};

export type GuestAgentAction = "shutdown" | "reboot" | "sync-time" | "trim";

export type GuestAgentActionInput = {
  action: GuestAgentAction;
  mount?: string;
  minimumBytes?: number;
};

export type GuestAgentFilesystemResizeInput = {
  mountPoint: string;
};

export type GuestAgentFilesystemResizeResult = {
  mountPoint: string;
  device?: string;
  filesystem: string;
  previousTotalBytes: number;
  totalBytes: number;
  freeBytes: number;
};

export type GuestAgentFilesystemResizeResponse = {
  task: Task;
  result: GuestAgentFilesystemResizeResult;
};

export type GuestAgentServiceLog = {
  unit: string;
  content: string;
  truncated: boolean;
};

export type GuestAgentDiagnostics = {
  checkedAt: string;
  failedServices: GuestAgentService[];
  serviceLogs: GuestAgentServiceLog[];
  kernelWarnings: string[];
  systemErrors: string[];
  oomEvents: string[];
  recommendations: string[];
  commandErrors: string[];
};

export type GuestAgentFileContent = {
  path: string;
  dataBase64: string;
  offset: number;
  size: number;
  eof: boolean;
  sha256?: string;
};

export type GuestAgentFileWriteInput = {
  path: string;
  dataBase64: string;
  mode?: "replace" | "append";
  overwrite?: boolean;
};

export type GuestAgentFileWriteResponse = {
  task: Task;
  result: { path: string; bytesWritten: number; sha256?: string };
};

export type GuestAgentPasswordInput = { username: string; password: string };
export type GuestAgentSSHKeyInput = { username: string; keys: string[] };
export type GuestAgentAuthorizedKeys = { username: string; keys: string[] };

export type GuestAgentProcess = {
  pid: number;
  ppid?: number;
  user?: string;
  state?: string;
  command: string;
  args?: string;
};

export type GuestAgentProcessList = { items: GuestAgentProcess[]; total: number };
export type GuestAgentService = {
  unit: string;
  load: string;
  active: string;
  sub: string;
  description?: string;
};
export type GuestAgentServiceList = { items: GuestAgentService[]; total: number };
export type GuestAgentProcessSignalInput = { pid: number; signal: "term" | "kill" };
export type GuestAgentUser = { username: string; domain?: string; loginTime?: number };
export type GuestAgentUserList = { items: GuestAgentUser[]; total: number };
export type GuestAgentPolicy = {
  vmUuid: string;
  expected: boolean;
  lastState?: GuestAgentState;
  lastError?: string;
  consecutiveFailures: number;
  lastCheckedAt?: string;
  lastHealthyAt?: string;
  updatedBy?: string;
  updatedAt: string;
};
export type GuestAgentPolicyInput = { expected: boolean };
export type GuestAgentHealthSample = {
  id: number;
  vmUuid: string;
  sampledAt: string;
  state: GuestAgentState;
  latencyMs: number;
  connected: boolean;
  error?: string;
};
export type GuestAgentHealthHistory = {
  items: GuestAgentHealthSample[];
  total: number;
  connectivityPercent: number;
};

export type VMSnapshotList = {
  items: VMSnapshot[];
  total: number;
};

export type ScheduleMode = "interval" | "daily";

export type SnapshotPolicyInput = {
  name: string;
  intervalMinutes: number;
  scheduleMode: ScheduleMode;
  dailyTime: string;
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
  architecture: string;
  architectures?: string[];
  machineTypes?: string[];
  virtualization?: string;
  uefi: boolean;
  secureBoot: boolean;
  tpm: boolean;
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
  diskReadBytes: number;
  diskWriteBytes: number;
  diskReadOperations: number;
  diskWriteOperations: number;
  diskReadBytesPerSecond: number;
  diskWriteBytesPerSecond: number;
  diskReadOperationsPerSecond: number;
  diskWriteOperationsPerSecond: number;
  interfaces?: NetworkInterfaceMetrics[];
};

export type NetworkInterfaceMetrics = {
  name: string;
  rxBytes: number;
  txBytes: number;
  rxBytesPerSecond: number;
  txBytesPerSecond: number;
};

export type HostMetrics = ResourceMetrics & {
  hostName: string;
};

export type VMMetrics = ResourceMetrics & {
  uuid: string;
};

export type MetricSample = {
  id: number;
  scope: "host" | "vm";
  targetId: string;
  sampledAt: string;
  cpuPercent: number;
  memoryUsedBytes: number;
  memoryTotalBytes: number;
  diskReadBytesPerSecond: number;
  diskWriteBytesPerSecond: number;
  diskReadIops: number;
  diskWriteIops: number;
  networkRxBytesPerSecond: number;
  networkTxBytesPerSecond: number;
};

export type MetricSampleList = {
  items: MetricSample[];
  total: number;
};

export type MetricHistoryFilters = {
  scope: "host" | "vm";
  targetId: string;
  from?: string;
  to?: string;
  limit?: number;
};

export type GuestFilesystemSample = {
  id: number;
  vmUuid: string;
  vmName: string;
  mountPoint: string;
  usedBytes: number;
  totalBytes: number;
  sampledAt: string;
};

export type GuestFilesystemSampleList = {
  items: GuestFilesystemSample[];
  total: number;
};

export type CapacityForecast = {
  scope: "storage_pool";
  targetId: string;
  usedBytes: number;
  totalBytes: number;
  usagePercent: number;
  growthBytesPerDay: number;
  daysToFull?: number;
  forecastAt?: string;
  sampleCount: number;
  windowDays: number;
};

export type CapacityForecastList = {
  items: CapacityForecast[];
  total: number;
  generatedAt: string;
};

export type AlertMetric = "host_cpu_percent" | "host_memory_percent" | "vm_cpu_percent" | "vm_memory_percent" | "storage_pool_percent" | "guest_filesystem_percent";
export type AlertOperator = "gt" | "gte" | "lt" | "lte";
export type AlertState = "ok" | "pending" | "firing";

export type AlertRule = {
  id: number;
  name: string;
  metric: AlertMetric;
  operator: AlertOperator;
  threshold: number;
  durationMinutes: number;
  enabled: boolean;
  webhookUrl: string;
  lastState: AlertState;
  lastChangedAt?: string;
  lastNotifiedAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type AlertRuleInput = Omit<AlertRule, "id" | "lastState" | "lastChangedAt" | "lastNotifiedAt" | "createdAt" | "updatedAt">;

export type AlertRuleList = {
  items: AlertRule[];
  total: number;
};

export type SystemInfo = {
  build: {
    version: string;
    commit: string;
    date: string;
  };
  tls: {
    enabled: boolean;
    subject?: string;
    issuer?: string;
    notBefore?: string;
    notAfter?: string;
    dnsNames?: string[];
    ipAddresses?: string[];
    requestHost?: string;
    requestHostMatched: boolean;
    expiresInDays?: number;
    keyFileMode?: string;
    lastReloadedAt?: string;
    reloadError?: string;
    warnings?: string[];
  };
};

export type DatabaseRestoreStatus = {
  state: "none" | "pending_restart" | "applied" | "failed";
  filename?: string;
  sizeBytes?: number;
  schemaVersion?: number;
  uploadedAt?: string;
  appliedAt?: string;
  message?: string;
};

export type DataPlaneOperation = {
  id: string;
  kind: string;
  vmUuid?: string;
  vmName?: string;
  taskId?: string;
  backupId?: string;
  phase: string;
  online: boolean;
  sourcePool?: string;
  sourceVolume?: string;
  sourcePath?: string;
  targetPool?: string;
  targetName?: string;
  targetPath?: string;
  status: "running" | "succeeded" | "failed" | "recovered" | "needs_attention";
  error?: string;
  createdAt: string;
  updatedAt: string;
};

export type DataPlaneOperationList = {
  items: DataPlaneOperation[];
  total: number;
};

export type DataPlaneRecovery = {
  status: "completed" | "rolled_back" | "cleaned" | "noop" | "needs_attention";
  message: string;
  recoveredAt?: string;
};

export type DataPlaneOperationResult = {
  operation: DataPlaneOperation;
  recovery: DataPlaneRecovery;
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
  type: "dir" | "netfs" | "logical";
  targetPath: string;
  sourceHost?: string;
  sourcePath?: string;
  volumeGroup?: string;
  autostart: boolean;
  start: boolean;
};

export type StorageVolume = {
  name: string;
  path?: string;
  type: string;
  format?: string;
  linked?: boolean;
  capacityBytes: number;
  allocationBytes: number;
  usedBy?: StorageVolumeUsage[];
};

export type StorageVolumeUsage = {
  vmUuid: string;
  vmName: string;
  target: string;
  readOnly: boolean;
};

export type StorageVolumeFormat = "qcow2" | "raw";

export type StorageVolumeCreateInput = {
  name: string;
  format: StorageVolumeFormat;
  capacityBytes: number;
};

export type StorageVolumeConvertInput = {
  targetPool: string;
  targetName: string;
  targetFormat: StorageVolumeFormat;
};

export type StorageVolumeFlattenInput = {
  targetPool: string;
  targetName: string;
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
  ipv6Mode?: "slaac" | "dhcpv6";
  dhcpEnabled?: boolean;
  dhcpStart?: string;
  dhcpEnd?: string;
  hosts?: NetworkDHCPHost[];
};

export type NetworkDHCPHost = {
  mac?: string;
  id?: string;
  ip: string;
  name?: string;
};

export type NetworkDHCPHostInput = {
  mac?: string;
  id?: string;
  ip: string;
  name?: string;
};

export type NetworkDHCPHostImportResult = {
  network: VirtualNetwork;
  imported: number;
  skipped: number;
};

export type NetworkDNSHost = {
  ip: string;
  hostnames: string[];
};

export type NetworkDNSHostInput = {
  ip: string;
  hostnames: string[];
};

export type VirtualNetwork = {
  name: string;
  uuid?: string;
  bridge?: string;
  active: boolean;
  persistent: boolean;
  autostart: boolean;
  forwardMode?: string;
  natIpv6?: boolean;
  mtu?: number;
  dnsDomain?: string;
  dnsServers?: string[];
  dnsHosts?: NetworkDNSHost[];
  addresses: NetworkAddress[];
  warnings?: string[];
};

export type VirtualNetworkList = {
  items: VirtualNetwork[];
  total: number;
};

export type HostBridge = {
  name: string;
};

export type HostBridgeList = {
  items: HostBridge[];
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

export type NetworkLeaseReservationInput = {
  ip: string;
  name?: string;
};

export type NetworkCheckStatus = "ok" | "warning" | "error";

export type NetworkCheck = {
  id: string;
  status: NetworkCheckStatus;
  title: string;
  detail: string;
};

export type NetworkDiagnostics = {
  network: string;
  checkedAt: string;
  active: boolean;
  bridge?: string;
  bridgeReady: boolean;
  mtu?: number;
  bridgeMtu?: number;
  ipv4Forwarding: boolean;
  ipv6Forwarding: boolean;
  ipv6Configured: boolean;
  natIpv6: boolean;
  nat66Firewall: boolean;
  firewallBackend?: string;
  portForwardRules: number;
  portForwardReady: boolean;
  portForwardError?: string;
  dhcpv6Range: boolean;
  dhcpv6Service: boolean;
  raService: boolean;
  dnsService: boolean;
  dnsResolution: boolean;
  dnsHostCount: number;
  gatewayReachable: boolean;
  outboundReachable: boolean;
  runtimeDrift: boolean;
  driftDetails?: string[];
  libvirtVersion?: string;
  dnsmasqVersion?: string;
  leaseCount: number;
  checks: NetworkCheck[];
};

export type PortForwardProtocol = "tcp" | "udp";

export type PortForward = {
  id: string;
  network: string;
  name: string;
  family: "ipv4" | "ipv6";
  protocol: PortForwardProtocol;
  hostAddress?: string;
  hostPort: number;
  guestAddress: string;
  guestPort: number;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
};

export type PortForwardInput = {
  name: string;
  family: "ipv4" | "ipv6";
  protocol: PortForwardProtocol;
  hostAddress?: string;
  hostPort: number;
  guestAddress: string;
  guestPort: number;
  enabled: boolean;
};

export type PortForwardList = {
  network: string;
  items: PortForward[];
  total: number;
};

export type VirtualNetworkAction = "start" | "stop";

export type VirtualNetworkCreateInput = {
  name: string;
  mode: "nat" | "isolated" | "bridge";
  subnet?: string;
  dhcpStart?: string;
  dhcpEnd?: string;
  ipv6Subnet?: string;
  ipv6Mode?: "slaac" | "dhcpv6";
  ipv6DhcpStart?: string;
  ipv6DhcpEnd?: string;
  natIpv6?: boolean;
  dnsDomain?: string;
  dnsServers?: string[];
  bridge?: string;
  mtu?: number;
  autostart: boolean;
  start: boolean;
};

export type VirtualNetworkUpdateInput = {
  mode: "nat" | "isolated" | "bridge";
  subnet?: string;
  dhcpStart?: string;
  dhcpEnd?: string;
  ipv6Subnet?: string;
  ipv6Mode?: "slaac" | "dhcpv6";
  ipv6DhcpStart?: string;
  ipv6DhcpEnd?: string;
  natIpv6?: boolean;
  dnsDomain?: string;
  dnsServers?: string[];
  bridge?: string;
  mtu?: number;
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
  targetId?: string;
  result?: AuditResult;
  from?: string;
  to?: string;
  page?: number;
  pageSize?: number;
};

export type AuditExportFormat = "json" | "csv";

export type TaskStatus = "queued" | "running" | "succeeded" | "failed" | "interrupted" | "cancelled";

export type Task = {
  id: string;
  kind: string;
  targetKind: string;
  targetId: string;
  status: TaskStatus;
  progress: number;
  step: string;
  error?: string;
  metadata?: Record<string, unknown>;
  actor: string;
  createdAt: string;
  updatedAt: string;
  cancellable: boolean;
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

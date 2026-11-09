import type { AuditEventFilters, AuditEventList, Backup, BackupCreateInput, BackupFilters, BackupList, BackupPinInput, BackupRepository, BackupRepositoryInput, BackupRepositoryList, BackupSchedule, BackupScheduleInput, BackupScheduleList, BackupSummary, ConsoleTicket, ConsoleTokenInput, DHCPLeaseList, HostDetails, HostDeviceList, HostMetrics, NetworkDHCPHostInput, Overview, SendConsoleKeyInput, Session, Settings, SettingsUpdateInput, SnapshotPolicy, SnapshotPolicyInput, SnapshotPolicyList, SnapshotPolicyRunResult, StoragePool, StoragePoolAction, StoragePoolCreateInput, StoragePoolList, StorageVolume, StorageVolumeCreateInput, StorageVolumeList, Task, TaskFilters, TaskList, User, UserCreateInput, UserList, UserUpdateInput, VirtualNetwork, VirtualNetworkAction, VirtualNetworkCreateInput, VirtualNetworkList, VirtualNetworkUpdateInput, VMAction, VMAutostartInput, VMBackupInput, VMBatchActionInput, VMBatchActionResult, VMCloneInput, VMConfigVersion, VMConfigVersionDetail, VMConfigVersionList, VMConfigVersionProtectInput, VMConfigVersionRollbackInput, VMCreateInput, VMDeviceInput, VMDiskResizeInput, VMExport, VMResourcesInput, VMRestoreInput, VM, VMMetrics, VMSnapshot, VMSnapshotCreateInput, VMSnapshotList, VMSummaryList, VMState, VMTemplate, VMTemplateInput, VMTemplateList, VMXML, VMXMLFormat, VMXMLUpdateInput } from "@/api/types";

let csrfToken = "";

export class ApiError extends Error {
  status: number;
  code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  const method = (init?.method ?? "GET").toUpperCase();
  if (init?.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  if (method !== "GET" && csrfToken && !headers.has("X-CSRF-Token")) headers.set("X-CSRF-Token", csrfToken);
  const response = await fetch(path, { ...init, headers, credentials: "same-origin" });
  if (!response.ok) {
    let code = "HTTP_ERROR";
    let message = `请求失败 (${response.status})`;
    try {
      const body = await response.json() as { error?: { code?: string; message?: string } };
      code = body.error?.code ?? code;
      message = body.error?.message ?? message;
    } catch {
      // Keep the status-based fallback when the response is not JSON.
    }
    throw new ApiError(response.status, code, message);
  }
  if (response.status === 204) {
    if (path === "/api/v1/session") csrfToken = "";
    return undefined as T;
  }
  const payload = await response.json() as unknown;
  if (path === "/api/v1/session" && payload && typeof payload === "object" && "csrfToken" in payload) {
    const nextToken = (payload as { csrfToken?: unknown }).csrfToken;
    if (typeof nextToken === "string") csrfToken = nextToken;
  }
  return payload as T;
}

export const api = {
  session: () => request<Session>("/api/v1/session"),
  login: (username: string, password: string) => request<Session>("/api/v1/session", {
    method: "POST",
    body: JSON.stringify({ username, password }),
  }),
  logout: () => request<void>("/api/v1/session", { method: "DELETE" }),
  overview: () => request<Overview>("/api/v1/overview"),
  host: () => request<HostDetails>("/api/v1/host"),
  hostMetrics: () => request<HostMetrics>("/api/v1/host/metrics"),
  hostDevices: () => request<HostDeviceList>("/api/v1/host/devices"),
  storagePools: () => request<StoragePoolList>("/api/v1/storage/pools"),
  storagePool: (name: string) => request<StoragePool>(`/api/v1/storage/pools/${encodeURIComponent(name)}`),
  createStoragePool: (input: StoragePoolCreateInput) => request<StoragePool>("/api/v1/storage/pools", {
    method: "POST",
    body: JSON.stringify(input),
  }),
  storagePoolAction: (name: string, action: StoragePoolAction) => request<void>(
    `/api/v1/storage/pools/${encodeURIComponent(name)}/actions/${encodeURIComponent(action)}`,
    { method: "POST" },
  ),
  deleteStoragePool: (name: string) => request<void>(`/api/v1/storage/pools/${encodeURIComponent(name)}`, { method: "DELETE" }),
  storageVolumes: (pool: string) => request<StorageVolumeList>(`/api/v1/storage/pools/${encodeURIComponent(pool)}/volumes`),
  createStorageVolume: (pool: string, input: StorageVolumeCreateInput) => request<StorageVolume>(
    `/api/v1/storage/pools/${encodeURIComponent(pool)}/volumes`,
    { method: "POST", body: JSON.stringify(input) },
  ),
  uploadStorageVolume: (pool: string, file: File) => request<StorageVolume>(
    `/api/v1/storage/pools/${encodeURIComponent(pool)}/volumes/upload?name=${encodeURIComponent(file.name)}`,
    { method: "POST", headers: { "Content-Type": "application/octet-stream" }, body: file },
  ),
  deleteStorageVolume: (pool: string, name: string) => request<void>(
    `/api/v1/storage/pools/${encodeURIComponent(pool)}/volumes/${encodeURIComponent(name)}`,
    { method: "DELETE" },
  ),
  networks: () => request<VirtualNetworkList>("/api/v1/networks"),
  networkLeases: (name: string) => request<DHCPLeaseList>(`/api/v1/networks/${encodeURIComponent(name)}/leases`),
  createNetwork: (input: VirtualNetworkCreateInput) => request<VirtualNetwork>("/api/v1/networks", {
    method: "POST",
    body: JSON.stringify(input),
  }),
  updateNetwork: (name: string, input: VirtualNetworkUpdateInput) => request<VirtualNetwork>(
    `/api/v1/networks/${encodeURIComponent(name)}`,
    { method: "PATCH", body: JSON.stringify(input) },
  ),
  addNetworkDHCPHost: (name: string, input: NetworkDHCPHostInput) => request<VirtualNetwork>(
    `/api/v1/networks/${encodeURIComponent(name)}/hosts`,
    { method: "POST", body: JSON.stringify(input) },
  ),
  deleteNetworkDHCPHost: (name: string, mac: string) => request<VirtualNetwork>(
    `/api/v1/networks/${encodeURIComponent(name)}/hosts/${encodeURIComponent(mac)}`,
    { method: "DELETE" },
  ),
  networkAction: (name: string, action: VirtualNetworkAction) => request<void>(
    `/api/v1/networks/${encodeURIComponent(name)}/actions/${encodeURIComponent(action)}`,
    { method: "POST" },
  ),
  deleteNetwork: (name: string) => request<void>(`/api/v1/networks/${encodeURIComponent(name)}`, { method: "DELETE" }),
  vms: (query = "", state = "") => {
    const params = new URLSearchParams();
    if (query) params.set("q", query);
    if (state) params.set("state", state as VMState);
    const suffix = params.toString();
    return request<VMSummaryList>(`/api/v1/vms${suffix ? `?${suffix}` : ""}`);
  },
  vm: (uuid: string) => request<VM>(`/api/v1/vms/${encodeURIComponent(uuid)}`),
  vmMetrics: (uuid: string) => request<VMMetrics>(`/api/v1/vms/${encodeURIComponent(uuid)}/metrics`),
  vmXML: (uuid: string, format: VMXMLFormat) => request<VMXML>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/xml?format=${encodeURIComponent(format)}`,
  ),
  updateVMXML: (uuid: string, input: VMXMLUpdateInput) => request<VMXML>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/xml`,
    { method: "PUT", body: JSON.stringify(input) },
  ),
  vmConfigVersions: (uuid: string) => request<VMConfigVersionList>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/config-versions`,
  ),
  vmConfigVersion: (uuid: string, versionNo: number) => request<VMConfigVersionDetail>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/config-versions/${versionNo}`,
  ),
  protectVMConfigVersion: (uuid: string, versionNo: number, input: VMConfigVersionProtectInput) => request<VMConfigVersion>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/config-versions/${versionNo}`,
    { method: "PATCH", body: JSON.stringify(input) },
  ),
  rollbackVMConfigVersion: (uuid: string, versionNo: number, input: VMConfigVersionRollbackInput) => request<VMConfigVersionDetail>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/config-versions/${versionNo}/rollback`,
    { method: "POST", body: JSON.stringify(input) },
  ),
  exportVM: (uuid: string) => request<VMExport>(`/api/v1/vms/${encodeURIComponent(uuid)}/export`),
  vmSnapshots: (uuid: string) => request<VMSnapshotList>(`/api/v1/vms/${encodeURIComponent(uuid)}/snapshots`),
  createVMSnapshot: (uuid: string, input: VMSnapshotCreateInput) => request<VMSnapshot>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/snapshots`,
    { method: "POST", body: JSON.stringify(input) },
  ),
  revertVMSnapshot: (uuid: string, name: string) => request<void>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/snapshots/${encodeURIComponent(name)}/revert`,
    { method: "POST" },
  ),
  deleteVMSnapshot: (uuid: string, name: string) => request<void>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/snapshots/${encodeURIComponent(name)}`,
    { method: "DELETE" },
  ),
  snapshotPolicies: (uuid: string) => request<SnapshotPolicyList>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/snapshot-policies`,
  ),
  createSnapshotPolicy: (uuid: string, input: SnapshotPolicyInput) => request<SnapshotPolicy>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/snapshot-policies`,
    { method: "POST", body: JSON.stringify(input) },
  ),
  updateSnapshotPolicy: (id: number, input: SnapshotPolicyInput) => request<SnapshotPolicy>(
    `/api/v1/snapshot-policies/${id}`,
    { method: "PATCH", body: JSON.stringify(input) },
  ),
  deleteSnapshotPolicy: (id: number) => request<void>(`/api/v1/snapshot-policies/${id}`, { method: "DELETE" }),
  runSnapshotPolicy: (id: number) => request<SnapshotPolicyRunResult>(
    `/api/v1/snapshot-policies/${id}/run`,
    { method: "POST" },
  ),
  issueConsoleToken: (uuid: string, input: ConsoleTokenInput = { kind: "vnc" }) => request<ConsoleTicket>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/console-token`,
    { method: "POST", body: JSON.stringify(input) },
  ),
  sendConsoleKey: (uuid: string, input: SendConsoleKeyInput) => request<void>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/console/send-key`,
    { method: "POST", body: JSON.stringify(input) },
  ),
  createVM: (input: VMCreateInput) => request<VM>("/api/v1/vms", {
    method: "POST",
    body: JSON.stringify(input),
  }),
  cloneVM: (uuid: string, input: VMCloneInput) => request<Task>(`/api/v1/vms/${encodeURIComponent(uuid)}/clone`, {
    method: "POST",
    body: JSON.stringify(input),
  }),
  backupVM: (uuid: string, input: VMBackupInput) => request<Task>(`/api/v1/vms/${encodeURIComponent(uuid)}/backups`, {
    method: "POST",
    body: JSON.stringify(input),
  }),
  backups: (filters: BackupFilters = {}) => {
    const params = new URLSearchParams();
    if (filters.vmUuid) params.set("vmUuid", filters.vmUuid);
    if (filters.status) params.set("status", filters.status);
    if (filters.repositoryId) params.set("repositoryId", filters.repositoryId);
    if (filters.page) params.set("page", String(filters.page));
    if (filters.pageSize) params.set("pageSize", String(filters.pageSize));
    const suffix = params.toString();
    return request<BackupList>(`/api/v1/backups${suffix ? `?${suffix}` : ""}`);
  },
  backup: (id: string) => request<Backup>(`/api/v1/backups/${encodeURIComponent(id)}`),
  backupSummary: (vmUuid = "") => request<BackupSummary>(`/api/v1/backups/summary${vmUuid ? `?vmUuid=${encodeURIComponent(vmUuid)}` : ""}`),
  createBackup: (input: BackupCreateInput) => request<Task>("/api/v1/backups", {
    method: "POST",
    body: JSON.stringify(input),
  }),
  verifyBackup: (id: string, deep = false) => request<Task>(`/api/v1/backups/${encodeURIComponent(id)}/verify`, {
    method: "POST",
    body: JSON.stringify({ deep }),
  }),
  restoreBackup: (id: string, input: VMRestoreInput) => request<Task>(`/api/v1/backups/${encodeURIComponent(id)}/restore`, {
    method: "POST",
    body: JSON.stringify(input),
  }),
  deleteBackup: (id: string) => request<Task>(`/api/v1/backups/${encodeURIComponent(id)}`, { method: "DELETE" }),
  pinBackup: (id: string, input: BackupPinInput) => request<Backup>(`/api/v1/backups/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  }),
  backupRepositories: () => request<BackupRepositoryList>("/api/v1/backup-repositories"),
  createBackupRepository: (input: BackupRepositoryInput) => request<BackupRepository>("/api/v1/backup-repositories", {
    method: "POST",
    body: JSON.stringify(input),
  }),
  updateBackupRepository: (id: string, input: BackupRepositoryInput) => request<BackupRepository>(`/api/v1/backup-repositories/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  }),
  deleteBackupRepository: (id: string) => request<void>(`/api/v1/backup-repositories/${encodeURIComponent(id)}`, { method: "DELETE" }),
  backupSchedules: (vmUuid = "") => request<BackupScheduleList>(`/api/v1/backup-schedules${vmUuid ? `?vmUuid=${encodeURIComponent(vmUuid)}` : ""}`),
  createBackupSchedule: (input: BackupScheduleInput) => request<BackupSchedule>("/api/v1/backup-schedules", {
    method: "POST",
    body: JSON.stringify(input),
  }),
  updateBackupSchedule: (id: number, input: Omit<BackupScheduleInput, "vmUuid">) => request<BackupSchedule>(`/api/v1/backup-schedules/${id}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  }),
  deleteBackupSchedule: (id: number) => request<void>(`/api/v1/backup-schedules/${id}`, { method: "DELETE" }),
  runBackupSchedule: (id: number) => request<Task>(`/api/v1/backup-schedules/${id}/run`, { method: "POST" }),
  deleteVM: (uuid: string) => request<void>(`/api/v1/vms/${encodeURIComponent(uuid)}`, { method: "DELETE" }),
  setVMAutostart: (uuid: string, input: VMAutostartInput) => request<VM>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/autostart`,
    { method: "PATCH", body: JSON.stringify(input) },
  ),
  updateVMResources: (uuid: string, input: VMResourcesInput) => request<VM>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/resources`,
    { method: "PATCH", body: JSON.stringify(input) },
  ),
  resizeVMDisk: (uuid: string, target: string, input: VMDiskResizeInput) => request<VM>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/disks/${encodeURIComponent(target)}`,
    { method: "PATCH", body: JSON.stringify(input) },
  ),
  attachVMDevice: (uuid: string, input: VMDeviceInput) => request<VM>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/devices`,
    { method: "POST", body: JSON.stringify(input) },
  ),
  detachVMDevice: (uuid: string, device: string) => request<VM>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/devices/${encodeURIComponent(device)}`,
    { method: "DELETE" },
  ),
  vmAction: (uuid: string, action: VMAction) => request<VM>(
    `/api/v1/vms/${encodeURIComponent(uuid)}/actions/${encodeURIComponent(action)}`,
    { method: "POST" },
  ),
  batchVMAction: (input: VMBatchActionInput) => request<VMBatchActionResult>("/api/v1/vms/batch-actions", {
    method: "POST",
    body: JSON.stringify(input),
  }),
  templates: () => request<VMTemplateList>("/api/v1/templates"),
  template: (id: number) => request<VMTemplate>(`/api/v1/templates/${id}`),
  createTemplate: (input: VMTemplateInput) => request<VMTemplate>("/api/v1/templates", {
    method: "POST",
    body: JSON.stringify(input),
  }),
  updateTemplate: (id: number, input: VMTemplateInput) => request<VMTemplate>(`/api/v1/templates/${id}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  }),
  deleteTemplate: (id: number) => request<void>(`/api/v1/templates/${id}`, { method: "DELETE" }),
  tasks: (filters: TaskFilters = {}) => {
    const params = new URLSearchParams();
    if (filters.status) params.set("status", filters.status);
    if (filters.page) params.set("page", String(filters.page));
    if (filters.pageSize) params.set("pageSize", String(filters.pageSize));
    const suffix = params.toString();
    return request<TaskList>(`/api/v1/tasks${suffix ? `?${suffix}` : ""}`);
  },
  task: (id: string) => request<Task>(`/api/v1/tasks/${encodeURIComponent(id)}`),
  auditEvents: (filters: AuditEventFilters = {}) => {
    const params = new URLSearchParams();
    if (filters.actor) params.set("actor", filters.actor);
    if (filters.action) params.set("action", filters.action);
    if (filters.targetKind) params.set("targetKind", filters.targetKind);
    if (filters.result) params.set("result", filters.result);
    if (filters.page) params.set("page", String(filters.page));
    if (filters.pageSize) params.set("pageSize", String(filters.pageSize));
    const suffix = params.toString();
    return request<AuditEventList>(`/api/v1/audit-events${suffix ? `?${suffix}` : ""}`);
  },
  users: () => request<UserList>("/api/v1/users"),
  createUser: (input: UserCreateInput) => request<User>("/api/v1/users", {
    method: "POST",
    body: JSON.stringify(input),
  }),
  updateUser: (id: number, input: UserUpdateInput) => request<User>(`/api/v1/users/${id}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  }),
  settings: () => request<Settings>("/api/v1/settings"),
  updateSettings: (input: SettingsUpdateInput) => request<Settings>("/api/v1/settings", {
    method: "PUT",
    body: JSON.stringify(input),
  }),
};

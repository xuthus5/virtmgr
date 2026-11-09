# VirtMgr 备份与恢复方案

> 状态：已批准并完成首期实现。
> 日期：2026-11-09。
> 目标：在不破坏 libvirt 单一事实来源的前提下，把现有单机冷备份扩展为可发现、可恢复、可调度、可审计的备份中心。

## 1. 当前实现与缺口

VirtMgr 已经有虚拟机详情页的“冷备份”能力：

- `POST /api/v1/vms/{uuid}/backups` 创建 `vm.backup` 异步任务。
- 仅支持关机或异常停止的持久化虚拟机。
- 把 inactive 持久配置 XML、`virtmgr.vm-backup/v1` manifest 和可写 `file` 磁盘复制到指定活跃 libvirt 存储池。
- 复制失败时会删除本次已创建的卷；成功和失败都写审计。
- 任务中心可查看进度，存储页可看到备份产生的卷。

当前缺口：

| 能力 | 当前状态 |
|---|---|
| 备份集集中查询 | 缺失，只能从任务中心或存储卷间接判断 |
| 备份集生命周期 | 缺失，不能从 VirtMgr 删除整组备份 |
| 恢复 | 缺失，manifest 只是备份元数据，没有恢复入口 |
| 完整性校验 | 仅记录配置 XML 的 SHA-256，没有统一校验流程 |
| 定时备份 | 缺失 |
| 保留策略 | 缺失 |
| 备份存储库 | 只能每次选择一个 libvirt 存储池，没有全局配置和容量提示 |
| 中断恢复 | 任务会被标记中断，但备份目录/卷需要人工清理 |
| 在线备份 | 不支持，运行中的虚拟机必须先关机 |

## 2. 建设目标

### 2.1 必须完成

1. 提供独立的“备份中心”，集中查看所有虚拟机备份。
2. 创建备份集时形成稳定的备份清单、校验信息和任务记录。
3. 支持把备份恢复为一台新虚拟机，而不是直接覆盖原虚拟机。
4. 支持备份删除、完整性校验和异常状态处理。
5. 支持按虚拟机配置定时备份、保留数量和过期清理。
6. 复用现有角色、全局只读模式、CSRF、任务中心、SSE 和审计体系。
7. 任何失败都不能留下“看似成功但无法恢复”的备份。

### 2.2 首期不做

- 不做跨宿主机复制、异地容灾或多节点备份调度。
- 不做在-place 覆盖恢复。首期只恢复为新虚拟机；覆盖恢复风险更高，后置。
- 不做块设备、iSCSI、Ceph RBD 等非 `file` 后端磁盘的完整备份。
- 不做备份文件浏览、单文件恢复或客户机文件级恢复。
- 不把备份内容暴露成可直接下载的浏览器链接；磁盘备份可能包含敏感数据。
- 不在首期承诺“运行中强一致在线备份”。在线/增量备份作为后续阶段。

## 3. 推荐方案

### 3.1 备份模式

首期采用“冷全量备份”：

- 仅允许 `shutoff` 或 `crashed` 状态且持久化的虚拟机。
- 磁盘复制使用 libvirt Storage Volume API，生成独立可恢复副本。
- 不复制内存状态；恢复后等价于一次正常关机后的重新启动。
- 运行中的虚拟机在界面中明确提示“请先关机”，不自动强制关机。

后续阶段再评估两种在线方案：

1. **libvirt 外部快照 + 磁盘复制 + blockcommit**：兼容性较广，但需要处理快照链、回滚和失败清理。
2. **libvirt Backup API / 增量备份**：能力更完整，但依赖 libvirt/QEMU 版本和存储驱动，需要单独的兼容矩阵。

不建议首期直接实现在线备份，因为“快照创建成功”不等于“备份一致且可恢复”，失败路径很容易留下不可用的快照链。

### 3.2 备份存储库

新增“备份存储库”概念：

- 首期类型只支持 `libvirt-pool`，即一个已启用且活跃的 libvirt 存储池。
- 管理员在系统设置中指定默认备份存储库；创建备份时仍可由 Operator 选择其他活跃池。
- 默认存储库建议使用独立磁盘或独立 NFS 目录，不建议与虚拟机主磁盘共用同一存储池。
- 页面展示可用容量、预计备份大小和容量不足警告，但不把“容量足够”当作绝对保证。

数据模型保留扩展点，未来可增加：

- `s3`：S3 兼容对象存储。
- `sftp`：远端目录。
- `nfs`：宿主机挂载目录。

### 3.3 备份集与文件布局

每次备份生成一个逻辑“备份集”，由 `backup_id` 唯一标识。首期继续使用 libvirt 存储池卷承载产物，但卷名必须带备份 ID，避免不同虚拟机或同名备份冲突：

```text
vmb-<backup-id>-domain.xml
vmb-<backup-id>-manifest.json
vmb-<backup-id>-disk-00.qcow2
vmb-<backup-id>-disk-01.raw
```

`manifest.json` 最后写入，作为备份集完成的标志。manifest 至少包含：

```json
{
  "formatVersion": "virtmgr.vm-backup/v2",
  "backupId": "vmb_xxx",
  "createdAt": "2026-11-09T12:00:00Z",
  "toolVersion": "v0.4.0",
  "source": {
    "vmUuid": "original-uuid",
    "vmName": "demo-db-01",
    "state": "shutoff",
    "persistent": true
  },
  "config": {
    "artifact": "vmb_xxx-domain.xml",
    "sizeBytes": 12345,
    "sha256": "..."
  },
  "disks": [
    {
      "target": "vda",
      "sourceVolume": "demo-db-01.qcow2",
      "artifact": "vmb_xxx-disk-00.qcow2",
      "format": "qcow2",
      "capacityBytes": 85899345920,
      "sha256": "可选，严格校验时计算"
    }
  ],
  "restore": {
    "defaultTargetPool": "default",
    "requiresNewMac": true,
    "containsMemory": false
  }
}
```

兼容策略：

- 保留读取 `virtmgr.vm-backup/v1` 的能力。
- v1 备份可展示和恢复，但不会强行改写原 manifest。
- 新建备份统一写 v2。

### 3.4 数据模型

新增迁移 `0004_backups.sql`，建议表结构：

#### `backup_repositories`

| 字段 | 说明 |
|---|---|
| `id` | 主键 |
| `name` | 唯一名称 |
| `type` | 首期固定 `libvirt-pool` |
| `config_json` | 存储池名称等配置 |
| `created_at` / `updated_at` | 时间戳 |

#### `backups`

| 字段 | 说明 |
|---|---|
| `id` | 备份 ID，与 manifest 一致 |
| `vm_uuid` / `vm_name` | 备份时的虚拟机身份快照 |
| `repository_id` | 所属存储库 |
| `name` | 用户可见备份名称 |
| `status` | `running` / `completed` / `failed` / `interrupted` / `deleting` |
| `format_version` | 备份格式版本 |
| `manifest_volume` / `config_volume` | 产物卷名 |
| `total_bytes` | 预估/实际大小 |
| `created_by` | 操作者 |
| `created_at` / `completed_at` | 时间戳 |
| `error` | 失败原因 |
| `pinned` | 是否禁止自动清理 |

#### `backup_disks`

| 字段 | 说明 |
|---|---|
| `backup_id` | 外键 |
| `sequence` | 磁盘序号 |
| `target` | 域内设备名 |
| `source_volume` | 原磁盘卷 |
| `artifact_volume` | 备份产物卷 |
| `format` / `capacity_bytes` | 格式与容量 |
| `sha256` | 可选校验值 |

#### `backup_schedules`

| 字段 | 说明 |
|---|---|
| `id` | 主键 |
| `vm_uuid` | 目标虚拟机 |
| `repository_id` | 默认存储库 |
| `enabled` | 是否启用 |
| `schedule` | 首期使用固定间隔，如每 24 小时 |
| `timezone` | 默认 `Asia/Shanghai` |
| `retention_count` | 至少保留最近 N 个成功备份 |
| `retention_days` | 可选最大保留天数 |
| `last_run_at` / `next_run_at` | 调度状态 |
| `last_error` | 最近一次失败原因 |

任务仍然写现有 `tasks` 表。新增任务类型：

- `vm.backup`：创建备份，兼容现有类型。
- `vm.backup.restore`：恢复。
- `vm.backup.verify`：校验。
- `vm.backup.delete`：删除。

## 4. 用户体验与页面设计

### 4.1 左侧导航新增“备份”

仅向 `viewer`、`operator`、`admin` 展示只读列表；写按钮按角色和只读模式禁用。

备份中心结构：

1. 顶部统计：备份总数、成功/失败、占用容量、最近成功时间。
2. 标签页：
   - **备份集**：时间、虚拟机、备份名、状态、大小、存储库、创建人、操作。
   - **定时计划**：虚拟机、频率、保留数量、下次执行、最近结果。
   - **存储库**：名称、类型、池、容量、状态。首期仅管理员可维护。
3. 备份详情抽屉/页面：
   - manifest 摘要、源虚拟机身份、创建状态。
   - 磁盘列表、目标卷、容量、校验状态。
   - 操作：校验、恢复为新虚拟机、固定保留、删除。

### 4.2 虚拟机详情页

现有“冷备份”卡片升级为“备份与恢复”：

- 保留“创建备份”入口，但提交后跳转到备份详情或显示任务进度。
- 新增该虚拟机最近备份列表。
- 明确显示“需要关机”的前置条件和“不含内存状态”的能力边界。
- 每个历史备份提供查看、校验、恢复入口。

### 4.3 恢复向导

恢复只允许“恢复为新虚拟机”，分四步：

1. **选择备份**：展示备份格式、创建时间、大小、校验状态。
2. **配置目标**：新名称、目标存储池、是否保留 PCI/USB 直通、是否创建后启动。
3. **预检**：名称冲突、UUID 重新生成、存储池容量、网络是否存在、源产物是否齐全、manifest 是否可读。
4. **确认执行**：二次确认后创建恢复任务，实时展示复制磁盘和定义虚拟机的进度。

恢复规则：

- 生成新的 VM UUID。
- 生成新的 MAC 地址，避免与现有虚拟机冲突。
- 默认不保留 PCI/USB 直通设备，避免设备被占用；高级选项才允许保留。
- 磁盘恢复为独立文件，不保留外部 backing chain。
- 恢复失败时删除本次已创建的卷和域定义，不修改源虚拟机。
- 恢复成功后可选择启动，但默认不自动启动。

## 5. 后端流程

### 5.1 创建备份

```text
校验权限/只读/CSRF
  → 读取 VM 详情并校验 shutoff/crashed + persistent
  → 校验存储库和容量
  → 生成 backup_id + manifest 计划
  → 创建任务
  → 逐盘复制为 vmb-<id>-disk-NN
  → 写入 vmb-<id>-domain.xml
  → 计算配置校验值
  → 最后写入 vmb-<id>-manifest.json
  → 更新 backups 状态为 completed
  → 写审计并更新任务进度
```

任何一步失败都要：

1. 删除本次已创建的产物卷。
2. 把内存状态标记为 `failed`。
3. 写失败审计并记录任务失败原因。
4. 不把未完成的 manifest 当作可恢复备份。

### 5.2 完整性与恢复预检

校验分两档：

- **quick**：manifest 可解析、产物齐全、配置 XML SHA-256 正确。
- **deep**：额外检查每个磁盘产物是否可打开、格式和容量是否匹配；完整磁盘 SHA-256 校验作为可选操作，避免对大磁盘造成持续 IO 压力。

恢复前默认执行 quick；deep 校验由用户显式触发。

### 5.3 删除备份

- 只允许删除 `completed` / `failed` / `interrupted` 的备份集；运行中的备份禁止删除。
- 先删除所有产物卷，再删除 SQLite 记录，最后写审计。
- 部分卷删除失败时保留数据库记录并标记 `delete_failed`，允许重试，避免产生不可见的孤儿卷。
- 提供“孤儿备份”扫描：存储库中存在 `vmb-` 前缀但目录中没有记录的卷，只展示，不自动删除。

### 5.4 备份恢复

```text
读取 backup + manifest
  → quick verify
  → 目标名称/容量/网络/直通预检
  → 创建恢复任务
  → 按 manifest 复制磁盘到目标存储池
  → 改写 XML：UUID、名称、MAC、磁盘路径
  → DefineXML 新域
  → 可选设置 autostart / 启动
  → 更新任务、审计和备份恢复历史
```

失败回滚范围仅限本次恢复创建的资源，不能影响原备份集。

## 6. 定时备份与保留策略

调度器复用现有定时快照策略的实现模式：

- 应用启动时加载启用的计划，每分钟扫描到期任务。
- 默认全局最多 1 个备份任务并发，避免磁盘 IO 叠加。
- 计划执行时再次校验虚拟机状态；运行中则记录跳过原因，不自动关机。
- 新备份成功后执行保留清理。
- 清理顺序：未固定 → 非手动保护 → 最旧的成功备份。
- 永远至少保留 `retention_count` 个成功备份；可选 `retention_days` 到期后再删除。
- 失败备份不占用成功保留数量，但应在界面上可见并允许清理。
- 计划不会与快照调度器同时触发同一虚拟机的高 IO 任务；后续可以抽出统一任务锁。

首期调度配置建议只提供：

- 每 N 小时/天执行一次。
- 时间窗口（可选）。
- 保留最近 N 份。
- 最大保留天数。
- 启用/暂停。

暂不引入完整 Cron 表达式，避免 UI 和时区配置复杂化。

## 7. API 草案

### 7.1 备份存储库

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/backup-repositories` | 列表 |
| POST | `/api/v1/backup-repositories` | 创建存储库，Admin |
| PATCH | `/api/v1/backup-repositories/{id}` | 修改，Admin |
| DELETE | `/api/v1/backup-repositories/{id}` | 删除，Admin |

### 7.2 备份集

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/backups?vmUuid=&status=&page=` | 备份列表 |
| POST | `/api/v1/backups` | 创建备份，Operator/Admin |
| GET | `/api/v1/backups/{id}` | 备份详情 |
| POST | `/api/v1/backups/{id}/verify` | quick/deep 校验 |
| POST | `/api/v1/backups/{id}/restore` | 恢复为新虚拟机 |
| DELETE | `/api/v1/backups/{id}` | 删除备份集 |
| PATCH | `/api/v1/backups/{id}` | 固定/取消固定保留 |

兼容处理：保留 `POST /api/v1/vms/{uuid}/backups`，内部转调新的备份服务，避免现有调用方失效。

### 7.3 定时计划

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/backup-schedules` | 计划列表 |
| POST | `/api/v1/backup-schedules` | 创建计划 |
| PATCH | `/api/v1/backup-schedules/{id}` | 更新/启停计划 |
| DELETE | `/api/v1/backup-schedules/{id}` | 删除计划 |
| POST | `/api/v1/backup-schedules/{id}/run` | 立即执行一次 |

## 8. 权限与审计

推荐权限矩阵：

| 操作 | viewer | operator | admin |
|---|---:|---:|---:|
| 查看备份列表/详情 | 是 | 是 | 是 |
| 查看 manifest 元数据 | 是 | 是 | 是 |
| 创建备份 | 否 | 是 | 是 |
| 校验备份 | 否 | 是 | 是 |
| 删除备份 | 否 | 否 | 是 |
| 恢复为新虚拟机 | 否 | 否 | 是 |
| 需要二次确认输入备份名 | - | - | 是 |
| 管理存储库 | 否 | 否 | 是 |

全局只读模式禁止创建、删除、恢复和管理存储库；查看和 quick verify 可选择保留为只读操作。

审计动作建议：

- `backup.create`
- `backup.verify`
- `backup.restore`
- `backup.delete`
- `backup.schedule.create`
- `backup.schedule.update`
- `backup.schedule.delete`
- `backup.repository.create/update/delete`

审计 detail 只记录备份 ID、虚拟机身份、大小、状态、磁盘数量和错误摘要，不记录磁盘内容、XML 正文、Cloud-init 内容或备份文件路径中的敏感信息。

## 9. 安全与资源保护

- 备份名称、存储库 ID、目标池和恢复名称必须使用严格白名单校验。
- 不接受任意宿主机路径；所有读写都经过 libvirt 存储池或受控存储库适配器。
- 备份磁盘是完整数据副本，默认禁止浏览器下载；只有未来增加“受审计导出”功能时才开放。
- 默认限制恢复/备份并发为 1，避免占满磁盘 IO。
- 备份任务设置超时；现有创建备份超时为 6 小时，恢复任务应使用独立可配置超时。
- 大磁盘操作必须流式复制，不把磁盘整体读入内存。
- 恢复时重新生成 UUID/MAC，避免与现有域和网络冲突。
- 只允许管理员执行恢复和删除，降低误操作风险。

## 10. 分阶段落地

### 阶段 1：备份目录与手动备份

- 新增 `backups`、`backup_disks`、`backup_repositories` 表和迁移。
- 统一 backup manifest v2。
- 新增备份中心列表和详情页。
- 兼容现有 `POST /vms/{uuid}/backups`。
- 完成删除、quick verify、失败清理和任务进度。
- 保留虚拟机详情页现有创建入口，但改为写入备份目录。

### 阶段 2：恢复为新虚拟机

- 恢复预检和恢复任务。
- XML 重写：新 UUID、名称、MAC、磁盘路径。
- 回滚本次恢复资源。
- 恢复向导 UI。
- 恢复审计和 E2E 闭环。

### 阶段 3：定时备份与保留

- 计划表、调度器、立即执行。
- 保留数量和保留天数清理。
- 固定保留标记。
- 与任务锁/快照调度协同。

### 阶段 4：在线与增量能力

- 评估 libvirt Backup API 和外部快照方案。
- 按 QEMU/libvirt 版本做兼容矩阵。
- 远程存储库、压缩和加密。
- 在-place 覆盖恢复，需独立安全评审。

## 11. 验收标准

### 备份

- 关机持久虚拟机可以创建完整备份，manifest 最后落盘。
- 任意一步失败后，本次产物卷被清理，任务和备份记录明确失败。
- 备份列表可准确展示状态、大小、来源、创建人和存储库。
- 删除备份会删除全部产物；部分失败时可重试且不会静默丢失记录。
- quick verify 能发现 manifest 缺失、配置校验不符和磁盘缺失。

### 恢复

- 恢复出的新虚拟机拥有新 UUID、新 MAC 和独立磁盘文件。
- 原虚拟机定义和磁盘不被修改。
- 恢复失败后不残留新域定义和本次创建的磁盘。
- 预检能阻止名称冲突、网络不存在和容量不足。
- 恢复成功后可正常启动并读取原磁盘数据。

### 调度与权限

- 运行中的虚拟机不会被定时任务自动关机。
- 同一时间最多一个备份任务执行。
- 保留策略只删除符合条件的旧成功备份，固定备份永不自动删除。
- Viewer 无法创建、恢复、删除；Operator 无法恢复和删除；Admin 可以。
- 全局只读模式阻断所有备份写操作。
- 所有成功/失败/拒绝操作都有审计记录。

## 12. 已确认的实现边界

1. **首期只做冷备份。** 仅支持 `shutoff` / `crashed` 的持久虚拟机；在线和增量备份后置。
2. **只支持恢复为新虚拟机。** 不覆盖或修改源虚拟机，恢复后生成新 UUID、MAC 和独立磁盘。
3. **备份存储首期只使用 libvirt 活跃存储池。** 自动发现活跃池并优先把 `default` 设为默认存储库。
4. **权限采用 Operator 创建/校验、Admin 删除/恢复/管理存储库。** 所有写操作同时受全局只读模式约束。
5. **首期只完整支持 `file` 后端磁盘。** 块设备、iSCSI、Ceph RBD 等后端需要后续独立适配。
6. **不提供备份下载。** 备份产物仅通过受控的校验、恢复和删除流程使用。

阶段 1 至阶段 3 已按上述边界实现；在线/增量备份属于后续阶段，需要单独的兼容矩阵和安全评审。

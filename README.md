# VirtMgr Console

面向单台物理机 KVM/libvirt 实例的 Web 虚拟机管理平台。后端使用 Go，前端使用 React、TypeScript、Tailwind CSS 和 shadcn/ui 风格组件。

## 当前能力

- 本地管理员登录、HttpOnly 会话与退出登录。
- libvirt 宿主机信息读取。
- 物理主机页：CPU 拓扑与频率、内存、libvirt 版本、资源分配率、存储池、虚拟网络、虚拟机资源分配，以及 PCI/USB/网络/块设备的 libvirt 节点设备视图。
- 实时资源趋势：物理主机与虚拟机详情页每 2 秒采样 CPU、内存和网络吞吐，服务端使用前后计数差计算利用率/速率，前端保留最近 30 个采样点并以 SVG 折线展示。
- 全局搜索：顶栏 `Ctrl+K` / `Meta+K` 打开，可按虚拟机名称、UUID、主机名、状态、IP、网络源，以及虚拟网络、存储池和页面名称快速跳转；结果按角色过滤，支持方向键、Enter 和 Escape 键盘操作。
- 实时事件页：通过 SSE 接收 libvirt 域生命周期事件、快照/存储/网络资源变更；支持搜索和级别筛选，并可直接跳转到相关虚拟机详情。
- 任务中心：从 SQLite 查询任务状态、进度、步骤、失败原因和关联资源，支持按状态筛选与分页；ISO 上传、虚拟机克隆和冷备份均接入真实任务进度，服务重启时自动将排队中或执行中的任务标记为已中断。
- 虚拟机列表、状态、vCPU、内存、磁盘、网络和自动启动信息。
- 虚拟机详情：UUID、OS/架构/机型、磁盘、网卡、VNC 图形控制台，以及可切换运行态/持久配置的只读域 XML；XML 使用 CodeMirror 6 展示，支持行号、折叠、搜索和 XML 语法高亮。
- 域 XML 编辑：Admin 可在可写模式下通过 CodeMirror 6 编辑持久配置 XML。保存前由前端限制 512 KiB、后端校验名称/UUID 不可变并拒绝 QEMU commandline 命名空间，随后通过 libvirt `DOMAIN_DEFINE_VALIDATE` 校验并更新定义；只修改持久配置，不启动虚拟机，运行中的域在下次启动时生效，操作记录 `vm.xml.update` 审计。
- 虚拟机配置导出：详情页可下载稳定的 JSON 导出包，包含虚拟机详情、持久配置 XML、磁盘/网络/快照元数据；运行中的域额外包含 live XML。导出为只读能力，Viewer 和全局只读模式均可使用，并记录 `vm.export` 审计。
- 虚拟机冷备份：详情页可为关机或异常停止的持久域创建异步 `vm.backup` 任务，将 inactive 持久配置 XML、带 SHA-256 的 manifest 和每个可写 `file` 磁盘复制到指定活跃存储池。操作要求 Operator/Admin、可写模式和 CSRF；本次已创建但未完成的备份卷会自动回滚，成功与失败均记录审计。备份不包含内存状态，暂不提供恢复能力。
- Guest Agent 信息：运行中虚拟机详情通过 QEMU Guest Agent 读取主机名和 IPv4/IPv6 地址，过滤回环、链路本地和重复地址；Agent 不可用时显示“未连接”且不影响详情加载。虚拟机列表展示 Agent 可提供的 IP。
- VNC 控制台：Operator/Admin 可通过一次性 token 打开独立全屏 noVNC 页面，WebSocket 经服务端代理到 libvirt graphics FD，浏览器不直连宿主机 VNC 端口。
- 存储池与卷视图：池状态、容量、目标路径、卷格式、容量和分配量。
- 存储池生命周期：Operator/Admin 可创建目录池（dir）、启动/停止、刷新卷清单，并删除未激活的池定义；池内文件不会被自动删除，操作写入审计。
- 在活跃存储池创建 qcow2/raw 卷：支持容量、格式和重名校验，写操作受角色/只读模式约束并记录审计。
- 上传 ISO：从浏览器以原始字节流写入活跃存储池的 libvirt 卷，限制 `.iso` 名称、1 MiB - 16 GiB 大小和必填 `Content-Length`，失败自动清理未完成卷。
- 删除存储卷：删除前输入卷名确认；会扫描全部 libvirt 域磁盘来源，仍被虚拟机使用的卷拒绝删除。
- 虚拟网络管理：查看 NAT/桥接模式、桥接口、IPv4/IPv6 地址、DHCP 范围和实时 DHCP 租约；Operator/Admin 可创建 NAT 或隔离网络、启停网络并删除未激活网络。
- 创建虚拟机：指定名称、vCPU、内存、BIOS/UEFI 固件和 Secure Boot，可从活跃存储池选择已有 qcow2/raw 卷并自动填充磁盘配置，也可手工输入路径；网络从活跃虚拟网络中选择。UEFI 使用 libvirt firmware 自动选择，Secure Boot 仅在 UEFI 固件下可选，非法组合由后端拒绝并写审计。
- Cloud-init：创建虚拟机时可启用 NoCloud seed ISO，填写 cloud-config 或 shell user-data；后端在指定活跃存储池生成卷标为 `cidata` 的 ISO，并作为只读 CD-ROM 挂载。审计只记录启用状态、存储池和字节数，不记录用户数据内容。
- 虚拟机模板：Operator/Admin 可在独立模板页维护名称、描述、vCPU、内存、磁盘格式、网络、固件、Secure Boot、Cloud-init、ISO 路径和启动策略；新建虚拟机页面可从模板预填规格，虚拟机名称与磁盘路径仍需操作者填写。模板写操作受角色/只读模式约束并记录 `vm_template.create/update/delete` 审计，审计不保存 Cloud-init 内容。
- PCI / USB 设备直通：虚拟机详情页可从宿主机 libvirt 节点设备中选择 PCI 或 USB 设备直通，展示类型、厂商/型号、地址和驱动；运行中的持久域使用 live+config 热插拔，关机域写入持久配置，同一设备不能被多台虚拟机重复分配。写操作受角色/只读模式约束并记录 `vm.device.attach` / `vm.device.detach` 审计。
- 定时快照策略：虚拟机详情页可创建、编辑、启停、删除策略并立即执行；间隔限制为 5 分钟到 7 天，保留数量为 1-50。服务端每分钟扫描到期策略，只清理带本策略 `[policy:<id>]` 标记的历史快照，不触碰手工快照；策略变更与执行记录 `vm.snapshot_policy.create/update/delete` 和 `vm.snapshot.schedule` 审计。
- 克隆虚拟机：仅允许关机或异常停止的持久域；通过 libvirt 存储卷复制创建独立磁盘，生成新的 UUID、MAC、VNC 端口和可选 NVRAM，支持克隆后启动/autostart。克隆以 `vm.clone` 异步任务执行，实时写入任务进度，失败自动回滚已创建的卷和域定义，并记录审计。
- 删除已关机虚拟机：删除前输入名称确认；当前版本只删除域定义，保留磁盘文件。
- 生命周期动作：启动、正常关机、重启、强制重启、暂停、恢复、强制下电。
- 批量生命周期操作：虚拟机列表支持多选并批量执行启动、关机、重启、暂停、恢复和强制动作；最多 100 台，逐台校验状态并独立写入审计，部分失败不影响其余虚拟机。
- 虚拟机开机自启动：详情页可实时启用或禁用持久域 autostart；临时域、Viewer 和全局只读模式禁止修改，操作写入审计。
- CPU / 内存调整：详情页可修改持久域的 vCPU 和内存。运行、阻塞或暂停的域只能在当前最大 vCPU / 最大内存内热调整；关机或异常停止的域可扩展持久最大值，下次启动生效。操作受角色/只读模式约束并记录 `vm.resources.update` 审计。
- 磁盘扩容：详情页展示每块磁盘的虚拟容量、已分配空间和数据源，并支持对可写文件型 qcow2/raw 数据盘扩容。只允许增大容量；容量通过 libvirt 域块信息读取，运行中的域使用 live block resize，关机域优先调整存储卷，未注册为存储卷的嵌套文件通过临时目录池调整。宿主机扩容后仍需在客户机内扩展分区和文件系统，操作记录 `vm.disk.resize` 审计。
- 写操作角色校验、全局只读开关、CSRF 校验和 SQLite 审计记录。
- 运行时只读模式：管理员可在系统设置页即时启用或关闭只读模式；状态持久化到 SQLite，立即影响会话和所有虚拟机、存储、网络、用户写操作，并记录 `settings.readonly.update` 审计。
- 审计日志：管理员可查询全部写操作记录，支持按动作、结果筛选和分页。
- 用户管理：管理员可创建本地用户、调整 Viewer/Operator/Admin 角色、禁用或启用账号；不能修改当前登录账号自身。
- 虚拟机搜索与状态筛选。
- 深色/浅色主题、移动端抽屉导航、键盘跳转到主内容、抽屉焦点陷阱与 Escape 关闭。
- 移动端虚拟机列表采用横向滚动和首列冻结；详情页、资源趋势和 VNC 控制台在窄屏下保持可用。
- SQLite 用户/会话存储，使用 `modernc.org/sqlite`，不依赖 CGO SQLite。
- 只读健康检查：`/healthz`、`/readyz`。

首次启动默认只读，不会对 libvirt 执行写操作。`VIRTMGR_READ_ONLY` 用于初始化或尚未持久化时的默认值；管理员可在“系统设置”中即时切换，后续启动读取 SQLite 中的持久化状态。写操作开启后，`admin` 和 `operator` 角色可执行虚拟机、存储池、存储卷、网络和用户管理操作；强制重启、强制下电和删除资源需要二次确认。

## 环境要求

- Go 1.26+
- Node.js 24+ 与 npm
- libvirt 开发库：构建机需要 `libvirt-devel`，运行机需要 `libvirt-libs`
- 可访问的 libvirt socket，默认 `qemu:///system`

## 本地运行

首次启动必须提供管理员密码：

```bash
export VIRTMGR_ADMIN_PASSWORD='replace-with-a-strong-password'
export VIRTMGR_DATABASE_PATH=/tmp/virtmgr.db
export VIRTMGR_LISTEN=127.0.0.1:8443

make build
./bin/virtmgr
```

打开 `http://127.0.0.1:8443`。

前端开发模式：

```bash
# 终端 1：启动后端
VIRTMGR_ADMIN_PASSWORD=secret VIRTMGR_DATABASE_PATH=/tmp/virtmgr.db go run ./cmd/virtmgr

# 终端 2：启动 Vite
cd web && npm run dev
```

## 配置

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `VIRTMGR_LISTEN` | `127.0.0.1:8443` | HTTP 监听地址 |
| `VIRTMGR_LIBVIRT_URI` | `qemu:///system` | libvirt 连接 URI |
| `VIRTMGR_DATABASE_PATH` | `/var/lib/virtmgr/virtmgr.db` | SQLite 路径 |
| `VIRTMGR_HOST_NAME` | 系统主机名 | 页面展示的主机名 |
| `VIRTMGR_ADMIN_USERNAME` | `admin` | 首次启动创建的管理员用户名 |
| `VIRTMGR_ADMIN_PASSWORD` | 无 | 首次启动必须提供 |
| `VIRTMGR_SESSION_TTL` | `8h` | 会话有效期 |
| `VIRTMGR_COOKIE_SECURE` | `false` | HTTPS 部署时设为 `true` |
| `VIRTMGR_READ_ONLY` | `true` | 只读模式 |
| `VIRTMGR_DEMO_MODE` | `false` | 使用确定性的 demo 数据，供 E2E 使用 |

> 写操作会直接调用 libvirt。首次在真实宿主机启用前，请确认已具备备份、维护窗口和回滚预案。E2E 只在 `VIRTMGR_DEMO_MODE=true` 下执行写操作，不会触碰真实虚拟机。

## 已实现 API

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/v1/session` | 登录并建立 HttpOnly 会话 |
| `GET` | `/api/v1/session` | 当前用户、角色、只读模式和 CSRF token |
| `DELETE` | `/api/v1/session` | 退出登录 |
| `GET` | `/api/v1/overview` | 宿主机和虚拟机总览 |
| `GET` | `/api/v1/host` | 物理主机资源、存储池、虚拟网络和虚拟机分配明细 |
| `GET` | `/api/v1/host/devices` | libvirt 节点设备列表，包含 PCI、USB、网络、块设备和 DRM 设备 |
| `GET` | `/api/v1/host/metrics` | 宿主机 CPU、内存和物理网卡实时采样与速率 |
| `GET` | `/api/v1/events?limit=100` | 读取最近事件快照 |
| `GET` | `/api/v1/events/stream?limit=100` | SSE 事件流：先推送最近事件快照，再推送实时增量和 15 秒心跳 |
| `GET` | `/api/v1/tasks` | 任务列表，支持 `status` / `page` / `pageSize` |
| `GET` | `/api/v1/tasks/{id}` | 单个任务详情 |
| `GET` | `/api/v1/storage/pools` | 存储池列表、状态和容量 |
| `GET` | `/api/v1/storage/pools/{pool}` | 存储池详情 |
| `POST` | `/api/v1/storage/pools` | 创建目录类型存储池，需 `X-CSRF-Token` |
| `POST` | `/api/v1/storage/pools/{pool}/actions/{action}` | 启动、停止或刷新存储池，需 `X-CSRF-Token` |
| `DELETE` | `/api/v1/storage/pools/{pool}` | 删除未激活的存储池定义，需 `X-CSRF-Token` |
| `GET` | `/api/v1/storage/pools/{pool}/volumes` | 活跃存储池的卷列表 |
| `POST` | `/api/v1/storage/pools/{pool}/volumes` | 在活跃存储池创建 qcow2/raw 卷，需 `X-CSRF-Token` |
| `POST` | `/api/v1/storage/pools/{pool}/volumes/upload?name=install.iso` | 以 `application/octet-stream` 流式上传 ISO，需 `Content-Length` 和 `X-CSRF-Token` |
| `DELETE` | `/api/v1/storage/pools/{pool}/volumes/{volume}` | 删除未被虚拟机占用的卷，需 `X-CSRF-Token` |
| `GET` | `/api/v1/networks` | 虚拟网络、桥接口、地址和 DHCP 配置 |
| `GET` | `/api/v1/networks/{name}/leases` | 读取网络 DHCP 租约，包含主机名、IP、MAC、接口和到期时间 |
| `POST` | `/api/v1/networks` | 创建 NAT/隔离虚拟网络，需 `X-CSRF-Token` |
| `PATCH` | `/api/v1/networks/{name}` | 编辑未激活 NAT/隔离网络的模式、IPv4 子网、DHCP 范围和 autostart，需 `X-CSRF-Token` |
| `POST` | `/api/v1/networks/{name}/hosts` | 为网络添加静态 DHCP 主机条目，活跃和未激活网络均支持，需 `X-CSRF-Token` |
| `DELETE` | `/api/v1/networks/{name}/hosts/{mac}` | 删除网络静态 DHCP 主机条目，活跃和未激活网络均支持，需 `X-CSRF-Token` |
| `POST` | `/api/v1/networks/{name}/actions/{action}` | 启动或停止虚拟网络，需 `X-CSRF-Token` |
| `DELETE` | `/api/v1/networks/{name}` | 删除未激活虚拟网络，需 `X-CSRF-Token` |
| `GET` | `/api/v1/vms` | 虚拟机列表，支持 `q` / `state` |
| `POST` | `/api/v1/vms` | 创建虚拟机域定义，需 `X-CSRF-Token` |
| `POST` | `/api/v1/vms/{uuid}/clone` | 将关机/异常停止的持久域克隆为异步 `vm.clone` 任务，返回 `202` 和任务对象，需 `X-CSRF-Token` |
| `POST` | `/api/v1/vms/{uuid}/backups` | 将关机/异常停止的持久域冷备份到活跃存储池，创建异步 `vm.backup` 任务并返回 `202`，请求体 `{ "name": "...", "targetPool": "..." }`，需 `X-CSRF-Token` |
| `GET` | `/api/v1/vms/{uuid}` | 虚拟机详情 |
| `GET` | `/api/v1/vms/{uuid}/metrics` | 虚拟机 CPU、内存和虚拟网卡实时采样与速率 |
| `GET` | `/api/v1/vms/{uuid}/xml?format=live\|config` | 只读域 XML，支持运行态和持久配置 |
| `PUT` | `/api/v1/vms/{uuid}/xml` | Admin 编辑持久配置 XML，请求体 `{ "xml": "..." }`，需 `X-CSRF-Token` |
| `GET` | `/api/v1/vms/{uuid}/export` | 下载虚拟机配置与元数据 JSON 包，响应带 `Content-Disposition` |
| `POST` | `/api/v1/vms/{uuid}/console-token` | 申请 30 秒有效、单次使用、绑定当前用户和虚拟机的控制台 token，需 `X-CSRF-Token` |
| `GET` | `/api/v1/vms/{uuid}/console?token=...` | WebSocket 升级并代理 VNC 图形通道 |
| `GET` | `/api/v1/vms/{uuid}/snapshots` | 虚拟机 libvirt 快照列表 |
| `POST` | `/api/v1/vms/{uuid}/snapshots` | 创建快照，需 `X-CSRF-Token` |
| `POST` | `/api/v1/vms/{uuid}/snapshots/{name}/revert` | 回滚快照，需 `X-CSRF-Token` |
| `DELETE` | `/api/v1/vms/{uuid}/snapshots/{name}` | 删除快照，需 `X-CSRF-Token` |
| `GET` | `/api/v1/vms/{uuid}/snapshot-policies` | 定时快照策略列表 |
| `POST` | `/api/v1/vms/{uuid}/snapshot-policies` | 创建定时快照策略，需 `X-CSRF-Token` |
| `PATCH` | `/api/v1/snapshot-policies/{id}` | 更新策略、启停策略，需 `X-CSRF-Token` |
| `DELETE` | `/api/v1/snapshot-policies/{id}` | 删除策略定义，需 `X-CSRF-Token` |
| `POST` | `/api/v1/snapshot-policies/{id}/run` | 立即执行一次策略并返回新快照，需 `X-CSRF-Token` |
| `DELETE` | `/api/v1/vms/{uuid}` | 删除已关机虚拟机域定义，需 `X-CSRF-Token` |
| `PATCH` | `/api/v1/vms/{uuid}/autostart` | 启用或禁用开机自启动，请求体 `{ "enabled": true }`，需 `X-CSRF-Token` |
| `PATCH` | `/api/v1/vms/{uuid}/resources` | 调整 vCPU 与内存，请求体 `{ "vcpus": 4, "memoryMiB": 4096 }`，需 `X-CSRF-Token` |
| `PATCH` | `/api/v1/vms/{uuid}/disks/{target}` | 扩容 qcow2/raw 数据盘，请求体 `{ "capacityBytes": 107374182400 }`，需 `X-CSRF-Token` |
| `POST` | `/api/v1/vms/{uuid}/actions/{action}` | 生命周期动作，需 `X-CSRF-Token` |
| `POST` | `/api/v1/vms/{uuid}/devices` | 直通 PCI/USB 主机设备，请求体 `{ "device": "pci_0000_3b_00_0" }`，需 `X-CSRF-Token` |
| `DELETE` | `/api/v1/vms/{uuid}/devices/{device}` | 移除直通设备，需 `X-CSRF-Token` |
| `GET` | `/api/v1/templates` | 虚拟机模板列表 |
| `POST` | `/api/v1/templates` | 创建虚拟机模板，需 `X-CSRF-Token` |
| `PATCH` | `/api/v1/templates/{id}` | 更新虚拟机模板，需 `X-CSRF-Token` |
| `DELETE` | `/api/v1/templates/{id}` | 删除虚拟机模板，需 `X-CSRF-Token` |
| `GET` | `/api/v1/audit-events` | 审计查询，仅管理员；支持 `actor` / `action` / `targetKind` / `result` / `page` / `pageSize` |
| `GET` | `/api/v1/users` | 本地用户列表，仅管理员 |
| `POST` | `/api/v1/users` | 创建本地用户，需管理员与 `X-CSRF-Token` |
| `PATCH` | `/api/v1/users/{id}` | 更新用户角色和启用状态，需管理员与 `X-CSRF-Token` |

动作白名单为 `start`、`shutdown`、`reboot`、`reset`、`pause`、`resume`、`destroy`。允许动作由 libvirt 当前状态推导，前端只消费后端返回的 `allowedActions`。

开机自启动直接调用 libvirt `SetAutostart`，只对持久域开放；临时域返回 `409 ACTION_NOT_ALLOWED`。该操作同样受角色和全局只读模式约束，并记录 `vm.autostart` 审计，详情包含操作前后的 autostart 值。

CPU / 内存调整使用 libvirt `SetVcpusFlags` 和 `SetMemoryFlags`。运行中的域以 `LIVE|CONFIG` 热调整，但目标值不能超过当前活动最大 vCPU / 最大内存；超过上限返回 `409 ACTION_NOT_ALLOWED`，需先关机再扩容。关机或异常停止的域使用 `CONFIG`，需要扩容时先更新持久最大值再更新当前值。vCPU 限制为 1-255，内存限制为 128 MiB - 1 TiB；成功和失败均记录 `vm.resources.update` 审计，包含调整前后的规格。

磁盘扩容通过 `PATCH /api/v1/vms/{uuid}/disks/{target}` 接受目标容量（字节）。支持持久域中可写、未只读的 `file` 类型 qcow2/raw 数据盘，新容量必须大于当前容量，单盘上限 1 PiB。容量读取优先使用 `Domain.GetBlockInfo`，因此目录卷内的嵌套磁盘文件也能展示和操作。运行、阻塞、暂停或挂起域使用 `Domain.BlockResize` 的 `BYTES|EXTEND` 热扩展块设备；关机或异常停止域优先使用 `StorageVol.Resize`，若文件未单独注册为存储卷，则临时定义一个指向其所在目录的池，调整卷容量后立即销毁池定义。CD-ROM、只读盘、非文件后端、临时域和无法读取容量的磁盘返回明确错误。成功后记录 `vm.disk.resize` 审计，包含目标设备、数据源、扩容前后容量和结果。

libvirt 扩容的是宿主机侧虚拟磁盘（qcow2 默认仍按需分配），不会自动扩展客户机内分区或文件系统。Linux 客户机通常还需执行 `growpart` 和 `resize2fs` / `xfs_growfs`，Windows 客户机需在磁盘管理中扩展卷；本程序有意不支持缩容。

域 XML 接口默认返回 `live`，可用 `format=config` 读取 libvirt inactive XML；Viewer、Operator 和 Admin 均可读取。只有 Admin 可在可写模式下通过 `PUT` 提交持久配置 XML：请求上限 512 KiB，根元素必须是 `domain`，`name` 与 `uuid` 必须和现有虚拟机一致，且拒绝 QEMU 专有命名空间以避免 commandline hook 注入。后端先解析身份与安全边界，再调用 libvirt `DomainDefineXMLFlags` 的 `DOMAIN_DEFINE_VALIDATE` 完成语义校验；校验失败不会更新定义。编辑只写持久配置，不启动域，运行中的域在下次启动时使用新定义；审计记录名称、状态、字节数、SHA-256 和结果，不保存 XML 正文。

配置导出接口始终读取持久配置 XML，并在虚拟机处于运行、阻塞、暂停、关机中或挂起状态时额外读取 live XML；关机、崩溃或未知状态只导出持久配置，避免对 inactive 域请求 live XML。导出包包含格式版本、导出时间、虚拟机详情、XML、快照列表和不含磁盘数据的声明。导出不在响应中写入宿主机文件，由浏览器保存 JSON；该只读操作不受全局只读模式或 Viewer 角色阻塞，但成功和失败都会记录 `vm.export` 审计，且审计不保存 XML 正文。

Guest Agent 信息在读取单台虚拟机详情时按需查询，使用 1 秒超时；主机名来自 `guest-get-host-name`，IP 来自 `guest-network-get-interfaces`。虚拟机列表也会对运行中/阻塞状态 VM 查询 Agent 信息，使用独立 libvirt 连接、最多 4 个并发和 2 秒批次预算，避免阻塞列表主流程。查询失败一律降级为 `agentConnected=false`，不会让详情或列表接口失败。

VNC 控制台仅对 Operator/Admin 开放，全局只读模式同样禁止打开。一次性 token 默认 30 秒有效且只能消费一次；WebSocket 连接仍要求有效会话 Cookie，服务端通过 `OpenGraphicsFD` 建立到 libvirt VNC graphics 的原始字节通道，不在浏览器暴露宿主机 VNC 端口。控制台打开动作写入审计。

事件页使用进程内事件总线保留最近 500 条事件，通过 `EventSource` 订阅 SSE；慢消费者不会阻塞 libvirt 事件回调。事件用于运行态观察和页面刷新提示，不写入 SQLite，也不替代审计日志。libvirt 事件循环负责将域定义、启动、停止、暂停、恢复和崩溃等状态转换为统一业务事件。

主机设备接口只读调用 libvirt 节点设备 API，按 PCI 地址、USB 总线/设备、网络接口或块设备路径整理展示。设备热插拔期间的短暂查询失败会跳过单个设备，不影响其余设备列表；全部设备都读取失败时接口返回错误。

资源趋势接口返回当前累计计数对应的利用率与速率，不使用额外历史数据库。宿主机 CPU 来自 libvirt 节点 CPU 计数，网卡吞吐来自 `/proc/net/dev` 的物理接口（排除回环和常见虚拟接口）；虚拟机 CPU 来自域累计 CPU 时间，内存优先使用 libvirt RSS，网络吞吐汇总各域网卡 target 设备。首次采样速率返回 0，后续请求按实际时间差计算；计数器回退时速率降级为 0，CPU 使用率限制在 0-100%。前端每 2 秒轮询并保留最近 30 个点。

任务中心当前落地了持久化任务表、查询 API、分页/状态筛选、页面自动刷新和启动恢复逻辑。ISO 上传会创建 `storage.volume.upload` 任务，在流式写入过程中按读取进度更新百分比；虚拟机克隆会创建 `vm.clone` 任务，在准备、磁盘复制、域定义和启动阶段更新进度，并在完成或失败时写入最终状态。demo 模式另预置运行中、成功、失败和已中断任务用于展示。

克隆接口只接受关机或异常停止状态的持久域，并要求可写磁盘是 libvirt 存储池管理的 `file` 卷。后端使用独立 libvirt 连接执行磁盘复制，避免长时间占用主连接；复制前重写域 XML 的 name、UUID、磁盘来源、MAC、NVRAM 和 VNC 端口。默认复制到源卷所在活跃存储池，也可通过请求体 `targetPool` 指定目标池；任务失败会回滚新建域和存储卷。

冷备份接口同样只接受关机或异常停止状态的持久域，并要求每个可写 `file` 磁盘由 libvirt 存储池管理；目标池必须处于活跃状态。备份名需满足安全文件名规则，目标池内已存在同名产物时返回冲突。备份产物写入目标池：`<name>-domain.xml` 保存 inactive 持久域 XML，`<name>-manifest.json` 使用 `virtmgr.vm-backup/v1` 格式记录虚拟机身份、磁盘映射和配置 XML 的 SHA-256，`<name>-disk-NN.<ext>` 保存各磁盘副本。任务按磁盘复制、配置和 manifest 写入阶段更新进度；失败时删除本次已创建但未完成的卷。备份不包含内存状态、不读取宿主机任意路径，也不提供恢复或删除接口；成功和失败均记录 `vm.backup` 审计，且不保存 XML 正文。

审计查询按时间倒序返回，默认每页 50 条、最大 200 条；结果字段仅接受 `success` 或 `failure`。Viewer 和 Operator 访问审计接口返回 `403 FORBIDDEN`，全局只读模式不影响审计读取。

用户管理接口仅管理员可访问。用户名长度为 3-64 位，仅允许字母、数字、点、下划线和短横线；初始密码至少 12 个字符。管理员不能修改当前登录账号自身的角色或状态，用户被禁用后已有会话也会立即失效。全局只读模式会拒绝用户创建和更新。

创建虚拟机当前使用宿主机上已存在的磁盘文件；删除虚拟机操作不删除磁盘。创建请求省略 `firmware` 时按 `bios` 处理，`firmware` 可设为 `bios` 或 `uefi`；`secureBoot` 仅能与 UEFI 一起使用。UEFI 域通过 `<os firmware='efi'>` 和 `<loader secure='yes|no'/>` 交给 libvirt 选择宿主机上可用的 OVMF 固件与 NVRAM 模板，运行态 XML 会回读为 `uefi` 和对应的安全启动状态。创建请求可选携带 `cloudInit: { userData, pool }`；后端生成包含 `meta-data` 与 `user-data`、卷标为 `cidata` 的 NoCloud ISO，通过 `<serial>cloud-init</serial>` 标记的只读 CD-ROM 挂载。用户数据限制为 64 KiB，且不会写入审计。存储页可创建目录类型存储池，执行启动、停止、刷新和删除定义；目标路径必须是绝对路径，池名需满足安全字符限制，删除仅允许在池未激活时执行且不会递归删除磁盘文件。活跃池可创建和删除 qcow2/raw 卷，也可上传 ISO；ISO 上传直接使用请求体作为 libvirt 流，不落宿主机临时文件，仅允许安全文件名的 `.iso` 文件，大小限制为 1 MiB - 16 GiB。删除卷会先检查所有虚拟机磁盘引用，存在占用时返回 `409 VOLUME_IN_USE`。未激活池不能枚举、创建、上传或删除卷。存储池写操作记录 `storage.pool.create`、`storage.pool.start`、`storage.pool.stop`、`storage.pool.refresh` 和 `storage.pool.delete` 审计。

虚拟机模板保存可复用的规格与默认值，不包含实例名称和磁盘路径。模板名称大小写不敏感唯一；创建/更新时校验 vCPU、内存、磁盘格式、网络、ISO 绝对路径、UEFI/Secure Boot 组合和 Cloud-init 64 KiB 上限。模板写操作仅对 Operator/Admin 开放，并受运行时只读模式约束；审计记录模板名称、规格和 Cloud-init 字节数，不保存用户数据正文。

PCI/USB 直通使用 libvirt 节点设备名作为稳定标识。PCI 设备写入 `<hostdev type='pci'>` 和 PCI 地址，USB 设备写入 vendor/product ID 及可选 bus/device 地址；`managed='yes'` 允许 libvirt 管理宿主机驱动解绑。运行中的持久域使用 `AFFECT_LIVE|AFFECT_CONFIG`，临时运行域只修改 live XML，关机持久域只修改 config XML；热插拔不受宿主 QEMU/guest OS 支持时返回 `409 ACTION_NOT_ALLOWED`。附件前服务层会扫描全部虚拟机，阻止同一设备重复分配。审计只记录设备名、类型、厂商/产品 ID 和地址，不涉及客户机数据。

定时快照策略保存在 SQLite `snapshot_policies` 表，虚拟机与快照状态仍以 libvirt 为唯一事实来源。策略名称在单台虚拟机内唯一，仅允许字母、数字、点、下划线和短横线；每台虚拟机最多 10 个策略。内置调度器随应用启动，每分钟查询到期且启用的策略并串行执行，按 `策略名-UTC时间戳` 创建快照；快照描述包含 `[policy:<id>]` 标记，保留清理只删除同一策略标记下超出保留数量的快照。执行失败会把错误写入策略并在下一间隔继续尝试，不会形成紧凑重试循环。策略 CRUD 与执行均校验 Operator/Admin 权限和运行时只读模式，并写入审计。

虚拟网络写操作仅对 Operator/Admin 开放。创建请求支持 `nat` / `isolated` 模式、IPv4 子网、可选 DHCP 起止地址、autostart 和创建后启动；子网至少为 `/29`，网关和默认 DHCP 范围由后端推导。编辑接口以网络名称作为不可变标识，只允许修改未激活的 NAT/隔离网络，可调整模式、IPv4 子网、DHCP 范围和 autostart；底层保留原有 DHCP host 等未编辑的 XML 配置。活跃网络必须先停止才能修改整网配置或删除，但静态 DHCP 主机条目是独立资源，活跃时也可通过 `virNetworkUpdate` 同时更新 live 和持久配置，无需中断网络。静态主机校验 IPv4 属于网络子网且避开网关、网络地址和广播地址，并拒绝重复 MAC/IP。桥接、路由等其他转发类型若没有 IPv4 DHCP 段则不支持静态主机。网络名称重复返回 `409 CONFLICT`，活跃网络删除返回 `409 CONFLICT`。所有网络写操作记录 `network.create`、`network.update`、`network.dhcp.host.add`、`network.dhcp.host.delete`、`network.start`、`network.stop` 或 `network.delete` 审计。

网络列表可展开查看 DHCP 租约，数据直接来自 libvirt `GetDHCPLeases`，包含主机名、IP、MAC、接口、客户端标识和到期时间。未激活网络返回空租约列表，租约读取不写审计。

## 测试

后端与前端单元测试：

```bash
make test
```

真实 libvirt 集成测试（只读与 cloud-init 临时资源闭环）：

```bash
VIRTMGR_TEST_REAL_LIBVIRT=1 go test ./internal/platform/libvirt -run TestRealLibvirtReadOnly -v
VIRTMGR_TEST_REAL_LIBVIRT=1 go test ./internal/platform/libvirt -run TestRealLibvirtCloudInitCreate -v
```

Playwright E2E 严格串行执行。`make e2e` 会先跑只读场景，再跑独立端口上的可写 demo 场景：

```bash
make e2e
```

`web/playwright.config.ts` 与 `web/playwright.writable.config.ts` 均固定 `workers: 1`、`fullyParallel: false`、`maxFailures: 1`，测试文件使用 `test.describe.configure({ mode: "serial" })`。不要通过命令行覆盖这些串行约束；该环境曾因大批量并发 Playwright 导致物理机异常。

## 目录

详细设计见 [`docs/architecture-design.md`](docs/architecture-design.md)。后端采用 `internal/modules/<domain>/{app,http}` 与 `internal/platform/<infra>` 分层，`bootstrap` 负责依赖装配。

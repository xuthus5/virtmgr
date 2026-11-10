# VirtMgr 设计方案

面向单台物理机 KVM/libvirt 实例的 Web 虚拟机管理平台。

| 项目 | 内容 |
|---|---|
| 产品名 | VirtMgr Console |
| 技术栈 | Go + React + TypeScript + shadcn/ui + Tailwind CSS |
| 当前版本范围 | 单 libvirt 端点、单物理机、多虚拟机 |
| 视觉参考 | `/root/projects/dosh` 的布局、密度、主题变量与状态表达 |
| 事实来源 | libvirt 是虚拟机、存储池、网络、快照等基础设施状态的唯一事实来源 |

## 1. 产品定位与边界

### 1.1 目标

VirtMgr 用于在浏览器中完成一台 KVM/libvirt 宿主机上的日常虚拟机运维：

- 查看宿主机 CPU、内存、存储、网络和虚拟机健康度。
- 创建、启动、关机、重启、暂停、恢复、强制下电和删除虚拟机。
- 查看虚拟机硬件、磁盘、网卡、IP、快照、事件和运行指标。
- 通过 VNC 或串口控制台进入虚拟机。
- 管理 libvirt 存储池、存储卷、ISO 和虚拟网络。
- 对所有写操作进行权限校验、危险操作确认和审计。

### 1.2 首版不做

- 不做跨多台物理机的集群调度或迁移编排。首版固定连接一个 libvirt 端点；未来多主机能力通过独立的 host registry/agent 扩展。
- 不替代 libvirt，不自行维护一份“虚拟机状态数据库”。
- 不提供任意 Shell 执行、任意宿主机文件浏览或任意 libvirt API 代理。
- 不在首版实现 SPICE、远程桌面协议转换、GPU 虚拟化和复杂 PCI 拓扑编辑；当前仅提供单设备 PCI/USB 直通与移除。
- 不做 Kubernetes 资源语义映射；只借鉴 dosh 的壳层、表格、状态和主题模式。

### 1.3 设计原则

1. **libvirt 权威**：页面展示和操作都从 libvirt 实时读取；SQLite 只保存用户、会话、任务、审计和少量本地偏好。
2. **服务端判定能力**：虚拟机当前允许哪些操作由后端根据 libvirt 状态计算，前端不硬编码状态机。
3. **危险操作显式化**：强制下电、删除虚拟机、删除磁盘、回滚快照必须二次确认，必要时输入名称。
4. **默认可观测**：长任务、错误、事件和审计都有明确落点，不把失败静默成 toast。
5. **单二进制交付**：前端通过 `go:embed` 打入 Go 二进制；默认连接本机 `/var/run/libvirt/libvirt-sock`。

### 1.4 当前落地状态（MVP）

当前实现已完成第一条可运行闭环：

- 本地管理员登录、Argon2id 密码、HttpOnly 会话、CSRF token。
- SQLite 使用 `modernc.org/sqlite`，保存用户、会话、任务表、审计、设置和定时快照策略；虚拟机与快照状态不落库。
- libvirt 只读能力：宿主机信息、虚拟机列表、详情、磁盘、网卡、VNC 图形信息和当前允许动作。
- 物理主机页读取宿主机、存储池、虚拟网络和虚拟机分配汇总，并展示异常提醒与容量使用率。
- 物理主机设备：`GET /host/devices` 读取 libvirt 节点设备，解析 PCI、USB、网络、块设备和 DRM 能力；主机页展示设备地址、厂商/型号、驱动和在线状态。
- 创建虚拟机：名称、vCPU、内存、BIOS/UEFI 固件、Secure Boot、磁盘路径/格式、虚拟网络、可选 ISO、autostart 和创建后启动。磁盘可从活跃存储池的现有 qcow2/raw 卷选择，网络只列出活跃虚拟网络。UEFI 使用 libvirt firmware 自动选择，Secure Boot 仅允许与 UEFI 组合；固件与安全启动状态进入创建审计，并从运行态域 XML 回读。
- Cloud-init：创建和模板配置时可选提供 NoCloud `user-data` 与目标活跃存储池。前端提供可视化字段（软件包、启动命令、时区/Locale、sudo/SSH 用户）并实时生成 `#cloud-config` YAML，同时保留带语法高亮和校验的高级源码模式。后端生成卷标 `cidata`、包含 `meta-data`/`user-data` 的 ISO，通过带 `serial=cloud-init` 标记的只读 CD-ROM 挂载；用户数据限制 64 KiB，审计仅记录启用状态、池和字节数，不记录内容。
- 虚拟机模板：SQLite `vm_templates` 保存可复用的 vCPU、内存、磁盘格式、网络、固件、Secure Boot、Cloud-init、ISO 路径和启动策略。新建虚拟机表单可选择模板预填，但实例名称和磁盘路径始终由操作者填写。模板 CRUD 仅对 Operator/Admin 开放，受全局只读模式约束并记录 `vm_template.create/update/delete` 审计。
- PCI/USB 直通：`POST /vms/{uuid}/devices` 与 `DELETE /vms/{uuid}/devices/{device}` 使用 libvirt 节点设备名分配或移除主机设备。PCI 以地址匹配，USB 以 vendor/product ID 和 bus/device 匹配；服务层阻止同一设备跨虚拟机重复分配，并记录 `vm.device.attach` / `vm.device.detach` 审计。
- 定时快照策略：SQLite `snapshot_policies` 保存间隔、保留数量和启用状态；应用内调度器每分钟执行到期策略，按 `[policy:<id>]` 描述标记创建并清理本策略快照。CRUD、立即执行、权限/只读校验和审计已实现。
- 克隆虚拟机：仅允许关机/异常停止的持久域，通过独立 libvirt 连接复制存储卷，重写域 XML 的 name、UUID、磁盘来源、MAC、NVRAM 和 VNC 端口。默认复制到源卷所在活跃池，可指定 `targetPool`；克隆以 `vm.clone` 异步任务执行，失败回滚新建卷和域，成功/失败写入审计。
- 虚拟机冷备份：`POST /vms/{uuid}/backups` 仅允许关机/异常停止的持久域，将 inactive 持久配置 XML、版本化 manifest 和可写 `file` 磁盘复制到指定活跃存储池。备份以 `vm.backup` 异步任务执行并更新真实进度；失败删除本次已创建但未完成的卷，成功/失败均写入审计，不包含内存状态。
- Guest Agent：运行中/阻塞状态的虚拟机详情按需调用 `guest-get-host-name` 和 `guest-network-get-interfaces`，返回主机名与去重后的 IPv4/IPv6 地址，并标记 `agentConnected`。单次查询 1 秒超时；列表查询使用独立 libvirt 连接、最多 4 个并发和 2 秒批次预算，查询失败降级为空信息，不阻塞列表主流程。
- 删除虚拟机：仅允许已关机或异常停止的域；删除域定义并保留磁盘，要求输入名称二次确认。
- 生命周期写能力：`start`、`shutdown`、`reboot`、`reset`、`pause`、`resume`、`destroy`。
- 开机自启动写能力：详情页通过 `PATCH /vms/{uuid}/autostart` 切换持久域的 autostart；临时域、Viewer 和全局只读模式拒绝修改，成功/失败均记录 `vm.autostart` 审计。
- CPU / 内存写能力：详情页通过 `PATCH /vms/{uuid}/resources` 调整持久域规格。运行、阻塞、暂停和挂起域使用 `SetVcpusFlags` / `SetMemoryFlags` 的 `LIVE|CONFIG`，目标不能超过当前活动最大值；关机或异常停止域使用 `CONFIG`，扩容时先更新持久最大值。临时域、不支持的瞬态状态、Viewer 和只读模式拒绝修改，成功/失败均记录 `vm.resources.update` 审计。
- 磁盘扩容写能力：详情页通过 `PATCH /vms/{uuid}/disks/{target}` 扩容可写文件型 qcow2/raw 数据盘，请求体为字节容量。仅支持扩大、不支持缩容；容量通过 `Domain.GetBlockInfo` 回读，运行域使用 `Domain.BlockResize`，关机域优先使用 `StorageVol.Resize`，目录卷内未单独注册的嵌套文件使用临时目录池调整后清理。CD-ROM、只读盘、非文件后端、临时域、Viewer 和只读模式拒绝修改，成功/失败均记录 `vm.disk.resize` 审计。宿主机容量扩展不替代客户机内分区/文件系统扩容。
- 服务端按 `viewer` / `operator` / `admin` 和全局只读模式判定权限，动作按 libvirt 当前状态校验并写审计。
- 用户管理：管理员可创建本地账号、调整角色、禁用/启用用户；用户写操作同样记录审计，只读模式拒绝用户写操作。
- 存储能力：存储池状态、类型、目标路径、容量，目录池（dir）创建、启动/停止、刷新和未激活池定义删除，活跃池的卷列表，创建 qcow2/raw 卷，流式上传 ISO，以及删除未被虚拟机引用的卷。
- 虚拟网络管理：网络模式、桥接口、IPv4/IPv6 地址、DHCP 范围、实时 DHCP 租约和自动启动状态；Operator/Admin 可创建 NAT/隔离网络、启停网络并删除未激活网络。模式、地址推导和删除约束由后端校验，写操作记录审计。
- 实时事件：进程内事件总线保留最近 500 条事件，真实 libvirt 后端注册域生命周期回调并运行默认事件循环；demo 后端生成 VM、快照、存储和网络事件。`GET /events` 提供快照，`GET /events/stream` 通过 SSE 推送初始快照、实时增量和 15 秒心跳。
- 任务中心基础：SQLite `tasks` 表提供持久化任务列表、状态筛选、分页和单任务查询；启动时将遗留的排队/运行任务标记为 `interrupted`。前端每 5 秒刷新，展示进度、步骤、失败原因和关联虚拟机。ISO 上传已作为第一个真实生产者，在流式读取时更新任务进度。
- 前端包含总览、虚拟机列表、虚拟机详情、动作按钮、只读提示和危险动作二次确认；虚拟机详情可切换查看运行态/持久配置 XML 并复制。
- 控制台：Operator/Admin 可在虚拟机详情内嵌或独立窗口中打开 noVNC 与 xterm.js；后端申请一次性短时效 token，通过 WebSocket 将浏览器二进制帧桥接到 libvirt `OpenGraphicsFD` 或 console stream，并在打开时记录审计。
- 域 XML：`GET /api/v1/vms/{uuid}/xml?format=live|config` 默认 live，使用 `DOMAIN_XML_INACTIVE` 读取 config；Admin 可通过 `PUT /api/v1/vms/{uuid}/xml` 在可写模式编辑持久配置，服务端锁定名称/UUID、拒绝 QEMU hook 命名空间，并以 `DOMAIN_DEFINE_VALIDATE` 校验后定义，审计不保存 XML 正文。
- 虚拟机配置导出：`GET /api/v1/vms/{uuid}/export` 返回稳定版本化的 JSON 包，包含虚拟机详情、持久配置 XML、活动域 live XML、快照和磁盘/网络元数据；只读能力不受 Viewer 或全局只读模式限制，但记录 `vm.export` 审计，不包含磁盘镜像数据。
- Playwright 使用只读和可写两套严格串行配置；可写场景只连接有状态 demo 后端，不对真实虚拟机执行写操作。

P0 控制台闭环已落地：`POST /vms/{uuid}/console-token` 生成 30 秒有效、单次使用且绑定用户/VM 的 token，`GET /vms/{uuid}/console` 完成会话校验、token 消费、WebSocket 升级和 libvirt graphics FD 双向桥接；demo 后端提供最小 RFB 握手，供严格串行 Playwright 验证浏览器连接。存储卷创建/删除和 ISO 上传已实现；ISO 使用 `application/octet-stream` 请求体直接写入 libvirt 流，要求 `Content-Length`、`.iso` 文件名和 1 MiB - 16 GiB 大小，失败时删除未完成卷。删除前会扫描全部域 XML 的磁盘来源，仍被引用的卷返回 `409 VOLUME_IN_USE`。未激活存储池不能枚举、创建、上传或删除卷，后端返回 `503 UNAVAILABLE`。虚拟网络创建/启停/删除已实现：支持 `nat` / `isolated` 模式和 IPv4 子网，子网至少为 `/29`，活跃网络必须先停止才能删除。实时事件已实现内存事件总线、libvirt 生命周期采集和 SSE 推送；事件不属于长期审计。设计文档其余部分保留为目标架构与后续阶段约束。

## 2. 总体架构

```text
┌──────────────────────────────── Browser ────────────────────────────────┐
│ React 19 + TypeScript + TanStack Router/Query/Table + shadcn/ui        │
│ 页面：总览 / 虚拟机 / 存储 / 网络 / 物理主机 / 任务 / 事件 / 审计 / 设置 │
└───────────────────────────────┬─────────────────────────────────────────┘
                                │ HTTPS REST + SSE + WebSocket
┌───────────────────────────────▼─────────────────────────────────────────┐
│                         VirtMgr Go 服务                                 │
│ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌───────────────────┐ │
│ │ HTTP/API    │ │ Auth/RBAC   │ │ Audit       │ │ Task Manager      │ │
│ │ chi + OpenAPI│ │ Session/CSRF│ │ SQLite/Slog │ │ 有界 worker pool  │ │
│ └──────┬──────┘ └─────────────┘ └─────────────┘ └─────────┬─────────┘ │
│        │                                                   │           │
│ ┌──────▼───────────────────────────────────────────────────▼─────────┐ │
│ │                    libvirt Adapter / Service Layer                 │ │
│ │ Domain · StoragePool/Volume · Network · Snapshot · Event · Stats   │ │
│ └──────────────────────────────────┬─────────────────────────────────┘ │
└────────────────────────────────────┼───────────────────────────────────┘
                                     │ libvirt C API / UNIX socket
┌────────────────────────────────────▼───────────────────────────────────┐
│                         KVM / libvirtd / QEMU                           │
│ Domains · qcow2/raw volumes · bridges/NAT · snapshots · VNC socket      │
└────────────────────────────────────────────────────────────────────────┘
```

### 2.1 项目目录结构

后端沿用 dosh 的 `modules/<domain>/{app,http}` + `platform/<infra>` 结构。`app` 放业务和仓储接口，`http` 只做协议转换，`platform` 提供 libvirt、SQLite 等基础设施实现。

```text
virtmgr/
├── api/
│   └── openapi.yaml
├── cmd/
│   └── virtmgr/
│       └── main.go
├── internal/
│   ├── bootstrap/
│   │   ├── app.go                依赖装配、启动与关闭
│   │   ├── router.go             中间件和模块路由挂载
│   │   ├── spa.go                go:embed 前端与 SPA fallback
│   │   ├── app_test.go
│   │   └── routes_test.go
│   ├── domain/
│   │   ├── types.go              公共查询、分页、能力类型
│   │   ├── errors.go             领域错误与 HTTP 映射依据
│   │   ├── vm.go
│   │   ├── lifecycle.go
│   │   ├── snapshot.go
│   │   ├── storage.go
│   │   ├── network.go
│   │   ├── host.go
│   │   ├── metrics.go            资源计数、宿主机/虚拟机指标模型
│   │   ├── event.go
│   │   ├── task.go
│   │   ├── audit.go
│   │   └── types_test.go
│   ├── modules/
│   │   ├── auth/
│   │   │   ├── app/
│   │   │   │   ├── service.go
│   │   │   │   ├── session.go
│   │   │   │   ├── password.go
│   │   │   │   └── *_test.go
│   │   │   └── http/
│   │   │       ├── handler.go
│   │   │       └── handler_test.go
│   │   ├── overview/
│   │   │   ├── app/service.go
│   │   │   └── http/handler.go
│   │   ├── vms/
│   │   │   ├── app/
│   │   │   │   ├── service.go     查询、详情和能力计算
│   │   │   │   ├── batch.go       批量生命周期编排与逐台结果
│   │   │   │   ├── create.go      创建规格到 libvirt XML
│   │   │   │   ├── lifecycle.go   start/shutdown/reboot/pause/destroy
│   │   │   │   ├── snapshot.go
│   │   │   │   ├── metadata.go
│   │   │   │   ├── validate.go
│   │   │   │   └── *_test.go
│   │   │   └── http/
│   │   │       ├── handler.go
│   │   │       └── handler_test.go
│   │   ├── console/
│   │   │   ├── app/service.go     一次性 token、VNC/串口连接编排
│   │   │   └── http/
│   │   │       ├── handler.go
│   │   │       └── websocket.go
│   │   ├── storage/
│   │   │   ├── app/
│   │   │   │   ├── service.go
│   │   │   │   ├── write.go
│   │   │   │   ├── upload.go
│   │   │   │   └── *_test.go
│   │   │   └── http/handler.go
│   │   ├── networks/
│   │   │   ├── app/
│   │   │   │   ├── service.go
│   │   │   │   ├── write.go
│   │   │   │   └── *_test.go
│   │   │   └── http/handler.go
│   │   ├── host/
│   │   │   ├── app/service.go
│   │   │   └── http/handler.go
│   │   ├── events/
│   │   │   ├── app/service.go
│   │   │   └── http/handler.go
│   │   ├── tasks/
│   │   │   ├── app/service.go
│   │   │   └── http/handler.go
│   │   ├── audithistory/
│   │   │   ├── app/service.go
│   │   │   └── http/handler.go
│   │   └── settings/
│   │       ├── app/service.go
│   │       └── http/handler.go
│   ├── platform/
│   │   ├── runtimeconfig/
│   │   │   └── mode.go           持久化运行时模式与并发安全读取
│   │   ├── libvirt/
│   │   │   ├── backend.go        连接、重连、版本和健康检查
│   │   │   ├── domain.go         Domain 列表/详情/定义/删除
│   │   │   ├── lifecycle.go      生命周期动作
│   │   │   ├── snapshot.go
│   │   │   ├── storage.go
│   │   │   ├── network.go
│   │   │   ├── stats.go          批量 domain stats 与缓存
│   │   │   ├── events.go         libvirt 事件订阅
│   │   │   ├── devices.go        节点设备枚举与 XML 解析
│   │   │   ├── console.go        graphics/console 通道
│   │   │   ├── xml/              XML 生成、解析和兼容性校验
│   │   │   └── fake/             测试用 Backend 实现
│   │   ├── metrics/
│   │   │   └── service.go        前后计数差分、CPU/网络速率计算
│   │   ├── sqlite/
│   │   │   ├── db.go             连接、PRAGMA、连接池
│   │   │   ├── migrate.go        嵌入式迁移与版本检查
│   │   │   ├── migrations/
│   │   │   │   └── 0001_init.sql
│   │   │   ├── users.go
│   │   │   ├── sessions.go
│   │   │   ├── tasks.go
│   │   │   ├── audit.go
│   │   │   ├── settings.go
│   │   │   └── *_test.go
│   │   ├── authz/
│   │   │   └── policy.go         角色到能力的集中映射
│   │   ├── audit/
│   │   │   └── logger.go         审计事件组装与脱敏
│   │   ├── eventbus/
│   │   │   └── bus.go            进程内环形事件总线与有界订阅
│   │   ├── tasks/
│   │   │   └── recorder.go       任务创建、进度和终态记录
│   │   ├── httpserver/
│   │   │   ├── server.go
│   │   │   └── middleware.go
│   │   ├── httpx/
│   │   │   └── httpx.go          JSON、错误、请求解析
│   │   ├── config/
│   │   │   └── config.go
│   │   └── requestid/
│   │       └── requestid.go
│   ├── version/
│   │   └── version.go
│   └── performance/
│       └── capacity_test.go
├── web/
│   ├── embed.go                   //go:embed dist
│   ├── index.html
│   ├── package.json
│   ├── package-lock.json
│   ├── components.json
│   ├── tailwind.config.ts
│   ├── postcss.config.js
│   ├── vite.config.ts
│   ├── vitest.config.ts
│   ├── playwright.config.ts
│   ├── tsconfig*.json
│   ├── e2e/
│   └── src/
│       ├── api/
│       │   ├── client.ts
│       │   ├── schema.d.ts        OpenAPI 生成
│       │   └── types.ts
│       ├── components/
│       │   ├── layout/
│       │   │   ├── app-shell.tsx
│       │   │   ├── page-header.tsx
│       │   │   └── host-status-card.tsx
│       │   ├── data/
│       │   │   ├── data-table.tsx
│       │   │   ├── detail-item.tsx
│       │   │   ├── metric-card.tsx
│       │   │   └── resource-bar.tsx
│       │   ├── feedback/
│       │   │   ├── states.tsx
│       │   │   └── status-badge.tsx
│       │   ├── resources/
│       │   │   ├── vm-actions-menu.tsx
│       │   │   ├── delete-vm-dialog.tsx
│       │   │   ├── metadata-card.tsx
│       │   │   └── yaml-dialog.tsx
│       │   └── ui/                shadcn 生成组件
│       ├── features/
│       │   ├── auth/
│       │   │   └── use-session.ts
│       │   ├── search/
│       │   │   ├── global-search.tsx
│       │   │   ├── search-model.ts
│       │   │   └── search-model.test.ts
│       │   ├── overview/
│       │   │   ├── overview-page.tsx
│       │   │   └── use-overview-query.ts
│       │   ├── vms/
│       │   │   ├── vms-page.tsx
│       │   │   ├── vm-detail-page.tsx
│       │   │   ├── vm-create-page.tsx
│       │   │   ├── vm-columns.tsx
│       │   │   ├── use-vms-query.ts
│       │   │   ├── use-vm-stream.ts
│       │   │   ├── components/
│       │   │   │   ├── vm-summary.tsx
│       │   │   │   ├── vm-lifecycle-actions.tsx
│       │   │   │   ├── vm-console-panel.tsx
│       │   │   │   ├── vm-hardware-tab.tsx
│       │   │   │   ├── vm-disks-tab.tsx
│       │   │   │   ├── vm-network-tab.tsx
│       │   │   │   ├── vm-snapshots-tab.tsx
│       │   │   │   ├── vm-xml-tab.tsx
│       │   │   │   └── vm-events-tab.tsx
│       │   │   └── dialogs/
│       │   │       ├── lifecycle-confirm-dialog.tsx
│       │   │       ├── delete-vm-dialog.tsx
│       │   │       ├── snapshot-dialog.tsx
│       │   │       └── vm-create-steps.tsx
│       │   ├── console/
│       │   │   ├── vnc-console-page.tsx
│       │   │   ├── serial-console-page.tsx
│       │   │   └── no-vnc-viewport.tsx
│       │   ├── storage/
│       │   │   ├── storage-page.tsx
│       │   │   ├── storage-pool-detail-page.tsx
│       │   │   ├── storage-columns.tsx
│       │   │   └── volume-upload-dialog.tsx
│       │   ├── networks/
│       │   │   ├── networks-page.tsx
│       │   │   ├── network-detail-page.tsx
│       │   │   └── network-columns.tsx
│       │   ├── host/
│       │   │   ├── host-page.tsx
│       │   │   ├── host-devices-panel.tsx
│       │   │   └── use-host-query.ts
│       │   ├── events/
│       │   │   ├── events-page.tsx
│       │   │   ├── event-columns.tsx
│       │   │   └── use-event-stream.ts
│       │   ├── tasks/
│       │   │   ├── tasks-page.tsx
│       │   │   └── use-task-stream.ts
│       │   ├── audit/
│       │   │   └── audit-page.tsx
│       │   └── settings/
│       │       └── settings-page.tsx
│       ├── lib/
│       │   ├── theme.tsx
│       │   ├── capabilities.ts
│       │   ├── utils.ts
│       │   └── list-search.ts
│       ├── routes/
│       │   └── login-page.tsx
│       ├── test/
│       │   └── setup.ts
│       ├── index.css
│       ├── main.tsx
│       └── router.tsx
├── deploy/
│   ├── systemd/virtmgr.service
│   ├── docker/Dockerfile
│   └── docker/compose.yaml
├── docs/
├── scripts/
├── Makefile
├── go.mod
├── go.sum
└── README.md
```

### 2.2 模块内部约定

每个业务模块保持与 dosh 相同的轻量分层：

- `app/service.go`：业务编排和只读能力；只依赖 `domain` 与模块内定义的 `Repository` 接口。
- `app/write.go`、`app/lifecycle.go` 等：按职责拆分写操作，不按一个接口一个文件机械拆分。
- `http/handler.go`：解析请求、调用 `app`、写入 JSON；不包含业务判断和 libvirt 调用。
- `http/handler_test.go`：使用 fake app/repository 验证状态码、错误格式和权限边界。
- `*_test.go` 与实现文件同目录，沿用 dosh 的测试组织方式。
- 单个文件超过约 300 行或出现明显独立职责时再拆分，避免过早碎片化。

跨模块约束：

```text
cmd                 -> bootstrap
bootstrap           -> modules/*/http, platform/*
modules/*/http      -> modules/*/app, platform/httpx, platform/authz
modules/*/app       -> domain（不直接依赖 platform/libvirt）
platform/libvirt    -> domain（实现各模块的 Repository 接口）
platform/sqlite     -> domain（实现用户、会话、任务、审计存储）
domain              -> 标准库
```

业务模块不能直接 import 另一个模块的 `http` 包；跨域聚合通过 `bootstrap` 注入的能力或 `platform` 层完成。

### 2.3 libvirt 接入选型

推荐使用 `libvirt.org/go/libvirt` CGO 绑定：

- 与 libvirt 的状态、错误码、事件和 XML 语义保持一致。
- 支持 `DomainOpenConsole`、`DomainOpenGraphics`、快照、域统计等控制台和可观测能力。
- 比拼接 `virsh` 命令更安全、可测试，也避免命令注入。

代价是构建机需要 `libvirt-dev`，运行机需要 `libvirt0`，并且部署位置需要能访问 libvirt socket。推荐首版直接以 systemd 服务运行在宿主机上；容器部署时挂载 `/var/run/libvirt/libvirt-sock` 并共享 `libvirt` 组。

适配层必须定义接口，测试使用 fake，不依赖真实 libvirt：

```go
type Backend interface {
    ListDomains(ctx context.Context) ([]domain.VM, error)
    Domain(ctx context.Context, uuid string) (domain.VM, error)
    DomainActions(ctx context.Context, uuid string) ([]string, error)
    Start(ctx context.Context, uuid string) error
    Shutdown(ctx context.Context, uuid string, mode domain.ShutdownMode) error
    Destroy(ctx context.Context, uuid string) error
    DefineXML(ctx context.Context, xml string) error
    // storage, network, snapshot, stats, events...
}
```

### 2.4 状态与任务模型

- VM、存储池、网络、快照状态实时来自 libvirt，不写数据库副本。
- 创建 VM、克隆、冷备份、ISO 上传、快照回滚、批量操作等长任务落 SQLite `tasks` 表。当前已实现任务查询、状态恢复、页面轮询刷新，以及 ISO 流式上传、VM 克隆和虚拟机冷备份的真实进度更新；后续长任务生产者通过同一任务模型更新进度，SSE 任务流作为增强项。
- 短生命周期 libvirt 事件由事件采集器统一转换为业务事件，进入进程内事件总线，再通过 SSE 推送给前端；事件总线只保留最近 500 条，进程重启后清空。
- 前端 TanStack Query 只缓存查询结果；收到 SSE 后按资源 key 失效，不自行推测最终状态。

## 3. 技术栈

### 3.1 后端

| 类别 | 选型 |
|---|---|
| 语言 | Go，与 dosh 保持一致的新版本基线 |
| HTTP | `go-chi/chi/v5` |
| libvirt | `libvirt.org/go/libvirt`（CGO） |
| 日志 | `log/slog`，JSON 输出，携带 request ID |
| 数据库 | SQLite WAL（`modernc.org/sqlite`），只存用户、会话索引、任务、审计、设置 |
| API 契约 | OpenAPI 3.1 + 生成 TypeScript 类型 |
| 实时事件 | 进程内环形事件总线 + SSE；保留最近 500 条，控制台使用 WebSocket |
| 测试 | Go 单元测试 + fake backend + 测试 libvirtd 集成测试 |

### 3.2 前端

复用 dosh 的技术与组件模式：

- React 19、TypeScript、Vite。
- Tailwind CSS 3.4、shadcn/ui `new-york`、Radix UI。
- TanStack Router、TanStack Query、TanStack Table。
- Recharts 展示资源趋势。
- lucide-react 图标、sonner 通知。
- CodeMirror 展示和编辑 libvirt XML。
- 新增 `noVNC` 与 `xterm.js` 用于图形/串口控制台。
- 使用 OpenAPI 生成 API 类型，避免手写重复类型。

### 3.3 SQLite 落地

SQLite 固定使用 `modernc.org/sqlite`，通过标准库 `database/sql` 访问，不引入 ORM，也不增加第二个 SQLite 驱动。该驱动为纯 Go 实现，不新增 CGO 依赖；CGO 仅用于 libvirt 绑定。

实时事件不落 SQLite：事件总线提供当前进程内的短期运行视图，默认保留 500 条，慢订阅者丢弃新事件而不反向阻塞 libvirt。需要追责的写操作仍由 `audit_events` 持久化，二者不得混用。

```go
import (
	"database/sql"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	dsn := "file:" + path +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(1)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// 首版写路径简单且审计必须可靠，单连接可避免 SQLITE_BUSY 和写写竞争。
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	return db, nil
}
```

数据库默认位于 `/var/lib/virtmgr/virtmgr.db`，可通过 `VIRTMGR_DATABASE_PATH` 覆盖。迁移文件使用 `go:embed` 编译进二进制，由 `platform/sqlite/migrate.go` 在启动时按版本顺序执行。

首版表：

| 表 | 用途 |
|---|---|
| `schema_migrations` | 已执行迁移及校验和 |
| `users` | 本地用户、Argon2id 密码哈希、角色、禁用状态 |
| `sessions` | 会话 ID 哈希、用户、CSRF secret、创建/过期时间 |
| `tasks` | 长任务类型、目标、状态、步骤、进度、错误和操作者 |
| `audit_events` | 审计主体、动作、目标、来源 IP、结果、request ID 和 `detail_json` |
| `settings` | 只读模式、宿主机展示名等非 libvirt 设置 |
| `login_attempts` | 本地登录限流与锁定窗口 |
| `snapshot_policies` | 定时快照策略、调度时间、最近快照和失败原因 |

约定：

- VM、存储池、网络和快照不在 SQLite 建业务表，避免与 libvirt 状态双写。
- 手写 SQL，仓储方法接收 `context.Context`；单条写操作使用显式事务。
- 审计写入和任务状态更新必须检查错误；危险操作的审计写入失败时 fail-closed。
- 迁移只允许向前执行；发布前在临时数据库和已有数据库副本上分别验证。
- 备份使用 SQLite backup API 或 `VACUUM INTO`，不在运行中直接复制 WAL 数据库文件。
- 首版使用单连接保证写一致性；若审计查询压力显现，再拆为单写连接 + WAL 只读连接池，API 与仓储接口不变。

## 4. 领域模型

### 4.1 虚拟机

```ts
type VMState = "running" | "shutoff" | "paused" | "pmsuspended" | "crashed" | "starting" | "stopping" | "unknown";

type VM = {
  uuid: string;
  name: string;
  title?: string;
  description?: string;
  state: VMState;
  persistent: boolean;
  autostart: boolean;
  vcpu: { current: number; max: number };
  memory: { currentBytes: number; maxBytes: number };
  os: { type: string; arch: string; machine?: string };
  firmware: "bios" | "uefi";
  secureBoot: boolean;
  cloudInit: boolean;
  disks: Disk[];
  interfaces: Interface[];
  graphics?: Graphics;
  ips: string[];
  hostname?: string;
  agentConnected: boolean;
  uptimeSeconds?: number;
  snapshotCount: number;
  allowedActions: VMAction[];
  createdAt?: string;
};

type VMAction =
  | "start" | "shutdown" | "reboot" | "reset"
  | "pause" | "resume" | "destroy"
  | "snapshot" | "clone" | "delete" | "edit-xml";
```

### 4.2 存储

当前实现通过 libvirt Storage Pool/Volume API 读取存储池和卷，并支持目录池（dir）的生命周期：`StoragePoolDefineXML` 定义、启动/停止、`Refresh` 刷新和删除未激活定义。池名和绝对目标路径由服务层校验；删除只移除 libvirt 定义，不递归删除宿主机目录或卷文件。活跃池使用 `StorageVolCreateXML` 创建 qcow2/raw 卷，并在删除前扫描所有域的磁盘来源，确认没有虚拟机引用后才调用卷删除。整个流程不直接访问宿主机文件系统。创建请求包含卷名、格式和字节容量；卷名必须是安全文件名并以对应扩展名结尾。`StorageVolume` 暂未返回 `attachedTo`、`bootable` 等关联推导字段；占用信息在删除时实时校验，不落库。

```ts
type StoragePool = {
  name: string;
  type: "dir" | "logical" | "lvm" | "iscsi" | "rbd" | "zfs" | string;
  state: "active" | "inactive" | "degraded" | "unknown";
  autostart: boolean;
  targetPath?: string;
  capacityBytes: number;
  allocationBytes: number;
  availableBytes: number;
  volumeCount: number;
};

type StorageVolume = {
  name: string;
  pool: string;
  format: "qcow2" | "raw" | "iso" | string;
  capacityBytes: number;
  allocationBytes: number;
  path?: string;
  bootable: boolean;
  attachedTo: Array<{ uuid: string; name: string; target: string }>;
};
```

### 4.3 网络与快照

当前网络写能力通过 libvirt Network API 实现。创建时由后端根据 IPv4 子网推导网关、前缀和默认 DHCP 范围，可选 DHCP 起止地址必须位于子网内且避开网关；支持 NAT 转发与隔离网络，以及 autostart、创建后启动和失败回滚。编辑时网络名称不可变，仅允许未激活的持久 NAT/隔离网络；后端在原始 libvirt XML 上定点替换 forward、IPv4 address/prefix 和 DHCP range，并保留 DHCP host、IPv6、MAC 等其他元素，再通过 `NetworkDefineXML` 原子更新持久定义。编辑活跃网络返回 `409 NETWORK_ACTIVE`，桥接/路由等不支持的类型返回 `409 NETWORK_UPDATE_UNSUPPORTED`。静态 DHCP 主机条目作为独立资源管理：对激活的持久网络使用 `virNetworkUpdate` 的 `ADD_LAST/DELETE + IP_DHCP_HOST + LIVE|CONFIG`，对未激活网络使用 `CONFIG`，因此无需停止网络即可实时增删；服务层校验 MAC、IPv4 子网归属、保留地址和重复 MAC/IP。启停动作按当前网络状态校验，活跃网络删除返回 `409 CONFLICT`。网络状态始终来自 libvirt，不写入业务数据库；审计记录 `network.create`、`network.update`、`network.dhcp.host.add`、`network.dhcp.host.delete`、`network.start`、`network.stop`、`network.delete`。

```ts
type VirtualNetwork = {
  name: string;
  uuid: string;
  mode: "nat" | "route" | "bridge" | "isolated" | "open";
  state: "active" | "inactive";
  autostart: boolean;
  bridge?: string;
  ipv4?: { address: string; prefix: number; dhcpStart?: string; dhcpEnd?: string };
  leases: DHCPLease[];
};

type Snapshot = {
  name: string;
  description?: string;
  createdAt: string;
  state: "running" | "shutoff" | "paused" | "unknown";
  current: boolean;
  parent?: string;
  hasMemory: boolean;
};
```

### 4.4 定时快照策略

策略只保存调度元数据，快照本体和虚拟机状态仍由 libvirt 管理。每个策略属于一台虚拟机，名称在虚拟机内唯一；间隔限制为 5 分钟至 7 天，保留数量为 1-50，每台虚拟机最多 10 个策略。服务启动后由单实例调度器每分钟查询 `enabled = 1 AND next_run_at <= now` 的策略并串行执行，避免同一进程内并发创建快照。

```ts
type SnapshotPolicy = {
  id: number;
  vmUuid: string;
  name: string;
  intervalMinutes: number;
  retention: number;
  enabled: boolean;
  lastRunAt?: string;
  nextRunAt: string;
  lastSnapshot?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
};
```

执行时使用 `策略名-UTC时间戳` 作为快照名，并在描述中写入 `VirtMgr 定时快照策略 <name> [policy:<id>]`。保留清理只选择包含当前策略标记的快照，按创建时间倒序保留最新 N 份，因此手工快照和其他策略的快照不会被误删。创建失败会记录 `lastError` 并推进 `next_run_at`，避免紧密重试；清理失败会保留新快照、记录错误并继续调度。策略 CRUD 和执行均要求 Operator/Admin 且受运行时只读模式约束，分别记录 `vm.snapshot_policy.create/update/delete` 与 `vm.snapshot.schedule` 审计。

### 4.5 PCI / USB 直通

直通以 libvirt 节点设备名为稳定标识，领域模型只保存可从域 XML 回读的设备元数据。PCI 使用 `<hostdev mode='subsystem' type='pci' managed='yes'>` 和 `source/address`；USB 使用 vendor/product ID，并在可解析节点设备名时附带 bus/device，避免仅按型号匹配导致多设备歧义。宿主机驱动由 libvirt `managed='yes'` 处理，平台不直接写 sysfs。

```ts
type PassthroughDevice = {
  name: string;
  type: "pci" | "usb";
  vendor?: string;
  vendorId?: string;
  product?: string;
  productId?: string;
  address?: string;
  driver?: string;
};
```

写路径仅对 Operator/Admin 开放并受运行时只读模式约束。运行中的持久域使用 live+config 热插拔，临时运行域只修改 live XML，关机持久域只修改 config XML；libvirt/QEMU 不支持对应热插拔时映射为 `409 ACTION_NOT_ALLOWED`。服务层在附加前扫描全部虚拟机，拒绝跨虚拟机重复分配。附件/移除成功和失败均写审计，详情页回读域 XML 的 `hostdev` 条目展示实际分配结果。

### 4.6 自定义元数据

只保存 libvirt 不表达的少量信息，例如展示名称、备注和标签。优先写入域 XML 的自定义 metadata 命名空间，再由后端读取；SQLite 不保存可被 libvirt 推导出的状态。这样重建数据库不会丢失虚拟机业务信息。

## 5. API 设计

所有接口位于 `/api/v1`，JSON 使用 camelCase。错误统一返回：

```json
{
  "error": {
    "code": "VM_STATE_CONFLICT",
    "message": "当前状态不允许执行该操作",
    "requestId": "req_...",
    "detail": { "state": "shutoff", "action": "reboot" }
  }
}
```

### 5.1 会话与能力

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET` | `/session` | 当前用户、角色、只读模式、能力列表 |
| `POST` | `/session` | 登录 |
| `DELETE` | `/session` | 退出并销毁会话 |
| `GET` | `/capabilities` | 前端按钮级能力 |
| `GET` | `/users` | 本地用户列表（管理员） |
| `POST` | `/users` | 创建本地用户（管理员，需 CSRF） |
| `PATCH` | `/users/{id}` | 更新角色和启用状态（管理员，需 CSRF） |

### 5.2 总览与宿主机

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET` | `/overview` | 宿主机摘要、VM 计数、资源总量、最近事件 |
| `GET` | `/host` | CPU 型号/核数、内存、内核、libvirt/QEMU 版本 |
| `GET` | `/host/devices` | PCI、USB、网络、块设备和 DRM 节点设备（已实现） |
| `GET` | `/host/metrics` | 宿主机 CPU、内存、物理网卡当前采样与相邻采样速率（已实现） |

`/host/devices` 当前已实现：后端枚举 `pci`、`usb`、`usb_device`、`net`、`storage`、`drm` 能力，读取设备 XML、父设备、能力、在线和持久状态。设备热插拔导致单个设备查找或 XML 解析失败时跳过该设备；存在部分结果时仍返回其余设备，全部失败时返回错误。

`/host/metrics` 和 `/vms/{uuid}/metrics` 每次请求读取 libvirt 累计计数，并由进程内采样服务与上一请求对比计算 CPU 使用率和网络字节速率。首次采样速率为 0；计数器回退时速率降级为 0；CPU 使用率限制在 0-100%。宿主机网卡计数来自 `/proc/net/dev` 的物理接口，虚拟机网卡计数汇总域 XML 中各接口 target 设备。该实现不在 SQLite 保存历史，前端每 2 秒轮询并保留最近 30 个点，符合首版单进程部署模型。

### 5.3 虚拟机

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET` | `/vms` | 列表，支持 `q/state/pool/network/tag` 过滤 |
| `POST` | `/vms` | 按结构化配置创建虚拟机 |
| `GET` | `/vms/{uuid}` | 详情 |
| `PATCH` | `/vms/{uuid}/autostart` | 启停持久域 autostart，请求体 `{ "enabled": boolean }`（已实现） |
| `PATCH` | `/vms/{uuid}/resources` | 调整 vCPU 与内存，请求体 `{ "vcpus": number, "memoryMiB": number }`（已实现，需 CSRF） |
| `PATCH` | `/vms/{uuid}/disks/{target}` | 扩容 qcow2/raw 数据盘，请求体 `{ "capacityBytes": number }`（已实现，需 CSRF） |
| `POST` | `/vms/{uuid}/actions/{action}` | 白名单动作：start/shutdown/reboot/reset/pause/resume/destroy |
| `POST` | `/vms/{uuid}/devices` | 直通 PCI/USB 主机设备，请求体 `{ "device": "pci_0000_3b_00_0" }`（已实现） |
| `DELETE` | `/vms/{uuid}/devices/{device}` | 移除主机设备直通（已实现） |
| `POST` | `/vms/batch-actions` | 批量生命周期动作；请求体 `{ "action": "...", "uuids": ["..."] }`，最多 100 台，返回逐台成功或失败结果（已实现） |
| `GET` | `/vms/{uuid}/xml?format=live|config` | 只读 XML |
| `GET` | `/vms/{uuid}/export` | 下载配置与元数据 JSON 包（已实现） |
| `PUT` | `/vms/{uuid}/xml` | Admin 编辑持久 XML，先 validate 再 define（已实现） |
| `GET` | `/vms/{uuid}/metrics` | vCPU、内存和虚拟网卡的当前采样与相邻采样速率（已实现） |
| `GET` | `/vms/{uuid}/snapshots` | 快照列表 |
| `POST` | `/vms/{uuid}/snapshots` | 创建快照 |
| `POST` | `/vms/{uuid}/snapshots/{name}/revert` | 回滚快照 |
| `DELETE` | `/vms/{uuid}/snapshots/{name}` | 删除快照 |
| `GET` | `/vms/{uuid}/snapshot-policies` | 定时快照策略列表（已实现） |
| `POST` | `/vms/{uuid}/snapshot-policies` | 创建定时快照策略（已实现，需 CSRF） |
| `PATCH` | `/snapshot-policies/{id}` | 更新或启停策略（已实现，需 CSRF） |
| `DELETE` | `/snapshot-policies/{id}` | 删除策略定义（已实现，需 CSRF；不删除已有快照） |
| `POST` | `/snapshot-policies/{id}/run` | 立即执行策略并返回新快照（已实现，需 CSRF） |
| `POST` | `/vms/{uuid}/clone` | 克隆关机/异常停止的持久域为异步任务，返回 `202` 和任务对象（已实现） |
| `POST` | `/vms/{uuid}/backups` | 冷备份关机/异常停止的持久域到活跃存储池，返回 `202` 和 `vm.backup` 任务（已实现，需 CSRF） |
| `DELETE` | `/vms/{uuid}?disks=keep|delete` | 删除域和可选磁盘 |
| `POST` | `/vms/{uuid}/console-token` | 申请一次性控制台令牌，`kind` 为 `vnc` 或 `serial` |
| `GET` | `/vms/{uuid}/console?token=...` | 升级为 VNC WebSocket 并桥接 libvirt graphics FD |
| `GET` | `/vms/{uuid}/serial-console?token=...` | 升级为串口 WebSocket 并桥接 libvirt console stream |
| `POST` | `/vms/{uuid}/console/send-key` | 向运行中的域发送 Ctrl+Alt+Del、Backspace 或 F1-F12 组合键（需 CSRF） |

动作接口必须由后端基于当前状态返回 409 `VM_STATE_CONFLICT`，而不是只靠前端隐藏按钮。

`POST /vms` 当前支持 `firmware: "bios" | "uefi"` 和 `secureBoot: boolean`；省略或空值规范化为 `bios`，未知固件以及 `secureBoot=true` + BIOS 返回 `400 INVALID_INPUT`。UEFI 域使用 `<os firmware='efi'>` 与 `<loader secure='yes|no'/>`，由 libvirt 结合宿主机 firmware 配置选择 OVMF 代码和 NVRAM 模板；平台不硬编码发行版固件路径。

创建请求还可携带 `cloudInit: { "userData": "...", "pool": "default" }`。后端校验用户数据非空、无 NUL 且不超过 64 KiB，目标池必须是活跃 pool；随后通过 libvirt Storage Volume API 创建 `<vm-name>-cidata.iso` 并上传纯 Go 生成的 NoCloud ISO。域 XML 以只读 CD-ROM 和 `<serial>cloud-init</serial>` 挂载，虚拟机详情从运行态 XML 回读 `cloudInit` 状态。seed 创建或域定义失败时回滚卷和域，避免留下只完成一半的资源。

批量动作接口对 UUID 去重后逐台复用单机动作校验和审计。单台失败只记录在该结果项中，不中断其余虚拟机；整体权限不足或只读模式仍直接拒绝，并记录 `vm.batch` 失败审计。前端列表提供全选、多选、动作选择和危险动作确认，结果区展示成功数量及失败原因。

虚拟机配置导出使用 `virtmgr.vm-export/v1` 格式版本，JSON 字段包含 `formatVersion`、`exportedAt`、`vm`、`configXml`、可选 `liveXml`、`snapshots` 和 `disclaimer`。服务端始终读取持久配置；仅当域处于 running、blocked、paused、shutting-down 或 suspended 时请求 live XML，避免对 inactive 域请求运行态定义。导出不生成宿主机文件，HTTP 响应通过 `Content-Disposition: attachment` 交给浏览器保存；审计只记录虚拟机名称、是否包含 live XML、快照数量和格式版本，不保存 XML 正文。该操作为只读语义，因此 Viewer 和全局只读模式均可执行，但成功/失败都会写入 `vm.export` 审计。

管理员 XML 编辑只接受持久域和 `config` 语义：请求体为 `{ "xml": "..." }`，上限 512 KiB，根元素必须是 `domain`，其中名称和 UUID 必须与目标虚拟机一致。服务端拒绝 `http://libvirt.org/schemas/domain/qemu/1.0` 命名空间，避免通过 `<qemu:commandline>` 等扩展注入宿主命令；随后调用 `DomainDefineXMLFlags(..., DOMAIN_DEFINE_VALIDATE)`，校验失败不更新定义。该操作需要 Admin、CSRF 和可写模式，只修改持久定义，不启动或重启域；运行中的域在下次启动时使用新定义。成功审计记录名称、状态、字节数、SHA-256 和格式版本，失败同样审计，但不保存 XML 正文。

虚拟机冷备份只接受 `shutoff` 或 `crashed` 状态且持久化的域，目标必须是活跃存储池。请求体为 `{ "name": "...", "targetPool": "..." }`，备份名需满足安全文件名规则；所有可写 `file` 磁盘必须能通过 libvirt Storage Volume API 定位。产物包括 `<name>-domain.xml`（inactive 持久 XML）、`<name>-manifest.json`（`virtmgr.vm-backup/v1`，记录虚拟机身份、磁盘映射和配置 SHA-256）以及 `<name>-disk-NN.<ext>`（磁盘副本）。创建前检查目标池内全部产物名，任一冲突返回 `409 CONFLICT`；磁盘复制、配置写入或 manifest 写入失败时删除本次已创建的卷。操作要求 Operator/Admin、可写模式和 CSRF，任务成功或失败均记录 `vm.backup` 审计，detail 不保存 XML 正文。该能力不包含内存状态，也不提供恢复或删除备份接口。

### 5.4 虚拟机模板

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET` | `/templates` | 模板列表 |
| `GET` | `/templates/{id}` | 模板详情 |
| `POST` | `/templates` | 创建模板，需 CSRF |
| `PATCH` | `/templates/{id}` | 更新模板，需 CSRF |
| `DELETE` | `/templates/{id}` | 删除模板，需 CSRF |

模板字段包括名称、描述、vCPU、内存、磁盘格式、网络、ISO 路径、固件、Secure Boot、Cloud-init 开关与 user-data、autostart 和创建后启动。Cloud-init 编辑器支持可视化字段与 YAML 源码双向切换：可视化修改生成标准 user-data，源码中的高级字段在仅查看时保持不变，切换回可视化修改前会提示可能被覆盖的字段。名称大小写不敏感唯一；Cloud-init 最多 64 KiB，提交前校验 YAML 语法和可视化用户名，审计只记录启用状态与字节数，不记录正文。模板不保存实例名称或磁盘路径，避免误用为实例状态来源。前端 `/templates` 提供 CRUD，并用 `/vms/new?template={id}` 预填创建表单。

### 5.5 存储与网络

当前已实现以下接口；其余接口是后续阶段的契约，尚未挂载：

- `GET /api/v1/storage/pools`
- `GET /api/v1/storage/pools/{pool}`（存储池详情）
- `POST /api/v1/storage/pools`（需 CSRF；仅支持目录池 `dir`，目标路径必须为绝对路径）
- `POST /api/v1/storage/pools/{pool}/actions/start|stop|refresh`（需 CSRF；按池状态校验）
- `DELETE /api/v1/storage/pools/{pool}`（需 CSRF；仅未激活池可删除定义）
- `GET /api/v1/storage/pools/{pool}/volumes`（仅活跃池；未激活池返回 `503 UNAVAILABLE`）
- `POST /api/v1/storage/pools/{pool}/volumes`（仅活跃池；需 CSRF，支持 `qcow2` / `raw`，重名返回 `409 CONFLICT`）
- `POST /api/v1/storage/pools/{pool}/volumes/upload?name={iso}`（仅活跃池；需 CSRF 和 `Content-Length`；以原始字节流创建 raw ISO 卷）
- `DELETE /api/v1/storage/pools/{pool}/volumes/{volume}`（仅活跃池；需 CSRF；被虚拟机引用返回 `409 VOLUME_IN_USE`）
- `GET /api/v1/networks`（虚拟网络列表）
- `GET /api/v1/networks/{name}/leases`（活跃网络的 DHCP 租约；未激活网络返回空列表）
- `POST /api/v1/networks`（需 CSRF；支持 `nat` / `isolated`，重名返回 `409 CONFLICT`）
- `PATCH /api/v1/networks/{name}`（需 CSRF；仅未激活 NAT/隔离网络，修改模式、子网、DHCP、autostart）
- `POST /api/v1/networks/{name}/hosts`（需 CSRF；活跃/未激活网络均可添加静态 DHCP 主机）
- `DELETE /api/v1/networks/{name}/hosts/{mac}`（需 CSRF；活跃/未激活网络均可删除静态 DHCP 主机）
- `POST /api/v1/networks/{name}/actions/start|stop`（需 CSRF；按当前状态校验）
- `DELETE /api/v1/networks/{name}`（需 CSRF；仅未激活网络可删除）

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET` | `/storage/pools` | 存储池列表 |
| `POST` | `/storage/pools` | 创建目录类型存储池（已实现） |
| `GET` | `/storage/pools/{name}` | 存储池详情 |
| `POST` | `/storage/pools/{name}/actions/start|stop|refresh` | 启动、停止或刷新池（已实现，按状态校验） |
| `DELETE` | `/storage/pools/{name}` | 删除未激活池定义（已实现；不删除磁盘文件） |
| `GET` | `/storage/pools/{name}/volumes` | 卷列表 |
| `POST` | `/storage/pools/{name}/volumes` | 创建 qcow2/raw 卷 |
| `POST` | `/storage/pools/{name}/volumes/upload` | 流式上传 ISO（已实现同步流式写入） |
| `DELETE` | `/storage/pools/{name}/volumes/{volume}` | 删除未挂载卷 |
| `GET` | `/networks` | 虚拟网络与宿主机桥 |
| `POST` | `/networks` | 创建网络 |
| `PATCH` | `/networks/{name}` | 编辑未激活 NAT/隔离网络 |
| `POST` | `/networks/{name}/hosts` | 添加静态 DHCP 主机（活跃网络实时生效） |
| `DELETE` | `/networks/{name}/hosts/{mac}` | 删除静态 DHCP 主机（活跃网络实时生效） |
| `GET` | `/networks/{name}/leases` | DHCP lease（已实现；未激活网络返回空列表） |
| `POST` | `/networks/{name}/start|stop` | 启停网络 |
| `DELETE` | `/networks/{name}` | 删除未激活网络 |

### 5.6 实时通道与系统

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET` | `/events?limit=100` | 最近事件快照；`limit` 范围 1-200，默认 100 |
| `GET` | `/events/stream?limit=100` | SSE：初始快照 + 实时事件 + 15 秒心跳，断线由浏览器自动重连 |
| `GET` | `/tasks?status=running&page=1&pageSize=50` | 任务列表（已实现）；状态可选 queued/running/succeeded/failed/interrupted |
| `GET` | `/tasks/{id}` | 单任务详情（已实现） |
| `GET` | `/tasks/stream` | 任务进度 SSE（目标能力，当前页面使用 5 秒轮询） |
| `GET` | `/audit-events` | 审计查询；支持 `actor` / `action` / `targetKind` / `result` 筛选和 `page` / `pageSize` 分页，仅管理员 |
| `GET` | `/settings` | 可管理设置 |
| `PUT` | `/settings` | 更新设置（管理员，需 CSRF；当前支持 `readOnly`，已实现） |
| `GET` | `/healthz` | 进程存活 |
| `GET` | `/readyz` | libvirt 与数据库就绪 |

事件结构包含 `id`、`occurredAt`、`source`、`kind`、`severity`、`targetKind`、`targetId`、`targetName`、`message` 和可选 `detail`。当前已映射域定义/删除、启动、停止、关机、暂停、恢复、崩溃和状态变化，以及快照、存储卷和虚拟网络写操作。后续 `scope=vm:{uuid}` 过滤属于目标能力，当前先提供全局流，由前端按目标字段筛选。

任务结构包含 `id`、`kind`、`targetKind`、`targetId`、`status`、`progress`、`step`、`error`、`actor`、`createdAt` 和 `updatedAt`。任务记录与审计职责不同：任务表达进度和结果，审计表达谁执行了什么操作；二者可关联同一个资源，但不互相替代。当前 ISO 上传通过 `storage.volume.upload` 任务记录流式进度，有界 worker pool 与可取消任务仍属于后续增强。

运行时只读模式保存在 SQLite `settings` 表的 `read_only` 键中。首次启动使用 `VIRTMGR_READ_ONLY` 初始化，之后以数据库持久化值为准；管理员通过 `PUT /settings` 切换，模式变化会立即作用于会话响应以及虚拟机、存储、网络和用户写服务。设置更新不受当前只读状态阻塞，但必须由管理员通过 CSRF 校验执行，并写入 `settings.readonly.update` 审计。

## 6. 前端信息架构

### 6.1 路由

```text
/login
/
/vms
/vms/new
/vms/$uuid
  ?tab=overview|console|hardware|disks|network|snapshots|xml|events
/console/vms/$uuid             独立全屏控制台
/templates
/storage
/storage/pools/$name
/networks
/networks/$name
/host
/tasks
/events
/audit
/users
/settings
```

详情页使用 query 参数保留当前 tab，便于刷新、复制链接和从全局搜索结果直达。

### 6.2 主导航

| 分组 | 菜单 | 图标语义 |
|---|---|---|
| 运行 | 总览 | LayoutDashboard |
| 资源 | 虚拟机 | Server/Monitor |
| 资源 | 虚拟机模板 | LayoutTemplate |
| 基础设施 | 存储 | Database |
| 基础设施 | 网络 | Network |
| 宿主 | 物理主机 | Cpu |
| 运维 | 任务中心 | ListChecks |
| 运维 | 事件 | Activity |
| 管理 | 审计日志 | ScrollText |
| 管理 | 用户管理 | Users |
| 管理 | 系统设置 | Settings2 |

底部固定当前宿主机连接状态卡：主机名、libvirt 版本、连接状态和重试入口，沿用 dosh 侧栏底部的健康状态卡。

### 6.3 全局搜索

顶栏全局搜索是运行与基础设施资源的统一入口，桌面端显示 `Ctrl+K` / `Meta+K` 提示，移动端保留图标按钮。

- 打开后按需加载虚拟机、虚拟网络和存储池，复用各功能页的 TanStack Query key，关闭后不额外轮询。
- 虚拟机支持按名称、UUID、Guest Agent 主机名、状态标签、IP 和网络源匹配；网络支持名称、UUID、桥接口、转发模式、地址与 DHCP 范围；存储池支持名称、UUID、类型、目标路径、状态和卷数量。
- 页面快速跳转覆盖总览、虚拟机、虚拟机模板、存储、网络、物理主机、任务中心、事件、审计日志和用户管理；审计与用户管理仅对管理员可见。
- 查询为空时展示当前角色可访问的快速跳转；有查询时按资源分组并限制每组结果数量。
- 支持鼠标点击、方向键切换、Enter 打开和 Escape 关闭；选择结果后直接进入资源详情或目标页面。

## 7. 页面布局

### 7.1 应用壳层

沿用 dosh 的结构：

- 顶栏 56px：移动端菜单、Logo、宿主机名与连接状态、全局搜索、任务入口、事件入口、主题切换、用户菜单。
- 左侧栏 240px：分组导航，桌面端 sticky，移动端抽屉。
- 内容区：`max-width: 1600px`，桌面 24px 内边距，移动端 16px。
- 页面标题区：面包屑、标题、状态/描述、右侧主操作。
- 圆角 8px，卡片使用 1px 边框和轻阴影，不使用大面积渐变或玻璃拟态。

```text
┌──────────────────────────── 56px header ────────────────────────────┐
│ [☰] V VirtMgr   ● kvm-node-01 / 正常   搜索 ⌘K  任务  事件  主题  用户 │
├──────────────┬───────────────────────────────────────────────────────┤
│ 运行          │ 面包屑                                                │
│  总览         │ 虚拟机                                     [新建虚拟机] │
│  虚拟机       │ 状态摘要与筛选                                        │
│ 基础设施      │ ┌───────────────────────────────────────────────────┐ │
│  存储         │ │ 数据表：名称 / 状态 / vCPU / 内存 / IP / 磁盘 ... │ │
│  网络         │ └───────────────────────────────────────────────────┘ │
│ 宿主          │                                                       │
│  物理主机     │                                                       │
│ 运维          │                                                       │
│  任务中心     │                                                       │
│  事件         │                                                       │
│ 管理          │                                                       │
│  审计日志     │                                                       │
│ ┌──────────┐ │                                                       │
│ │ 连接状态卡 │ │                                                       │
│ └──────────┘ │                                                       │
└──────────────┴───────────────────────────────────────────────────────┘
```

### 7.2 总览页

首屏回答四个问题：宿主机是否健康、虚拟机是否正常、资源是否吃紧、最近发生了什么。

```text
[运行中 12/16] [异常 1] [vCPU 分配 42/64] [内存分配 96/128 GiB]

┌─ 宿主机资源 ────────────────────────┐ ┌─ 虚拟机状态 ──────────────┐
│ CPU 实线/分配双指标条               │ │ Running  12                │
│ 内存 实线/分配双指标条              │ │ Paused    1                │
│ 存储池 Top 3                        │ │ Shut off  3                │
└─────────────────────────────────────┘ └───────────────────────────┘

┌─ 最近异常/事件 ─────────────────────────────────────────────────────┐
│ Warning  vm-db-01  QEMU 进程异常退出                       2 分钟前  │
└─────────────────────────────────────────────────────────────────────┘
```

- 指标卡沿用 dosh `MetricCard`，数值用等宽数字，状态色只用于图标底和状态点。
- 资源使用使用 `ResourceBar`，展示“实际使用 / 物理容量”，同时补充“已分配 / 物理容量”。
- 不把折线图放在首屏核心位置；趋势图进入物理主机详情或 VM 详情。

### 7.3 虚拟机列表

顶部工具栏：搜索、状态筛选、存储池筛选、标签筛选、刷新、新建。

表格列建议：

| 列 | 内容 |
|---|---|
| 名称 | 名称 + UUID 短码 + 标签 |
| 状态 | `StatusBadge` |
| 规格 | vCPU / 内存 |
| IP | 多个 IP 折叠展示，Guest Agent 不可用时显示 `-` |
| 磁盘 | 磁盘数量与总分配容量 |
| 网络 | 网桥/NAT 名称 |
| 自动启动 | 是/否 |
| 运行时长 | 运行中显示，停机显示 `-` |
| 操作 | 启动/关机/控制台/更多菜单 |

移动端沿用 dosh 的横向滚动和首列冻结策略，不把表格改成难以比较的卡片堆叠。虚拟机名称与选择框合并为冻结首列，表格滚动区域可通过键盘聚焦；窄屏详情页、趋势卡片和控制台头部均通过 Playwright 验收。

### 7.4 新建虚拟机

使用全页分步向导，而不是单页大表单：

1. **基本信息**：名称、展示名、描述、标签。
2. **安装来源**：ISO 安装、导入现有 qcow2/raw、空白磁盘 + cloud-init NoCloud seed ISO。
3. **计算资源**：vCPU、内存、CPU 模式、固件 BIOS/UEFI、Secure Boot、机器类型。
4. **存储与网络**：存储池、磁盘格式/容量、网络或桥、MAC 分配。
5. **确认创建**：展示最终 XML 摘要和资源占用，支持“创建后启动”。

右侧固定摘要卡显示名称、规格、磁盘、网络和预计占用。提交后返回任务 ID，跳到任务中心或 VM 详情。

### 7.5 虚拟机详情

```text
面包屑 / 虚拟机 / vm-db-01
vm-db-01  [Running]             [打开控制台] [关机] [更多操作]
UUID · 4 vCPU · 8 GiB · 192.168.122.31 · 运行 3 天 4 小时

┌───────────────────────────────────────┬───────────────────────────┐
│ 概览 控制台 硬件 磁盘 网络 快照 XML 事件 │ 状态                      │
│                                       │ 自动启动       是          │
│ 运行指标图表                          │ 固件           UEFI        │
│ 最近事件                              │ QEMU 版本      ...         │
│ 磁盘/网卡摘要                         │ 创建时间       ...         │
└───────────────────────────────────────┴───────────────────────────┘
```

Tab 内容：

- **概览**：状态、IP、规格、运行时长、CPU/内存/网络趋势、最近事件。
- **控制台**：详情页内嵌 noVNC 或 xterm.js 视图，可在图形/串口间切换；支持组合键、缩放、断开重连、全屏、展开到独立页面和分离窗口。
- **硬件**：vCPU、内存、CPU 模式、固件、机器类型、启动顺序、Guest Agent、设备。
- **磁盘**：target、bus、format、容量、allocation、存储池路径、是否可启动；支持热插拔的后续扩展。
- **网络**：网卡型号、MAC、虚拟网络/桥、IP、DHCP lease。
- **快照**：时间线或表格，标记 current，回滚和删除分离。
- **XML**：只读 CodeMirror；管理员可切换编辑，提交前 validate/define，并明确“仅定义不启动”。
- **事件**：该 VM 的 libvirt 与业务事件流。

### 7.6 独立控制台

`/console/vms/$uuid` 使用最小化黑色界面：

- 顶部工具栏：VM 名称、连接状态、图形/串口切换、组合键、缩放模式、断开/重连、全屏、展开与分离窗口。
- 图形控制台使用 noVNC canvas，支持自适应、原始大小和远端调整三种缩放模式。
- 串口控制台使用 xterm.js，支持多 PTY 设备选择、终端自适应和键盘输入。
- 断线时保留提示和重连按钮，不直接关闭窗口。
- 控制台令牌一次性、短时效、绑定 VM、用户、控制台类型和串口设备；同一串口设备只允许一个活动会话。

### 7.7 存储、网络、物理主机

- **存储**：顶部池容量卡；下方存储池表；进入池后展示卷、ISO、容量、挂载 VM。上传使用流式进度对话框。
- **网络**：虚拟网络表与宿主机桥分区；详情展示 IPv4、DHCP 范围、bridge、lease 和关联 VM。
- **物理主机**：硬件摘要、CPU/内存/磁盘/网络趋势、PCI/USB 设备和 libvirt/QEMU 版本。
- **任务中心**：运行中、成功、失败三类筛选；任务详情包含步骤、耗时、错误和关联资源。
- **事件与审计**：事件面向运行态，审计面向“谁在何时对什么做了什么”，两者不混用。

### 7.8 虚拟机模板页

模板页沿用 dosh 的卡片与紧凑表格布局：顶部说明模板用途，表格展示名称、规格、固件、网络/磁盘、Cloud-init、启动策略和更新时间；新建/编辑表单在页面内展开，Cloud-init 区域采用左侧可视化字段、右侧实时 YAML 预览的响应式双栏布局，源码模式使用 CodeMirror YAML 高亮。删除需要输入模板名称确认。用户可从行内“使用模板”进入 `/vms/new?template={id}`，新建页顶部模板选择器会预填规格、固件、网络、Cloud-init 和启动策略，但保留名称与磁盘路径供操作者填写。

## 8. 视觉与主题

### 8.1 主题策略

直接沿用 dosh 的 Slate + 蓝色体系，不引入新的品牌主色。VirtMgr 只替换 Logo、产品名和少量 virtualization 语义色。

```css
:root {
  --background: 216 33% 97%;
  --foreground: 220 27% 14%;
  --card: 0 0% 100%;
  --primary: 214 90% 43%;
  --primary-foreground: 0 0% 100%;
  --secondary: 215 25% 95%;
  --muted: 215 24% 95%;
  --muted-foreground: 218 11% 42%;
  --accent: 214 100% 96%;
  --accent-foreground: 214 90% 36%;
  --destructive: 0 72% 51%;
  --success: 152 65% 25%;
  --warning: 35 92% 30%;
  --info: 214 90% 43%;
  --border: 216 20% 89%;
  --ring: 214 90% 43%;
  --radius: 0.5rem;
}

.dark {
  --background: 222 28% 9%;
  --foreground: 214 26% 94%;
  --card: 222 26% 12%;
  --primary: 214 92% 66%;
  --primary-foreground: 222 36% 10%;
  --secondary: 218 22% 17%;
  --muted: 218 21% 16%;
  --muted-foreground: 215 17% 68%;
  --accent: 216 31% 20%;
  --destructive: 0 76% 68%;
  --success: 152 56% 66%;
  --warning: 38 92% 62%;
  --border: 217 20% 24%;
  --ring: 214 92% 68%;
}
```

### 8.2 组件规范

| 元素 | 规范 |
|---|---|
| 字体 | `Inter`, `Noto Sans SC`, system-ui；代码/XML 使用 `JetBrains Mono` |
| 页面标题 | 20px semibold，紧凑字距 |
| 卡片标题 | 16px semibold |
| 正文 | 14px；辅助文字 12px |
| 表头 | 12px，muted foreground |
| 圆角 | `0.5rem`；按钮、输入框、徽章统一使用语义 radius |
| 间距 | 8px 基准；页面块间距 16px，区块内 20px |
| 阴影 | 仅卡片使用 `shadow-panel`，弹窗使用 Radix 默认层级 |
| 动效 | 160ms ease-out，仅淡入和 hover，不做大幅位移 |

### 8.3 VM 状态色映射

| 状态 | 样式 | 含义 |
|---|---|---|
| Running | success 绿 | 正常运行 |
| Starting / Stopping | info 蓝 | 过渡状态，允许查看但限制操作 |
| Paused / PMSuspended | warning 黄 | 暂停或休眠，需要用户关注 |
| Shut off | secondary 灰 | 已定义但未运行 |
| Crashed | destructive 红 | QEMU 异常，提供诊断入口 |
| Unknown | 灰 | libvirt 状态不可读 |

状态徽章必须包含圆点 + 文本，不允许只靠颜色表达。危险操作按钮使用 destructive，但普通关机仍使用 outline + 确认，避免把所有生命周期按钮都做成红色。

## 9. 关键交互与安全

### 9.1 生命周期操作

- 正常关机：`Shutdown`，可选 ACPI，带超时提示。
- 强制下电：`Destroy`，必须二次确认并说明可能丢数据。
- 重启：优先 guest reboot；无 Guest Agent 时提示。
- 暂停/恢复：`Suspend/Resume` 与 `Pause/Unpause` 在 UI 文案中区分。
- 删除：先删除域定义，再让用户选择保留或删除磁盘；删除磁盘必须单独确认。
- 快照回滚：明确会丢弃回滚点之后的状态。
- XML 编辑：仅管理员；始终先 `DomainDefineXMLFlags` 校验，失败不落盘。

### 9.2 认证与权限

首版建议三种角色：

| 角色 | 权限 |
|---|---|
| Viewer | 只读查看、事件、XML 只读；不提供可交互控制台 |
| Operator | 生命周期、快照、控制台、上传 ISO；不能改 XML/删除宿主机资源 |
| Admin | 全部操作、用户管理、设置、审计 |

登录方式：

- 默认本地管理员，密码使用 Argon2id，首次启动通过环境变量或 CLI 生成。
- 可选 PAM 登录，服务运行用户需在明确的 PAM 配置中授权。
- 会话 Cookie 使用 `HttpOnly + Secure + SameSite=Strict`，空闲/绝对超时与 dosh 保持一致。
- 写请求校验 CSRF token；全局只读模式直接拒绝所有写操作。
- 控制台使用一次性 token，不在 URL 中暴露长期会话 Cookie。
- 反向代理部署时校验 `X-Forwarded-*` 仅在可信代理配置下生效。

### 9.3 审计

记录所有写操作和控制台打开事件：

```json
{
  "time": "2026-11-07T12:00:00Z",
  "requestId": "req_...",
  "actor": "admin",
  "role": "admin",
  "action": "vm.destroy",
  "target": { "kind": "vm", "uuid": "...", "name": "vm-db-01" },
  "sourceIp": "192.168.1.20",
  "result": "success",
  "detail": { "previousState": "running" }
}
```

密码、VNC 密码、私钥、cloud-init secret 和完整 XML 中的敏感字段必须脱敏。审计写入失败时，危险写操作默认 fail-closed。

### 9.4 文件与路径安全

- 只通过 libvirt Storage Pool/Volume API 操作磁盘，不直接拼接宿主路径。
- 上传 ISO 时直接创建目标 raw 卷并把请求体写入 libvirt 流；必须校验 `Content-Length` 和大小，池容量由 libvirt 写入时校验，失败时删除未完成卷。后续如需异步大文件任务，再引入临时卷和原子重命名。
- 禁止用户输入任意宿主机路径执行命令。
- XML 只接受 libvirt 可解析内容；禁止通过 XML 注入任意宿主机 hook/command。

## 10. 可观测性

- 所有请求带 `X-Request-ID`，日志、错误响应和审计可关联。
- 慢操作记录 libvirt 调用耗时、VM UUID、动作和结果，不记录敏感 XML。
- `/healthz` 只表示进程存活；`/readyz` 校验 libvirt 连接和 SQLite 可写。
- 指标采集使用批量 `ListAllDomains` + domain stats，避免每页每 VM 单独查询。
- 前端错误区分：网络失败、权限不足、状态冲突、libvirt 错误、任务失败，并保留可复制 request ID。

## 11. 部署形态

### 11.1 推荐：宿主机 systemd

```ini
[Unit]
Description=VirtMgr Console
After=libvirtd.service
Requires=libvirtd.service

[Service]
User=virtmgr
Group=virtmgr
SupplementaryGroups=libvirt
ExecStart=/usr/local/bin/virtmgr
Environment=VIRTMGR_LISTEN=:8443
Environment=VIRTMGR_LIBVIRT_URI=qemu:///system
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

### 11.2 容器化

- 容器需要挂载 `/var/run/libvirt/libvirt-sock`。
- 镜像包含 `libvirt0` 运行库；不要以 `--privileged` 运行。
- 若管理网络/存储，优先通过 libvirt API 和受控 socket，而不是挂载整个宿主机根目录。
- 前端静态资源由 Go `embed` 提供，无需单独部署 Node 服务。

## 12. 分阶段落地

### P0：可用闭环

- 应用壳层、登录、角色、CSRF、只读模式、审计基础。
- 总览、VM 列表、VM 详情、生命周期动作。
- 创建 VM（ISO/空白磁盘）、磁盘与网络摘要。
- 存储池/卷只读、网络只读、事件流、任务中心基础。
- 图形/串口控制台和一次性 token。

验收：可在浏览器完成创建、启动、关机、重启、暂停、强制下电、删除和打开控制台；所有写操作有审计。

### P1：日常运维增强

- 快照创建/回滚/删除、VM 克隆、ISO 上传。
- 存储池 CRUD、存储卷高级操作和虚拟网络详情/DHCP lease。
- Guest Agent IP/hostname、CPU/内存/网络趋势。
- 全局搜索、批量操作、只读模式开关。
- 移动端表格、控制台和详情页可用性验收。

当前 P1 已落地快照、ISO 上传、VM 克隆、Guest Agent 主机名/IP、宿主机与虚拟机 CPU/内存/网络趋势、全局搜索、虚拟机批量生命周期操作、运行时只读模式开关，以及目录存储池的创建、启停、刷新和删除。克隆要求源域关机/异常停止、持久化，且可写磁盘由 libvirt 存储池管理；默认在源卷所在活跃池创建独立卷，也可指定目标池。任务失败时回滚已创建卷和域定义。Guest Agent 查询失败按不可用降级，不影响虚拟机详情或列表读取。批量操作逐台校验动作和状态，部分失败不会回滚已成功的虚拟机。只读模式状态持久化并动态作用于全部写服务。移动端表格、详情页、趋势卡片和 VNC 控制台，以及跳转主内容、抽屉焦点陷阱、Escape 关闭和键盘聚焦滚动区域均已有严格串行 Playwright 验收。

验收：常见运维任务不依赖 `virsh`；长任务有进度和失败原因。

### P2：高级能力

- 虚拟机模板、PCI/USB 设备直通。
- 备份任务（已实现冷备份）、导出（已实现）、定时快照策略（已实现）。
- 多宿主机 registry/agent、迁移。
- OIDC、细粒度 RBAC、审计 Webhook。

当前 P2 已落地 UEFI/Secure Boot、cloud-init NoCloud seed ISO、虚拟机模板、PCI/USB 直通、定时快照策略、管理员持久 XML 编辑、虚拟机配置导出与冷备份：默认 BIOS 保持向后兼容；UEFI 使用 firmware 自动选择，Secure Boot 通过 loader secure 标志启用。cloud-init 用户数据由纯 Go ISO9660 生成器写入存储池卷，通过只读 CD-ROM 挂载，审计不保存用户内容。模板由 SQLite 持久化，支持 CRUD、权限/只读校验和从模板预填创建表单。直通使用 libvirt 节点设备和 hostdev XML，支持运行中热插拔、持久配置写入、跨虚拟机重复分配校验和审计。定时快照策略使用 SQLite 调度元数据和进程内每分钟调度器，支持 CRUD、立即执行、启用/禁用、保留清理和失败状态回写；清理仅作用于带 `[policy:<id>]` 标记的快照。XML 编辑锁定域身份、拒绝 QEMU commandline 命名空间并通过 libvirt validate/define 更新持久配置。配置导出提供版本化 JSON、持久/运行态 XML、快照元数据和只读模式下的审计闭环，不包含磁盘数据。冷备份将持久 XML、manifest 和可写磁盘复制到活跃存储池，以 `vm.backup` 任务展示真实进度，失败回滚本次产物，审计不保存 XML 正文，且暂不包含内存状态或恢复能力。创建审计记录固件、安全启动和 cloud-init 元信息，虚拟机详情展示回读结果，Go 单元测试、HTTP 集成测试与严格串行 Playwright 覆盖创建、模板、直通、策略、XML 编辑、导出和冷备份流程。

## 13. 与 dosh 的复用边界

可以直接复用或轻微改造：

- `AppShell` 的三段布局、移动端抽屉和连接状态卡。
- `PageHeader`、`DataTable`、`MetricCard`、`ResourceBar`、`StatusBadge`。
- `Dialog/AlertDialog/Sheet/DropdownMenu` 等 shadcn 组件。
- CSS 主题变量、明暗切换、字体、卡片和表格密度。
- CodeMirror 只读/编辑容器、错误状态、空状态和骨架屏模式。

不应直接复用：

- Kubernetes Namespace、Pod、Workload 等资源模型和路由。
- Kubernetes Token/OIDC 认证流程。
- Kubernetes 事件消息格式和 RBAC 判断方式。

## 14. 主要风险与决策

| 风险 | 决策 |
|---|---|
| CGO/libvirt 依赖使构建和部署复杂 | 首版直接面向 KVM 宿主机，提供 systemd 包；用接口隔离 libvirt，后续可替换 |
| 控制台端口和密码安全 | 后端代理 VNC socket，使用一次性 token，浏览器不直连宿主机端口 |
| 大文件 ISO 上传拖垮服务 | 流式写入、大小/容量上限、异步任务、worker pool |
| 长任务期间进程重启 | 任务落 SQLite；启动时将 running 状态标记为 interrupted，并提示重试 |
| libvirt 状态与页面不一致 | 所有动作以服务端状态校验为准；SSE 触发查询失效；禁止前端伪造最终状态 |
| 误删磁盘/强制下电 | 独立确认、名称输入、审计、只读模式 |
| VM 数量多导致页面卡顿 | 服务端分页/筛选，批量统计缓存，表格虚拟化留到规模验证后引入 |

## 15. 首版验收清单

- [ ] 单二进制启动后能连接本机 libvirt，并在 `/overview` 展示真实数据。
- [x] VM 列表、详情、状态筛选和全局搜索可用。
- [ ] 所有生命周期动作受服务端状态机和权限控制，并写入审计。
- [ ] 创建 VM 流程可生成可用域 XML，任务可追踪。
- [x] VNC 与串口控制台可通过一次性 token 打开，支持组合键、断开重连、全屏和分离窗口。
- [x] 存储池、卷、网络、DHCP lease 和事件可查看。
- [x] 明暗主题、桌面/移动布局与键盘操作通过可访问性检查。
- [ ] libvirt 不可用时 `/readyz` 失败，页面展示可恢复错误而不是空白。

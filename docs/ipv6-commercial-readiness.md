# IPv6 商用就绪度评估

> 评估日期：2026-11-20  
> 评估范围：单机 KVM/libvirt 精品版 VirtMgr，包括管理面 IPv6 接入、客户机 IPv4/IPv6 双栈、SLAAC、DHCPv6、NAT66、TLS 轮换和运维诊断。  
> 结论：**在“单机、受控网络、NAT66 仅出站”的产品边界内，当前版本已达到商用候选基线；不能据此宣称任意 IPv6 拓扑、任意发行版或公网入站场景均已覆盖。**

## 1. 商用判定

| 能力域 | 当前结论 | 边界 |
|---|---|---|
| 管理面 IPv6 接入 | 可用 | 支持双栈通配监听、IPv6 字面地址访问、IPv6 trusted proxy 和原生 TLS；生产应使用稳定 DNS 名称 |
| 客户机 IPv6 数据面 | 可用 | 支持双栈和 IPv6-only；IPv6-only 下 SLAAC 或有状态 DHCPv6；NAT 网络默认 NAT66 |
| 网络运维诊断 | 可用 | 覆盖桥、转发、DHCPv6、RA、DNS、NAT66、网关、出网和运行态漂移；无 `CAP_NET_ADMIN` 时如实告警 |
| 浏览器兼容 | 可用 | Chromium、Firefox、WebKit 均通过 IPv6 字面地址和移动宽度布局验收 |
| TLS 证书轮换 | 可用 | 支持在线重载、SAN/到期/私钥权限诊断和失败保留旧证书；不负责签发外部 CA 证书 |
| NAT66 公网入站 | 不支持 | 当前只承诺客户机出站；端口映射、安全组和入站防火墙策略不在本阶段范围 |
| 高级 IPv6 网络编排 | 部分支持 | 尚无前缀委派、精细 RA 标志/生存期、MTU、策略路由和 DNS 记录级管理 |

因此：

- 若产品承诺限定为“单机管理、客户机双栈/仅 IPv6 出网、受控上游网络”，可以进入商用交付和验收。
- 若产品承诺包含“IPv6 业务端口对外开放”“运营商前缀委派”“替代企业级 IPv6 路由/安全策略”，当前仍不具备完整商用条件。

## 2. 已闭环能力

### 管理面 IPv6 接入

- 支持监听 `[::]:8443`，已通过 IPv6 字面地址访问 `/healthz` 和登录页面。
- `trusted_proxies` 支持 IPv4/IPv6 CIDR；`X-Forwarded-For` 的 IPv6 客户端链已有边界测试。
- 原生 TLS 使用动态 `GetCertificate`；证书或私钥文件变化后，新连接自动加载，无需重启。
- 重载失败时继续提供上一份有效证书，并在 `/api/v1/system/info` 和系统设置页展示错误。
- TLS 状态包含 subject、issuer、有效期、剩余天数、DNS/IP SAN、当前请求主机匹配、私钥权限和重载错误。
- 证书剩余 30 天、SAN 不匹配、证书过期和私钥权限过宽均产生明确告警。

### 客户机 IPv6 数据面

- NAT/isolated 网络支持 IPv4-only、双栈和 IPv6-only。
- IPv6 支持 SLAAC（`/64`）和有状态 DHCPv6（`/64` 至 `/124`）；NAT 网络默认启用 NAT66，可显式关闭。
- 支持 IPv4/IPv6 DNS 上游、DNS domain、静态 IPv6 主机和网络配置告警。
- IPv4 静态主机使用 MAC；IPv6 静态主机使用 DUID/Client ID 或主机名，拒绝跨地址族误用身份字段。
- DHCPv6 租约可保留为静态主机；重复身份、重复 IP、缺少身份和地址越界均拒绝并记录审计。
- 活跃网络上的静态主机增删实时生效；修改前缀、移除网段或关闭 DHCPv6 时阻止导致主机失联的变更。
- `nat ipv6='yes'` 生成、NAT66 masquerade 规则、RA/DHCPv6 服务和运行态/持久态漂移均纳入诊断。
- 兼容 `iptables-nft` 的 `xt target "MASQUERADE"` 输出，不再因防火墙命令行格式差异误报。
- 空桥尚无客户机端口时，网关和 DNS 探测返回“待客户机接入” warning，而不是伪造 error。
- 明确边界：NAT66 仅提供客户机出站访问，不提供 IPv6 入站端口映射。

## 3. 真实兼容矩阵

以下结果均来自真实 libvirt 运行时，并执行同一组 `TestRealLibvirtIPv6*`：双栈静态主机、IPv6-only SLAAC、DHCPv6 客户端取址/NAT66 出网、SLAAC 客户端取址/网关访问。

| 发行版 | 内核 | libvirt | dnsmasq | 防火墙 | 结果 |
|---|---|---:|---:|---|---|
| Fedora Linux 44 Server | 7.2.11 | 12.0.0 | 2.92rel2 | nftables 1.1.6 | 通过 |
| Ubuntu 22.04 LTS | 5.15.0 | 8.0.0 | 2.91 | nftables 1.0.2 | 通过 |
| Ubuntu 22.04 LTS（降级验证） | 5.15.0 | 8.0.0 | 2.86 | nftables 1.0.2 | 通过 |
| Ubuntu 24.04 LTS | 6.8.0 | 10.0.0 | 2.91 | nftables 1.0.9 | 通过 |
| Debian GNU/Linux 12 | 6.1.0 | 9.0.0 | 2.90 | nftables 1.0.6 | 通过 |
| Debian GNU/Linux 13 | 6.12.0 | 11.3.0 | 2.91 | nftables 1.1.3 | 通过 |
| Rocky Linux 9 | 5.14.0 | 11.10.0 | 2.85 | nftables 1.0.9 | 通过 |
| Rocky Linux 10 | 6.12.0 | 12.5.0 | 2.90 | nftables 1.1.5 | 通过 |

Rocky Linux 用于验证 RHEL 9/10 兼容路径，不替代真实 RHEL 认证；若对外承诺 RHEL，仍需在目标 RHEL 小版本重复执行。

当前运行态最低基线为 libvirt `8.0.0`、dnsmasq `2.85`。2.85 基线来自 Rocky 9 的真实四类数据面通过结果，不再使用未经证据支持的 2.86 门槛。

浏览器结果：

| 浏览器 | 验收内容 | 结果 |
|---|---|---|
| Chromium | IPv6 字面地址登录、网络页、诊断卡、390/320px 移动布局 | 通过 |
| Firefox | IPv6 字面地址登录、网络页、诊断卡、390/320px 移动布局 | 通过 |
| WebKit | 同上；使用官方 Playwright Ubuntu 容器运行 | 通过 |

WebKit 不在 Fedora 宿主直接运行，原因是宿主动态库与 Ubuntu 版 WebKit 不兼容；容器仅提供浏览器运行时，不影响被测应用和 IPv6 地址语义。

## 4. 复现方式

```bash
# 当前主机静态能力、版本基线和真实 libvirt 回归
scripts/ipv6-compat-check.sh

# 三种浏览器对 IPv6 字面地址和移动布局的验收
scripts/ipv6-browser-compat.sh

# 原生 TLS：IPv6 监听、在线轮换、SAN 信任、失败回滚
scripts/tls-rotation-drill.sh

# 创建各发行版真实 cloud VM 并执行矩阵
scripts/ipv6-distro-matrix.sh all
```

矩阵报告默认写入 `/var/lib/libvirt/images/virtmgr-ipv6-matrix/reports/`。发布验收应保存对应版本的原始报告，不应只保留汇总表。

## 5. 后续优先能力

### P1：公网 IPv6 业务和高级网络所必需

1. **NAT66 入站端口映射**：为虚拟机配置 IPv6 DNAT/防火墙规则、冲突校验和回收；当前只有出站。工作量 M-L，价值取决于是否对外提供 IPv6 服务。
2. **IPv6 安全组/入站策略**：按网络或虚拟机管理允许的协议、端口和来源前缀，并支持默认拒绝；不能靠宿主机手工 nftables 长期维护。工作量 M-L。
3. **RA/DHCPv6 高级参数**：M/O 标志、RA lifetime、最大/最小间隔、DNS RA、多前缀和前缀委派。工作量 M，企业网络接入时价值高。
4. **IPv6 路由和 MTU 配置**：静态路由、策略路由、桥/虚拟网络 MTU 和 PMTU 诊断；对隧道、Overlay 和跨网段部署必要。工作量 M。
5. **管理面 IPv6 访问控制**：源 IPv6 CIDR allowlist、登录/API 速率限制、管理员会话审计统一使用规范化 `netip.Addr`。工作量 S-M，公网暴露前价值高。

### P2：商用运维增强

6. **DNS 记录级管理**：A/AAAA/PTR/CNAME、搜索域、递归/转发策略和记录冲突检查；当前只管理 domain 与 forwarder。工作量 M。
7. **DHCPv6 租约生命周期**：历史、导出、地址使用率、重复 DUID/冲突趋势和容量告警。工作量 S-M。
8. **双栈 Cloud-init 自动生成**：从所选网络生成 Netplan/network-config，并同步静态 IPv4/IPv6、网关、DNS 和 MTU。工作量 S-M。
9. **IPv6 可观测性**：把诊断结果、租约使用率、RA/DHCPv6 服务状态和 NAT66 规则状态纳入历史指标与告警。工作量 S-M。
10. **自动证书/DDNS 编排**：ACME/DNS-01、稳定 DNS 名称、前缀变化后的地址更新；当前只做证书热重载和 SAN 诊断。工作量 M。
11. **网络模板与 ULA 自动规划**：自动选择不冲突的 ULA 前缀、按项目分配 `/64`、导入现有 IPv6 网络定义。工作量 S-M。

### 非代码交付门槛

- 生产环境应使用受信 CA 签发的稳定 DNS 名称证书，并完成一次真实 CA 证书替换；自签证书演练不能替代 CA 签发验证。
- 上线前确认上游路由器允许 NAT66 出网，IPv6 forwarding 和防火墙策略持久化，且所选 DNS 解析器可达。
- 若目标客户要求 RHEL、SUSE 或国产发行版认证，必须在对应真实版本上补跑矩阵并保存报告。

## 6. 发布建议

可将当前版本作为**单机、受控网络、出站 NAT66**场景的商用候选版交付。交付说明必须写明：

- 管理面优先使用稳定 DNS 名称和受信证书；IPv6 字面地址只作为受控入口。
- NAT66 只承诺出站访问；公网入站、端口映射和安全组尚未实现。
- 每个目标发行版必须执行 `scripts/ipv6-compat-check.sh` 并保存原始报告。
- 网络诊断出现 error 时不得承载生产业务；warning 需结合边界确认。
- 真实 RHEL/SUSE/国产发行版和真实 CA 证书轮换属于目标环境验收项，不由本矩阵自动推导。

证书轮换和回滚步骤见 `docs/tls-rotation-runbook.md`。

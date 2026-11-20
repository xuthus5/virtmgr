# 管理面 TLS 证书轮换 Runbook

本文适用于 VirtMgr 原生 TLS。若由 nginx、Caddy 或云负载均衡终止 TLS，证书轮换和 SAN 监控应在对应代理层执行，VirtMgr 设置页不会显示该证书状态。

## 1. 前置条件

- 使用稳定 DNS 名称访问管理面，生产环境不要把动态 IPv6 字面地址作为唯一入口。
- 证书文件为 PEM 证书链，私钥文件为匹配的 PEM 私钥。
- `server.yaml` 中同时配置 `tls_cert_file` 和 `tls_key_file`。
- 私钥权限为 `0600`，属主为 VirtMgr 服务账号或仅该账号可读。
- 新证书 SAN 包含实际访问的 DNS 名称；如必须使用字面地址，再显式加入对应 IPv4/IPv6 地址。

推荐目录结构：

```text
/etc/virtmgr/tls/current -> releases/2026-11
/etc/virtmgr/tls/releases/2026-11/tls.crt
/etc/virtmgr/tls/releases/2026-11/tls.key
```

`/etc/virtmgr/tls/current` 是符号链接。`server.yaml` 指向 `current/tls.crt` 和 `current/tls.key`，通过一次原子切换让证书和私钥同时生效。

## 2. 新证书预检

```bash
openssl x509 -in new/tls.crt -noout -subject -issuer -dates -ext subjectAltName
openssl pkey -in new/tls.key -check -noout
test "$(openssl x509 -in new/tls.crt -pubkey -noout | openssl pkey -pubin -outform der | sha256sum)" \
  = "$(openssl pkey -in new/tls.key -pubout -outform der | sha256sum)"
```

必须确认：

- 证书未过期，`notBefore` 已生效。
- SAN 包含管理面 DNS 名称和需要的 IP。
- 证书链完整，客户端信任根和中间证书。
- 私钥与证书匹配，且私钥无口令或服务能够安全解密。
- 私钥权限为 `0600`。

## 3. 在线切换

1. 将新版本复制到新的发布目录，不要直接覆盖当前文件。
2. 校正属主和权限：

   ```bash
   chown root:virtmgr /etc/virtmgr/tls/releases/2026-11/tls.key
   chmod 0600 /etc/virtmgr/tls/releases/2026-11/tls.key
   chmod 0644 /etc/virtmgr/tls/releases/2026-11/tls.crt
   ```

3. 原子切换符号链接：

   ```bash
   ln -s releases/2026-11 /etc/virtmgr/tls/current.new
   mv -Tf /etc/virtmgr/tls/current.new /etc/virtmgr/tls/current
   ```

4. 用新 CA 链或系统信任库验证新连接：

   ```bash
   curl --noproxy '*' -gfsS --cacert ca.pem \
     "https://[2001:db8::10]:8443/healthz"
   openssl s_client -connect '[2001:db8::10]:8443' -servername virtmgr.example.com \
     -showcerts </dev/null 2>/dev/null | openssl x509 -noout -subject -dates -ext subjectAltName
   ```

5. 在系统设置页确认：

   - 当前主机显示“SAN 匹配”。
   - 有效期和剩余天数来自新证书。
   - 私钥权限为 `0600`。
   - 没有热重载失败或到期告警。

热重载只影响新建立的 TLS 连接，已有长连接可能继续使用旧会话；浏览器刷新或重新连接后使用新证书。

## 4. 失败处理

证书或私钥无法解析、证书与私钥不匹配时，VirtMgr 会：

- 继续使用上一份有效证书提供服务。
- 在 `/api/v1/system/info` 的 `tls.reloadError` 和设置页展示错误。
- 后续每次状态读取都会重试加载；修正文件后无需重启即可恢复。

如果服务已经无法启动，说明首次加载就失败。此时检查日志中的 `load TLS certificate` 错误，修复文件路径、权限或证书链后重启服务。

## 5. 回滚

保留上一版本发布目录，回滚时只切回符号链接：

```bash
ln -s releases/2026-10 /etc/virtmgr/tls/current.new
mv -Tf /etc/virtmgr/tls/current.new /etc/virtmgr/tls/current
```

等待数秒后重新建立连接，确认指纹和 SAN 已切回旧证书。回滚不需要重启服务。

若没有使用符号链接，而是分别替换证书和私钥，替换过程中可能出现短暂的不匹配。服务会保留旧证书并在下一次文件变化后重试，但生产环境仍推荐符号链接方案。

## 6. 演练与监控

仓库内演练命令：

```bash
scripts/tls-rotation-drill.sh
```

演练覆盖 IPv6 监听、初始证书、在线轮换、新 SAN 信任和无效证书替换后保留旧证书。它使用自签证书，只证明热重载行为，不证明真实 CA 签发或客户端信任链。

生产监控至少应：

- 每日检查证书剩余天数，30 天内告警。
- 检查当前请求主机是否匹配 SAN。
- 检查 `reloadError` 和私钥权限告警。
- 对管理面 IPv6 地址和 DNS 解析结果做外部探活。
- 在证书轮换后保存新旧指纹、SAN 和验证命令输出作为审计记录。

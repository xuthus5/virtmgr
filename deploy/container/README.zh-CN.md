# VirtMgr 容器化部署

VirtMgr 可以以非特权容器运行在 libvirt 宿主机上。容器是管理客户端，不运行 QEMU/KVM，因此不需要 `/dev/kvm`，也不需要 Docker 或 Podman 的控制 socket。

本目录包含：

- `Dockerfile`：Node 前端、Go/CGO 后端和 Debian runtime 三阶段镜像。
- `compose.yaml`：Docker Compose / Podman Compose 示例，包含数据、配置和 libvirt 挂载。

## 使用 GHCR 预构建镜像

GitHub Actions 会发布 `linux/amd64` 和 `linux/arm64` 多架构镜像：

- 推送 `v*` 标签后发布 `ghcr.io/xuthus5/virtmgr:latest`，以及 `:0.3.1`、`:0.3`、`:0` 等语义版本标签。
- 推送 `main` 分支后发布 `ghcr.io/xuthus5/virtmgr:nightly` 和 `:sha-<短提交>`。
- 对应 nightly 二进制位于 GitHub 的滚动 prerelease `nightly`，每次成功构建都会覆盖说明和附件。

直接拉取预构建镜像，无需本地编译：

```bash
export VIRTMGR_IMAGE=ghcr.io/xuthus5/virtmgr:nightly
docker compose pull
docker compose up -d --no-build
```

如果 GHCR 包是私有的，先使用具有 `read:packages` 权限的 Token 登录：

```bash
echo "$GHCR_TOKEN" | docker login ghcr.io -u "$GHCR_USER" --password-stdin
```

## 前置条件

- Linux 宿主机已经运行 libvirt 和 QEMU/KVM。
- 使用 Docker Compose v2，或安装了 Compose provider 的 Podman。
- 可以读取 VirtMgr 配置，并有权连接宿主机 libvirt socket。
- 为容器内 `/var/lib/virtmgr` 提供可写的持久化目录或命名卷。

VirtMgr 通过 UNIX socket 连接同一台宿主机上的 libvirt，不用于管理任意远程 libvirt 服务。

## 构建镜像

在仓库根目录执行：

```bash
podman build \
  --build-arg VERSION="$(git describe --tags --always --dirty)" \
  --build-arg COMMIT="$(git rev-parse --short HEAD)" \
  --build-arg BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -f deploy/container/Dockerfile \
  -t virtmgr:local .
```

Docker 使用相同参数：

```bash
docker build -f deploy/container/Dockerfile -t virtmgr:local .
```

最终镜像以系统用户 `virtmgr` 运行，前端静态资源已经嵌入二进制，默认监听 `8443`。

## 准备 `server.yaml`

```bash
cd deploy/container
cp ../server.yaml.example server.yaml
chmod 0640 server.yaml
```

至少需要确认：

```yaml
listen: "[::]:8443"
libvirt_uri: qemu:///system
database_path: /var/lib/virtmgr/virtmgr.db
host_name: virt-host-01
admin_username: admin
admin_password: replace-with-a-strong-password
cookie_secure: true
```

首次成功启动后应删除 `admin_password`。密码会以哈希形式写入 SQLite，并保存在持久化卷中。

如果 TLS 由 nginx、Caddy 或 Cloudflare Tunnel 终止，应设置 `cookie_secure: true`，同时把 HTTP 和 WebSocket 流量转发给容器，并保持原生 `tls_cert_file` / `tls_key_file` 为空。

## libvirt socket 挂载

推荐直接挂载完整的 libvirt 运行目录：

```yaml
volumes:
  - /run/libvirt:/run/libvirt
```

这样同时兼容两种 libvirt 部署方式，也能避免 daemon 重启并重新创建 socket 后，单文件 bind mount 指向旧 inode：

- 单体模式：`/run/libvirt/libvirt-sock`。
- 模块化模式：`/run/libvirt/virtqemud-sock`、`/run/libvirt/virtnetworkd-sock`、`/run/libvirt/virtstoraged-sock`、`/run/libvirt/virtnodedevd-sock`、`/run/libvirt/virtsecretd-sock`、`/run/libvirt/virtnwfilterd-sock`、`/run/libvirt/virtinterfaced-sock`。

在模块化 libvirt 主机上不要只挂载 `libvirt-sock`。页面可能能够启动，但虚拟机、网络或存储操作会失败。

检查宿主机 socket 的组 ID，并配置补充组：

```bash
stat -c '%g %G %a %n' /run/libvirt/virtqemud-sock
export LIBVIRT_GID="$(stat -c '%g' /run/libvirt/virtqemud-sock)"
```

单体 libvirt 主机应将 `virtqemud-sock` 替换为 `/run/libvirt/libvirt-sock`。

`compose.yaml` 会通过 `group_add` 传入该值。容器进程仍然是非 root，补充组只用于获得 socket 访问权限。

不需要挂载 `libvirt-admin-sock`、`virtqemud-admin-sock`、`virtlockd-sock` 或 `virtlogd-sock`。

## 挂载矩阵

| 宿主机来源 | 容器路径 | 模式 | 是否必需 | 用途 |
|---|---|---:|---|---|
| `/run/libvirt` | `/run/libvirt` | rw | 是 | libvirt 控制 socket |
| `./server.yaml` | `/etc/virtmgr/server.yaml` | ro | 是 | 运行配置 |
| 命名卷 `virtmgr-data` | `/var/lib/virtmgr` | rw | 是 | SQLite、会话、审计和元数据 |
| `./tls` | `/etc/virtmgr/tls` | ro | 可选 | 原生 TLS 证书和私钥 |
| 存储池目录 | 相同绝对路径 | rw/ro | 可选 | 直接访问镜像、ISO、导入文件和备份文件 |

### 存储目录

VirtMgr 需要直接读写的宿主机存储路径，必须在容器内挂载到相同的绝对路径。例如：

```yaml
volumes:
  - /var/lib/libvirt/images:/var/lib/libvirt/images
  - /srv/virtmgr/iso:/srv/virtmgr/iso:ro
  - /srv/virtmgr/backups:/srv/virtmgr/backups
```

容器内路径必须与 libvirt 中记录或页面中填写的路径一致。如果把 `/var/lib/libvirt/images` 挂载到其他路径，卷导入和磁盘操作会失败。

容器以非 root 用户运行。如果挂载目录不可写，应通过属主、属组或 POSIX ACL 授予容器用户权限。不要使用 `--privileged` 或把宿主机目录改成全局可写来解决权限问题。

### 原生 TLS

如果由 VirtMgr 自己终止 TLS：

```yaml
volumes:
  - ./tls:/etc/virtmgr/tls:ro
```

在 `server.yaml` 同时设置：

```yaml
tls_cert_file: /etc/virtmgr/tls/tls.crt
tls_key_file: /etc/virtmgr/tls/tls.key
cookie_secure: true
```

证书和私钥必须能够被容器用户读取，但私钥不能让其他用户全局可读。

## 网络模式

### 默认 bridge 网络

示例 Compose 会发布 `8443:8443`，适合普通虚拟机、存储管理和宿主机反向代理。

入站端口转发和 NAT66 诊断依赖宿主机防火墙规则。bridge 容器中的 `CAP_NET_ADMIN` 只作用于容器自己的网络命名空间，不能修改宿主机规则。需要这些能力与 systemd 部署完全一致时，应使用 host 网络。

### 完整功能的 host 网络

单机管理平台通常更适合 host 网络：

```yaml
services:
  virtmgr:
    network_mode: host
    # 启用 host 网络后删除 ports 映射。
    cap_add:
      - NET_ADMIN
```

使用 host 网络且宿主机运行反向代理时，建议设置 `listen: "127.0.0.1:8443"`。只有已经配置防火墙和 TLS 策略时，才使用 `listen: "[::]:8443"`。

## 不使用 Compose 直接运行镜像

创建持久化卷并直接启动镜像：

```bash
LIBVIRT_SOCK=/run/libvirt/virtqemud-sock
[ -S "$LIBVIRT_SOCK" ] || LIBVIRT_SOCK=/run/libvirt/libvirt-sock
export LIBVIRT_GID="$(stat -c '%g' "$LIBVIRT_SOCK")"

podman volume create virtmgr-data

podman run -d \
  --name virtmgr \
  --restart unless-stopped \
  -p 8443:8443 \
  -e TZ=Asia/Shanghai \
  -v virtmgr-data:/var/lib/virtmgr \
  -v "$PWD/server.yaml:/etc/virtmgr/server.yaml:ro" \
  -v /run/libvirt:/run/libvirt \
  --group-add "$LIBVIRT_GID" \
  --cap-add NET_ADMIN \
  --security-opt no-new-privileges \
  virtmgr:local
```

Docker 使用相同参数，只需把 `podman volume create` 和 `podman run` 替换为 `docker volume create` 和 `docker run`。TLS 和存储目录作为额外的 `-v` 参数加入：

```bash
  -v "$PWD/tls:/etc/virtmgr/tls:ro" \
  -v /var/lib/libvirt/images:/var/lib/libvirt/images \
```

使用 host 网络时，把 `-p 8443:8443` 替换为 `--network host`，并在 `server.yaml` 中设置对应的 `listen` 地址。

## 启动、检查和升级

```bash
cd deploy/container
docker compose up -d --build
docker compose ps
docker compose logs -f virtmgr
curl -fsSk https://127.0.0.1:8443/healthz
```

Podman 用户把 `docker compose` 替换为 `podman compose`。

更新仓库后升级：

```bash
docker compose build --pull
docker compose up -d
docker compose exec virtmgr /usr/local/bin/virtmgr version
```

升级前应备份 VirtMgr SQLite 数据库。优先使用系统设置中的在线导出功能；也可以停止容器后复制持久化卷中的数据库。

## 在容器内恢复账号密码

在运行中的容器内执行以下命令。命令会隐藏输入的新密码，不把密码写入 shell 历史：

```bash
docker compose exec virtmgr \
  /usr/local/bin/virtmgr reset-password \
  --config /etc/virtmgr/server.yaml \
  --username admin
```

## 安全边界

- 不要使用 `--privileged`。
- 不要挂载 `/var/run/docker.sock` 或 Podman socket。
- 不要挂载 `/dev/kvm`；QEMU 运行在宿主机，不运行在 VirtMgr 容器中。
- 不要挂载整个宿主机根目录，只挂载必要的存储路径。
- `server.yaml` 和私钥应只读，并且只授予必要用户或组访问权限。
- 通过 HTTPS 访问页面时必须设置 `cookie_secure: true`。
- 不要在缺少 TLS、防火墙和强管理员密码的情况下，把可写模式直接暴露到公网。

## 故障排查

### 无法连接 socket 或 permission denied

确认已经挂载 `/run/libvirt`，并且 `LIBVIRT_GID` 与宿主机 socket 组 ID 一致。可以在容器内检查：

```bash
docker compose exec virtmgr id
docker compose exec virtmgr ls -l /run/libvirt/virtqemud-sock
```

### 模块化 libvirt 下虚拟机或网络操作失败

把单文件 `libvirt-sock` 挂载改为 `/run/libvirt:/run/libvirt`，然后重启容器。

### 网络诊断提示无法访问防火墙

添加 `CAP_NET_ADMIN`。如果需要宿主机级别的端口转发和宿主机防火墙可见性，还要启用 `network_mode: host`。

### 宿主机存在存储目录，但页面中看不到

将该目录按相同绝对路径挂载到容器中，并确认容器用户具有访问权限。

### VNC 控制台可以连接，但看不到客户机画面

VirtMgr 通过 libvirt 代理控制台，不需要发布 QEMU VNC 端口。应检查 libvirt socket 挂载，以及反向代理是否正确转发 WebSocket 流量。

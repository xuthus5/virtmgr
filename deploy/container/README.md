# VirtMgr container deployment

VirtMgr can run as an unprivileged container on the same host as libvirt. The container is a management client: it does not run QEMU/KVM itself and does not need `/dev/kvm` or the Docker/Podman socket.

This directory contains:

- `Dockerfile`: multi-stage frontend, Go/CGO backend, and Debian runtime image.
- `compose.yaml`: a Docker Compose / Podman Compose example with the required data, configuration, and libvirt mounts.

## Prebuilt GHCR images

GitHub Actions publishes multi-architecture images for `linux/amd64` and `linux/arm64`:

- `ghcr.io/xuthus5/virtmgr:latest` and semantic-version tags such as `:0.3.1`, `:0.3`, and `:0` from `v*` tag builds.
- `ghcr.io/xuthus5/virtmgr:nightly` and `:sha-<short>` from pushes to the `main` branch.
- Matching nightly binaries are published to the rolling `nightly` GitHub prerelease, with notes and assets replaced on every successful build.

Pull a prebuilt image and run it without compiling locally:

```bash
export VIRTMGR_IMAGE=ghcr.io/xuthus5/virtmgr:nightly
docker compose pull
docker compose up -d --no-build
```

If the GHCR package is private, authenticate first with a token that has `read:packages`:

```bash
echo "$GHCR_TOKEN" | docker login ghcr.io -u "$GHCR_USER" --password-stdin
```

## Prerequisites

- A Linux host running libvirt and QEMU/KVM.
- Docker with Compose v2, or Podman with a Compose provider.
- Permission to read the VirtMgr configuration and connect to the host's libvirt sockets.
- A writable persistent directory or named volume for `/var/lib/virtmgr`.

VirtMgr connects to libvirt through its UNIX control sockets. It is intended for a libvirt daemon on the same host, not for an arbitrary remote libvirt endpoint.

## Build the image

From the repository root:

```bash
podman build \
  --build-arg VERSION="$(git describe --tags --always --dirty)" \
  --build-arg COMMIT="$(git rev-parse --short HEAD)" \
  --build-arg BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -f deploy/container/Dockerfile \
  -t virtmgr:local .
```

The equivalent Docker command uses the same arguments:

```bash
docker build -f deploy/container/Dockerfile -t virtmgr:local .
```

The final image runs as the system user `virtmgr`, embeds the frontend, and exposes port `8443`.

## Prepare `server.yaml`

```bash
cd deploy/container
cp ../server.yaml.example server.yaml
chmod 0640 server.yaml
```

At minimum, set:

```yaml
listen: "[::]:8443"
libvirt_uri: qemu:///system
database_path: /var/lib/virtmgr/virtmgr.db
host_name: virt-host-01
admin_username: admin
admin_password: replace-with-a-strong-password
cookie_secure: true
```

Remove `admin_password` after the first successful start. The password is hashed in SQLite and retained in the volume.

If TLS is terminated by nginx, Caddy, or Cloudflare Tunnel, set `cookie_secure: true`, route both HTTP and WebSocket traffic to the container, and keep the native `tls_cert_file` / `tls_key_file` fields empty.

## Libvirt sockets

The recommended mount is the complete libvirt runtime directory:

```yaml
volumes:
  - /run/libvirt:/run/libvirt
```

This works with both supported libvirt layouts and avoids stale bind mounts when a daemon recreates its socket:

- Monolithic: `/run/libvirt/libvirt-sock`.
- Modular: `/run/libvirt/virtqemud-sock`, `/run/libvirt/virtnetworkd-sock`, `/run/libvirt/virtstoraged-sock`, `/run/libvirt/virtnodedevd-sock`, `/run/libvirt/virtsecretd-sock`, `/run/libvirt/virtnwfilterd-sock`, and `/run/libvirt/virtinterfaced-sock`.

Do not mount only `libvirt-sock` on a modular-libvirt host. The UI may start, but VM, network, or storage operations can fail.

Check the host socket group and configure the supplementary group:

```bash
stat -c '%g %G %a %n' /run/libvirt/virtqemud-sock
export LIBVIRT_GID="$(stat -c '%g' /run/libvirt/virtqemud-sock)"
```

On a monolithic-libvirt host, use `/run/libvirt/libvirt-sock` in place of `virtqemud-sock`.

`compose.yaml` passes this value through `group_add`. The container process remains non-root; the supplementary group only grants access to the socket.

Do not mount `libvirt-admin-sock`, `virtqemud-admin-sock`, `virtlockd-sock`, or `virtlogd-sock`. They are not required by VirtMgr.

## Mount matrix

| Host source | Container path | Mode | Required | Purpose |
|---|---|---:|---|---|
| `/run/libvirt` | `/run/libvirt` | rw | Yes | libvirt control sockets |
| `./server.yaml` | `/etc/virtmgr/server.yaml` | ro | Yes | Runtime configuration |
| Named volume `virtmgr-data` | `/var/lib/virtmgr` | rw | Yes | SQLite, sessions, audit data, and metadata |
| `./tls` | `/etc/virtmgr/tls` | ro | Optional | Native TLS certificate and private key |
| Storage pool paths | Same absolute paths | rw/ro | Optional | Direct image, ISO, import, and backup file access |

### Storage paths

Mount every host storage path that VirtMgr must read or write directly at the same absolute path inside the container. For example:

```yaml
volumes:
  - /var/lib/libvirt/images:/var/lib/libvirt/images
  - /srv/virtmgr/iso:/srv/virtmgr/iso:ro
  - /srv/virtmgr/backups:/srv/virtmgr/backups
```

The path inside the container must match the path stored in libvirt or entered in the UI. Mounting `/var/lib/libvirt/images` at a different container path breaks volume imports and disk operations.

The container runs as a non-root user. If a mounted directory is not writable, grant the container user access with ownership or a POSIX ACL. Do not solve permission problems with `--privileged` or world-writable host directories.

### Native TLS

To let VirtMgr terminate TLS itself:

```yaml
volumes:
  - ./tls:/etc/virtmgr/tls:ro
```

Set both paths in `server.yaml`:

```yaml
tls_cert_file: /etc/virtmgr/tls/tls.crt
tls_key_file: /etc/virtmgr/tls/tls.key
cookie_secure: true
```

The certificate and key must be readable by the container user. Do not make the private key world-readable.

## Network modes

### Default bridge networking

The included Compose file publishes `8443:8443`. This is suitable for normal VM/storage management and a reverse proxy on the host.

Inbound port forwarding and NAT66 diagnostics use host firewall rules. A bridge container's `CAP_NET_ADMIN` applies to the container network namespace, not the host namespace. Use host networking when these features must work exactly as they do in a systemd deployment.

### Host networking for full functionality

For a single-host appliance, host networking is usually the better choice:

```yaml
services:
  virtmgr:
    network_mode: host
    # Remove the ports: mapping when host networking is enabled.
    cap_add:
      - NET_ADMIN
```

With host networking, `listen: "127.0.0.1:8443"` is appropriate when a reverse proxy runs on the host. Use `listen: "[::]:8443"` only when the service is protected by firewall and TLS policy.

## Run the image without Compose

Create the persistent volume and start the image directly:

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

The Docker equivalent uses `docker volume create` and `docker run` with the same arguments. Add the TLS and storage mounts as additional `-v` entries:

```bash
  -v "$PWD/tls:/etc/virtmgr/tls:ro" \
  -v /var/lib/libvirt/images:/var/lib/libvirt/images \
```

For host networking, replace `-p 8443:8443` with `--network host` and set `listen` accordingly in `server.yaml`.

## Start, inspect, and upgrade

```bash
cd deploy/container
docker compose up -d --build
docker compose ps
docker compose logs -f virtmgr
curl -fsSk https://127.0.0.1:8443/healthz
```

For Podman, replace `docker compose` with `podman compose`.

To upgrade after updating the repository:

```bash
docker compose build --pull
docker compose up -d
docker compose exec virtmgr /usr/local/bin/virtmgr version
```

Back up the VirtMgr SQLite database before an upgrade, preferably through the System Settings export action or after stopping the container.

## Account recovery inside the container

Run the password recovery command in the running container. It prompts for the new password without placing it in shell history:

```bash
docker compose exec virtmgr \
  /usr/local/bin/virtmgr reset-password \
  --config /etc/virtmgr/server.yaml \
  --username admin
```

## Security boundaries

- Do not use `--privileged`.
- Do not mount `/var/run/docker.sock` or the Podman socket.
- Do not mount `/dev/kvm`; QEMU runs on the host, not in the VirtMgr container.
- Do not mount the entire host root filesystem. Mount only required storage paths.
- Keep `server.yaml` and private keys read-only and readable only by the required users/groups.
- Keep `cookie_secure: true` whenever the UI is reached over HTTPS.
- Do not expose a writable VirtMgr instance directly to the public internet without TLS, a firewall, and a strong administrator password.

## Troubleshooting

### `Failed to connect socket` or permission denied

Confirm that `/run/libvirt` is mounted and that `LIBVIRT_GID` matches the host socket group. Verify inside the container:

```bash
docker compose exec virtmgr id
docker compose exec virtmgr ls -l /run/libvirt/virtqemud-sock
```

### VM or network operations fail on a modular-libvirt host

Replace a single `libvirt-sock` mount with `/run/libvirt:/run/libvirt` and restart the container.

### Network diagnostics warns about firewall access

Add `CAP_NET_ADMIN`. For host-level port forwarding and host firewall visibility, also enable `network_mode: host`.

### Storage path exists on the host but not in the UI

Mount the storage at the same absolute path inside the container and ensure the container user can access it.

### VNC console connects but the guest does not appear

VirtMgr proxies the console through libvirt. Do not publish QEMU VNC ports. Verify that the libvirt socket is mounted and that WebSocket traffic reaches the application through the reverse proxy.

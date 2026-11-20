#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKDIR="${VIRTMGR_MATRIX_WORKDIR:-/var/lib/libvirt/images/virtmgr-ipv6-matrix}"
NETWORK="${VIRTMGR_MATRIX_NETWORK:-virtmgr-ipv6-matrix}"
SSH_KEY="${WORKDIR}/id_ed25519"
GO_VERSION="${VIRTMGR_MATRIX_GO_VERSION:-1.27.4}"
GO_ARCHIVE="${WORKDIR}/go${GO_VERSION}.linux-amd64.tar.gz"
SOURCE_ARCHIVE="${WORKDIR}/virtmgr-source.tgz"

declare -A IMAGE_URL=(
  [ubuntu-22.04]="https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img"
  [ubuntu-24.04]="https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
  [debian-12]="https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-amd64.qcow2"
  [debian-13]="https://cloud.debian.org/images/cloud/trixie/latest/debian-13-genericcloud-amd64.qcow2"
  [rocky-9]="https://mirrors.nju.edu.cn/rocky/9/images/x86_64/Rocky-9-GenericCloud-Base.latest.x86_64.qcow2"
  [rocky-10]="https://mirrors.nju.edu.cn/rocky/10/images/x86_64/Rocky-10-GenericCloud-Base.latest.x86_64.qcow2"
)

declare -A VM_MAC=(
  [ubuntu-22.04]="52:54:00:61:22:04"
  [ubuntu-24.04]="52:54:00:61:24:04"
  [debian-12]="52:54:00:61:12:00"
  [debian-13]="52:54:00:61:13:00"
  [rocky-9]="52:54:00:61:09:00"
  [rocky-10]="52:54:00:61:10:00"
)

usage() {
  printf 'Usage: %s [all|%s ...]\n' "$0" "$(IFS=' '; printf '%s' "${!IMAGE_URL[*]}")"
}

if [[ $# -eq 0 ]]; then
  usage
  exit 1
fi
if [[ "$1" == "all" ]]; then
  set -- ubuntu-22.04 ubuntu-24.04 debian-12 debian-13 rocky-9 rocky-10
fi
for distro in "$@"; do
  if [[ -z "${IMAGE_URL[$distro]:-}" ]]; then
    printf 'Unsupported distribution: %s\n' "$distro" >&2
    usage >&2
    exit 1
  fi
done

for tool in virsh virt-install qemu-img cloud-localds ssh-keygen scp ssh curl tar sha256sum; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    printf '%s is required for the IPv6 distribution matrix\n' "$tool" >&2
    exit 1
  fi
done

mkdir -p "${WORKDIR}/images" "${WORKDIR}/seeds" "${WORKDIR}/reports"
if [[ ! -f "$SSH_KEY" ]]; then
  ssh-keygen -q -t ed25519 -N '' -f "$SSH_KEY"
fi
chmod 0600 "$SSH_KEY"

if ! virsh net-info "$NETWORK" >/dev/null 2>&1; then
  virsh net-define /dev/stdin <<XML
<network>
  <name>${NETWORK}</name>
  <forward mode='nat'><nat ipv6='yes'/></forward>
  <bridge name='virmatrix' stp='on' delay='0'/>
  <ip address='192.168.181.1' netmask='255.255.255.0'>
    <dhcp><range start='192.168.181.10' end='192.168.181.199'/></dhcp>
  </ip>
  <ip family='ipv6' address='fd00:181::1' prefix='64'>
    <dhcp><range start='fd00:181::10' end='fd00:181::ff'/></dhcp>
  </ip>
</network>
XML
  virsh net-autostart "$NETWORK"
fi
if ! virsh net-info "$NETWORK" | awk '$1 == "Active:" {print $2}' | grep -qx yes; then
  virsh net-start "$NETWORK"
fi

if [[ ! -f "$GO_ARCHIVE" ]]; then
  curl --noproxy '*' -fL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o "${GO_ARCHIVE}.partial"
  mv "${GO_ARCHIVE}.partial" "$GO_ARCHIVE"
fi

tar -C "$ROOT" \
  --exclude=.git --exclude=web/node_modules --exclude=web/dist --exclude=bin \
  -czf "${SOURCE_ARCHIVE}.partial" .
mv "${SOURCE_ARCHIVE}.partial" "$SOURCE_ARCHIVE"

ssh_vm() {
  local ip="$1"
  shift
  ssh -i "$SSH_KEY" -o IdentitiesOnly=yes -o BatchMode=yes -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null -o ConnectTimeout=10 "tester@$ip" "$@"
}

wait_for_ipv4() {
  local vm="$1" ip=""
  for _ in $(seq 1 120); do
    ip="$(virsh domifaddr "$vm" --source lease 2>/dev/null | awk '/ipv4/ {split($4, a, "/"); print a[1]; exit}')"
    if [[ -n "$ip" ]]; then
      printf '%s' "$ip"
      return 0
    fi
    sleep 2
  done
  return 1
}

wait_for_ssh() {
  local ip="$1"
  for _ in $(seq 1 120); do
    if ssh_vm "$ip" true >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  return 1
}

prepare_vm() {
  local distro="$1" family="$2" vm="$3" image="$4" overlay="$5" seed="$6"
  local user_data="${WORKDIR}/seeds/${distro}-user-data.yaml"
  local meta_data="${WORKDIR}/seeds/${distro}-meta-data.yaml"
  local groups="sudo"
  if [[ "$family" == "rhel" ]]; then
    groups="wheel"
  fi
  cat >"$user_data" <<EOF
#cloud-config
users:
  - name: tester
    groups: [${groups}]
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    ssh_authorized_keys:
      - $(cat "${SSH_KEY}.pub")
ssh_pwauth: false
disable_root: false
EOF
  cat >"$meta_data" <<EOF
instance-id: ${vm}
local-hostname: ${vm}
EOF
  cloud-localds "$seed" "$user_data" "$meta_data"

  if virsh dominfo "$vm" >/dev/null 2>&1; then
    virsh destroy "$vm" >/dev/null 2>&1 || true
    virsh undefine "$vm" >/dev/null 2>&1 || true
  fi
  qemu-img create -q -f qcow2 -F qcow2 -b "$image" "$overlay" 16G
  chown qemu:qemu "$overlay" "$seed" 2>/dev/null || true
  if ! virt-install --import --name "$vm" --memory 2048 --vcpus 2 --cpu host \
    --osinfo detect=on,require=off \
    --disk "path=${overlay},format=qcow2,bus=virtio" \
    --disk "path=${seed},device=cdrom" \
    --network "network=${NETWORK},model=virtio,mac=${VM_MAC[$distro]}" \
    --graphics none --noautoconsole >"${WORKDIR}/reports/${distro}-install.log" 2>&1; then
    cat "${WORKDIR}/reports/${distro}-install.log" >&2
    return 1
  fi
}

install_dependencies() {
  local family="$1" ip="$2"
  if [[ "$family" == "debian" ]]; then
    ssh_vm "$ip" "sudo bash -s" <<'EOF'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y --no-install-recommends qemu-system-x86 libvirt-daemon-system libvirt-clients libvirt-dev \
  dnsmasq iproute2 nftables iputils-ping curl ca-certificates build-essential pkg-config
apt-get install -y --no-install-recommends dhcpcd-base || apt-get install -y --no-install-recommends dhcpcd5
systemctl disable --now dnsmasq || true
systemctl enable --now libvirtd 2>/dev/null || systemctl enable --now virtqemud.socket virtnetworkd.socket
sysctl -w net.ipv6.conf.all.forwarding=1
EOF
  else
    ssh_vm "$ip" "sudo bash -s" <<'EOF'
set -euo pipefail
dnf install -y epel-release || true
dnf install -y dnf-plugins-core || true
crb enable || dnf config-manager --set-enabled crb || true
dnf install -y qemu-kvm libvirt-daemon-kvm libvirt-client libvirt-devel dnsmasq dhcpcd \
  iproute nftables iputils curl tar gcc make pkgconf-pkg-config ca-certificates
systemctl disable --now dnsmasq || true
systemctl disable --now firewalld || true
systemctl enable --now libvirtd 2>/dev/null || true
setenforce 0 2>/dev/null || true
sysctl -w net.ipv6.conf.all.forwarding=1
EOF
  fi
}

run_distro() {
  local distro="$1"
  local image="${WORKDIR}/images/${distro}-base.$(basename "${IMAGE_URL[$distro]}" | sed 's/.*\.//')"
  local vm="virtmgr-ipv6-${distro//./-}"
  local overlay="${WORKDIR}/images/${vm}.qcow2"
  local seed="${WORKDIR}/seeds/${distro}-seed.iso"
  local report="${WORKDIR}/reports/${distro}.log"
  local family="debian"
  if [[ "$distro" == rocky-* ]]; then
    family="rhel"
  fi

  printf '\n=== %s ===\n' "$distro"
  if [[ ! -s "$image" ]]; then
    printf 'Downloading %s\n' "${IMAGE_URL[$distro]}"
    curl --noproxy '*' -fL "${IMAGE_URL[$distro]}" -o "${image}.partial"
    mv "${image}.partial" "$image"
  fi
  printf 'image %s sha256 %s\n' "$image" "$(sha256sum "$image" | awk '{print $1}')"
  prepare_vm "$distro" "$family" "$vm" "$image" "$overlay" "$seed"

  local ip
  if ! ip="$(wait_for_ipv4 "$vm")"; then
    printf '%s did not obtain an IPv4 lease\n' "$distro" >&2
    return 1
  fi
  printf 'guest %s ip %s\n' "$vm" "$ip"
  if ! wait_for_ssh "$ip"; then
    printf '%s SSH did not become ready\n' "$distro" >&2
    return 1
  fi

  install_dependencies "$family" "$ip" | tee "${report}.install"
  scp -q -i "$SSH_KEY" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    "$GO_ARCHIVE" "tester@$ip:/tmp/go.tar.gz"
  ssh_vm "$ip" "sudo tar -C /usr/local -xzf /tmp/go.tar.gz"
  scp -q -i "$SSH_KEY" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    "$SOURCE_ARCHIVE" "tester@$ip:/tmp/virtmgr-source.tgz"
  ssh_vm "$ip" "sudo bash -s" <<'EOF'
set -euo pipefail
rm -rf /opt/virtmgr
mkdir -p /opt/virtmgr
tar -xzf /tmp/virtmgr-source.tgz -C /opt/virtmgr
chown -R tester:tester /opt/virtmgr
EOF

  printf 'outer IPv6: '
  if ssh_vm "$ip" "ping -6 -c 1 -W 3 2606:4700:4700::1111" >"${report}.outer-ipv6" 2>&1; then
    printf 'public egress OK\n'
  else
    printf 'public egress FAILED\n' >&2
    cat "${report}.outer-ipv6" >&2
    return 1
  fi

  if ssh_vm "$ip" "cd /opt/virtmgr && sudo env PATH=/usr/local/go/bin:/usr/sbin:/usr/bin:/sbin:/bin GOMODCACHE=/home/tester/go/pkg/mod GOCACHE=/home/tester/.cache/go-build GOSUMDB=sum.golang.org GOPROXY='${VIRTMGR_MATRIX_GOPROXY:-https://goproxy.cn,direct}' GOTOOLCHAIN=local VIRTMGR_TEST_REAL_LIBVIRT=1 scripts/ipv6-compat-check.sh" 2>&1 | tee "$report"; then
    printf '%s PASS\n' "$distro"
  else
    printf '%s FAIL\n' "$distro" >&2
    return 1
  fi
}

failures=0
for distro in "$@"; do
  set +e
  run_distro "$distro"
  status=$?
  set -e
  if (( status != 0 )); then
    failures=$((failures + 1))
    if [[ "${VIRTMGR_MATRIX_CONTINUE:-0}" != "1" ]]; then
      break
    fi
  fi
  if [[ "${VIRTMGR_MATRIX_KEEP_RUNNING:-0}" != "1" ]]; then
    virsh destroy "virtmgr-ipv6-${distro//./-}" >/dev/null 2>&1 || true
  fi
done

if (( failures > 0 )); then
  printf '\nIPv6 distribution matrix failed (%d failure(s)). Reports: %s/reports\n' "$failures" "$WORKDIR" >&2
  exit 1
fi
printf '\nIPv6 distribution matrix passed. Reports: %s/reports\n' "$WORKDIR"

#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

minimum_libvirt="8.0.0"
minimum_dnsmasq="2.85"
failures=0

normalize_version() {
  printf '%s' "$1" | sed -E 's/^[^0-9]*//; s/[^0-9.].*$//; s/\.$//'
}

version_at_least() {
  local actual normalized
  actual="$1"
  normalized="$(normalize_version "$actual")"
  [[ -n "$normalized" ]] || return 1
  printf '%s\n%s\n' "$2" "$normalized" | sort -V -C
}

report() {
  printf '%-24s %s\n' "$1" "$2"
}

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  failures=$((failures + 1))
}

warn() {
  printf 'WARN: %s\n' "$1" >&2
}

distro="unknown"
if [[ -r /etc/os-release ]]; then
  distro="$(. /etc/os-release && printf '%s %s' "${NAME:-unknown}" "${VERSION_ID:-}")"
fi
report "distribution" "$distro"
report "kernel" "$(uname -srm)"

if ! command -v virsh >/dev/null 2>&1; then
  fail "virsh is required to inspect the libvirt runtime"
else
  libvirt_version="$(virsh version --daemon 2>/dev/null | sed -n 's/^Using library: libvirt \([^ ]*\).*/\1/p' | head -n 1)"
  report "libvirt" "${libvirt_version:-unknown}"
  if [[ -z "$libvirt_version" ]]; then
    warn "could not identify the running libvirt version"
  elif ! version_at_least "$libvirt_version" "$minimum_libvirt"; then
    fail "libvirt $libvirt_version is below the supported baseline $minimum_libvirt"
  fi
fi

if ! command -v dnsmasq >/dev/null 2>&1; then
  fail "dnsmasq is required for managed DHCP/DHCPv6/RA networks"
else
  dnsmasq_version="$(dnsmasq --version 2>/dev/null | sed -n 's/^Dnsmasq version \([^ ]*\).*/\1/p' | head -n 1)"
  report "dnsmasq" "${dnsmasq_version:-unknown}"
  if [[ -z "$dnsmasq_version" ]]; then
    warn "could not identify the dnsmasq version"
  elif ! version_at_least "$dnsmasq_version" "$minimum_dnsmasq"; then
    fail "dnsmasq $dnsmasq_version is below the supported baseline $minimum_dnsmasq"
  fi
fi

if command -v nft >/dev/null 2>&1; then
  report "firewall backend" "$(nft --version 2>/dev/null | head -n 1)"
elif command -v ip6tables >/dev/null 2>&1; then
  report "firewall backend" "$(ip6tables --version 2>/dev/null | head -n 1)"
else
  warn "neither nftables nor ip6tables is installed; NAT66 firewall diagnostics cannot be completed"
fi

forwarding="$(cat /proc/sys/net/ipv6/conf/all/forwarding 2>/dev/null || printf 'unknown')"
report "IPv6 forwarding" "$forwarding"
if [[ "$forwarding" != "1" ]]; then
  warn "IPv6 forwarding is disabled; NAT66 clients cannot reach external IPv6 networks"
fi

if [[ "${VIRTMGR_COMPAT_SKIP_TESTS:-0}" != "1" ]]; then
  export GOSUMDB="${GOSUMDB:-sum.golang.org}"
  export VIRTMGR_TEST_REAL_LIBVIRT=1
  printf '\nRunning real libvirt IPv6 regression tests...\n'
  go test ./internal/platform/libvirt -run 'TestRealLibvirtIPv6' -count=1 -v
fi

if (( failures > 0 )); then
  printf '\nIPv6 compatibility check failed with %d blocking issue(s).\n' "$failures" >&2
  exit 1
fi
printf '\nIPv6 compatibility baseline satisfied. This check verifies only the current host; record each supported distribution/version combination separately.\n'

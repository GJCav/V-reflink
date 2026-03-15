#!/usr/bin/env bash
set -euo pipefail

missing=0

virtiofsd_bin=""
if command -v virtiofsd >/dev/null 2>&1; then
  virtiofsd_bin="$(command -v virtiofsd)"
elif [[ -x /usr/lib/qemu/virtiofsd ]]; then
  virtiofsd_bin="/usr/lib/qemu/virtiofsd"
fi

for bin in go ssh setsid wget ssh-keygen qemu-system-x86_64 qemu-img cloud-localds; do
  if ! command -v "${bin}" >/dev/null 2>&1; then
    echo "missing ${bin}" >&2
    missing=1
  fi
done

if [[ -z "${virtiofsd_bin}" ]]; then
  echo "missing virtiofsd (expected in PATH or /usr/lib/qemu/virtiofsd)" >&2
  missing=1
fi

if [[ ! -e /dev/kvm ]]; then
  echo "missing /dev/kvm" >&2
  missing=1
fi

if [[ ! -e /dev/vhost-vsock ]]; then
  echo "missing /dev/vhost-vsock (try: sudo modprobe vhost_vsock)" >&2
  missing=1
fi

if [[ "${missing}" -ne 0 ]]; then
  exit 1
fi

echo "vm prerequisites look good"

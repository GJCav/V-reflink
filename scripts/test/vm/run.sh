#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"

vm_disk="${VREFLINK_VM_DISK:-}"
vm_disk_format="${VREFLINK_VM_DISK_FORMAT:-}"
vm_firmware="${VREFLINK_VM_FIRMWARE:-uefi}"
vm_cid="${VREFLINK_VM_CID:-4}"
vm_ssh_port="${VREFLINK_VM_SSH_PORT:-2222}"
requested_share_root="${VREFLINK_VM_SHARE_ROOT:-}"
share_root=""
build_root="${repo_root}/.tmp/vm-integration/build"
runtime_root="${repo_root}/.tmp/vm-integration/runtime"
host_port="${VREFLINK_VM_HOST_PORT:-19090}"
guest_user="${VREFLINK_VM_SSH_USER:-}"
guest_key="${VREFLINK_VM_SSH_KEY:-}"
share_image=""
mounted_share=0

usage() {
  cat <<'EOF'
usage: scripts/test/vm/run.sh

Run the full virtiofs + vsock VM integration suite. The script will prepare the
guest image on demand, build the host and guest binaries, boot the guest, and
verify end-to-end reflink behavior over the shared tree.
EOF
}

fail() {
  echo "$*" >&2
  exit 1
}

case "${1:-}" in
  -h|--help|help)
    usage
    exit 0
    ;;
  "")
    ;;
  *)
    echo "unknown argument: $1" >&2
    usage >&2
    exit 1
    ;;
esac

daemon_pid=""
qemu_pid=""

cleanup() {
  if [[ -n "${daemon_pid}" ]]; then
    kill "${daemon_pid}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${qemu_pid}" ]]; then
    kill -TERM -- "-${qemu_pid}" >/dev/null 2>&1 || true
  fi
  if [[ "${mounted_share}" -eq 1 && -n "${share_root}" ]]; then
    sudo umount "${share_root}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

"${repo_root}/scripts/test/vm/check-prereqs.sh"

mkdir -p "${build_root}" "${runtime_root}"
run_root="$(mktemp -d "${runtime_root}/run.XXXXXX")"
prepared_env="${run_root}/prepared.env"

if [[ -z "${vm_disk}" || -z "${guest_user}" || -z "${guest_key}" ]]; then
  "${repo_root}/scripts/test/vm/prepare-image.sh" --write-env "${prepared_env}" >/dev/null
  # shellcheck disable=SC1090
  source "${prepared_env}"
  vm_disk="${VREFLINK_VM_DISK}"
  vm_disk_format="${VREFLINK_VM_DISK_FORMAT}"
  vm_firmware="${VREFLINK_VM_FIRMWARE}"
  guest_user="${VREFLINK_VM_SSH_USER}"
  guest_key="${VREFLINK_VM_SSH_KEY}"
fi

if [[ -z "${vm_disk_format}" ]]; then
  vm_disk_format="qcow2"
fi

overlay_disk="${run_root}/guest-overlay.qcow2"
seed_iso="${run_root}/seed.iso"
meta_data="${run_root}/meta-data"
user_data="${run_root}/user-data"
public_key="$(cat "${guest_key}.pub")"
instance_id="vreflink-$(date +%s)-$$"
invalid_log="${run_root}/vreflinkd-invalid.log"

if [[ -n "${requested_share_root}" ]]; then
  share_root="${requested_share_root}"
  mkdir -p "${share_root}"
else
  share_root="${run_root}/share"
  share_image="${run_root}/share.btrfs.img"
  mkdir -p "${share_root}"
  truncate -s 512M "${share_image}"
  mkfs.btrfs -q "${share_image}" >/dev/null
  sudo mount -o loop "${share_image}" "${share_root}"
  mounted_share=1
  sudo chown "$(id -u):$(id -g)" "${share_root}"
fi

mkdir -p "${share_root}/bin" "${share_root}/data"

qemu-img create -f qcow2 -F "${vm_disk_format}" -b "${vm_disk}" "${overlay_disk}" >/dev/null

cat > "${meta_data}" <<EOF
instance-id: ${instance_id}
local-hostname: vreflink-vm
EOF

cat > "${user_data}" <<EOF
#cloud-config
users:
  - name: ${guest_user}
    gecos: vreflink test user
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    groups: [adm, sudo]
    ssh_authorized_keys:
      - ${public_key}
package_update: false
package_upgrade: false
ssh_pwauth: false
disable_root: true
EOF

cloud-localds "${seed_iso}" "${user_data}" "${meta_data}"

rm -f "${share_root}/data/B" "${share_root}/data/rel-B" "${share_root}/data/escape-B"

go build -o "${build_root}/vreflink" "${repo_root}/cmd/vreflink"
go build -o "${build_root}/vreflinkd" "${repo_root}/cmd/vreflinkd"

cp "${build_root}/vreflink" "${share_root}/bin/vreflink"
chmod +x "${share_root}/bin/vreflink"
printf 'vm integration reflink payload\n' > "${share_root}/data/A"
printf 'vm integration relative reflink payload\n' > "${share_root}/data/rel-A"

set +e
timeout 5s "${build_root}/vreflinkd" \
  --share-root "${run_root}/missing-share" \
  --port "$((host_port + 1))" \
  > "${invalid_log}" 2>&1
invalid_status=$?
set -e

case "${invalid_status}" in
  0)
    fail "daemon unexpectedly started with an invalid share root"
    ;;
  124)
    cat "${invalid_log}" >&2 || true
    fail "daemon did not fail fast for an invalid share root"
    ;;
esac

if ! grep -q "does not exist" "${invalid_log}"; then
  cat "${invalid_log}" >&2 || true
  fail "invalid share root error did not mention the missing path"
fi

"${build_root}/vreflinkd" \
  --share-root "${share_root}" \
  --port "${host_port}" \
  > "${runtime_root}/vreflinkd.log" 2>&1 &
daemon_pid=$!

setsid "${repo_root}/scripts/test/vm/boot-qemu.sh" \
  --disk "${overlay_disk}" \
  --disk-format "${vm_disk_format}" \
  --seed-iso "${seed_iso}" \
  --firmware "${vm_firmware}" \
  --share "${share_root}" \
  --cid "${vm_cid}" \
  --ssh-port "${vm_ssh_port}" \
  > "${run_root}/qemu.log" 2>&1 &
qemu_pid=$!

ssh_base=(
  ssh
  -i "${guest_key}"
  -o BatchMode=yes
  -o StrictHostKeyChecking=no
  -o UserKnownHostsFile=/dev/null
  -p "${vm_ssh_port}"
  "${guest_user}@127.0.0.1"
)

run_guest() {
  "${ssh_base[@]}" "$@"
}

for _ in $(seq 1 90); do
  if run_guest true >/dev/null 2>&1; then
    break
  fi
  sleep 2
done

run_guest true >/dev/null 2>&1 || fail "guest SSH did not come up in time"

run_guest \
  "sudo mkdir -p /shared && if ! grep -q ' /shared virtiofs ' /proc/mounts; then sudo mount -t virtiofs shared /shared; fi"

run_guest \
  "chmod +x /shared/bin/vreflink && /shared/bin/vreflink --mount-root /shared --cid 2 --port ${host_port} /shared/data/A /shared/data/B"

cmp "${share_root}/data/A" "${share_root}/data/B"

run_guest \
  "cd /shared/data && /shared/bin/vreflink --mount-root /shared --cid 2 --port ${host_port} rel-A rel-B"

cmp "${share_root}/data/rel-A" "${share_root}/data/rel-B"

run_guest \
  "cd /shared/data && set +e; /shared/bin/vreflink --mount-root /shared --cid 2 --port ${host_port} ../../../etc/passwd escape-B > /tmp/vreflink-relative-escape.log 2>&1; status=\$?; set -e; test \$status -ne 0; grep -q 'guest mount root' /tmp/vreflink-relative-escape.log"

printf 'Z' | dd of="${share_root}/data/B" bs=1 seek=0 conv=notrunc status=none
if [[ "$(head -c 1 "${share_root}/data/A")" == "Z" ]]; then
  fail "source changed after destination write"
fi

echo "vm integration test passed"

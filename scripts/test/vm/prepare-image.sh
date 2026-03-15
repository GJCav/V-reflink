#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
asset_root="${VREFLINK_VM_ASSET_ROOT:-${repo_root}/.tmp/vm-assets/ubuntu-minimal}"
image_url="${VREFLINK_VM_BASE_IMAGE_URL:-https://cloud-images.ubuntu.com/minimal/releases/jammy/release/ubuntu-22.04-minimal-cloudimg-amd64.img}"
base_disk="${asset_root}/ubuntu-22.04-minimal-cloudimg-amd64.img"
ssh_key="${asset_root}/id_ed25519"
env_file=""

usage() {
  cat <<'EOF'
usage: prepare-image.sh [--write-env FILE]

Download the Ubuntu Minimal cloud image used by the VM suite, create the SSH
keypair if needed, and optionally write the resolved VM environment to FILE.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --write-env)
      env_file="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

mkdir -p "${asset_root}"

if [[ ! -f "${base_disk}" ]]; then
  wget -O "${base_disk}.tmp" "${image_url}"
  mv "${base_disk}.tmp" "${base_disk}"
fi

if [[ ! -f "${ssh_key}" ]]; then
  ssh-keygen -q -t ed25519 -N '' -f "${ssh_key}"
fi

env_contents=$(
  cat <<EOF
VREFLINK_VM_DISK=${base_disk}
VREFLINK_VM_DISK_FORMAT=qcow2
VREFLINK_VM_SSH_USER=vreflink
VREFLINK_VM_SSH_KEY=${ssh_key}
VREFLINK_VM_FIRMWARE=uefi
EOF
)

if [[ -n "${env_file}" ]]; then
  mkdir -p "$(dirname "${env_file}")"
  printf '%s\n' "${env_contents}" > "${env_file}"
fi

printf '%s\n' "${env_contents}"

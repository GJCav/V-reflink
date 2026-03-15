#!/usr/bin/env bash
set -euo pipefail

disk=""
disk_format="qcow2"
seed_iso=""
firmware="bios"
ovmf_code=""
ovmf_vars=""
share=""
cid=""
memory_mb=1024
cpus=2
ssh_port=2222
extra_args=()
qemu_runner=()
virtiofsd_runner=()

usage() {
  cat <<'EOF'
usage: boot-qemu.sh --disk DISK --share SHARE --cid CID [options]

required:
  --disk PATH         qcow2 or raw guest disk image
  --share PATH        host directory exported through virtiofs
  --cid N             guest vsock CID

optional:
  --memory MB         guest memory in MiB (default: 1024)
  --cpus N            vCPU count (default: 2)
  --ssh-port PORT     host port forwarded to guest 22 (default: 2222)
  --disk-format FMT   qemu disk format, for example qcow2 or raw (default: qcow2)
  --seed-iso PATH     cloud-init seed image attached as a second drive
  --firmware MODE     bios or uefi (default: bios)
  --ovmf-code PATH    UEFI code image; default auto-detected when firmware=uefi
  --ovmf-vars PATH    writable UEFI vars image; default auto-detected when firmware=uefi
  --extra-arg ARG     extra qemu argument, may be repeated
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --disk)
      disk="$2"
      shift 2
      ;;
    --share)
      share="$2"
      shift 2
      ;;
    --disk-format)
      disk_format="$2"
      shift 2
      ;;
    --seed-iso)
      seed_iso="$2"
      shift 2
      ;;
    --firmware)
      firmware="$2"
      shift 2
      ;;
    --ovmf-code)
      ovmf_code="$2"
      shift 2
      ;;
    --ovmf-vars)
      ovmf_vars="$2"
      shift 2
      ;;
    --cid)
      cid="$2"
      shift 2
      ;;
    --memory)
      memory_mb="$2"
      shift 2
      ;;
    --cpus)
      cpus="$2"
      shift 2
      ;;
    --ssh-port)
      ssh_port="$2"
      shift 2
      ;;
    --extra-arg)
      extra_args+=("$2")
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

if [[ -z "${disk}" || -z "${share}" || -z "${cid}" ]]; then
  usage >&2
  exit 1
fi

if ! command -v qemu-system-x86_64 >/dev/null 2>&1; then
  echo "qemu-system-x86_64 not found" >&2
  exit 1
fi

virtiofsd_bin=""
if command -v virtiofsd >/dev/null 2>&1; then
  virtiofsd_bin="$(command -v virtiofsd)"
elif [[ -x /usr/lib/qemu/virtiofsd ]]; then
  virtiofsd_bin="/usr/lib/qemu/virtiofsd"
else
  echo "virtiofsd not found" >&2
  exit 1
fi

runtime_dir="$(mktemp -d "${TMPDIR:-/tmp}/vreflink-vm.XXXXXX")"
socket_path="${runtime_dir}/virtiofsd.sock"
pid_path="${runtime_dir}/virtiofsd.pid"

if [[ ! -w /dev/kvm || ! -w /dev/vhost-vsock ]]; then
  if sudo -n true >/dev/null 2>&1; then
    qemu_runner=(sudo)
    virtiofsd_runner=(sudo)
  else
    echo "KVM or vhost-vsock requires elevated access, but sudo is not available" >&2
    exit 1
  fi
fi

if [[ ! -e /dev/vhost-vsock ]] && sudo -n true >/dev/null 2>&1; then
  sudo modprobe vhost_vsock || true
  sudo modprobe vsock || true
  sudo modprobe vmw_vsock_virtio_transport || true
fi

if [[ "${firmware}" == "uefi" ]]; then
  if [[ -z "${ovmf_code}" ]]; then
    for candidate in /usr/share/OVMF/OVMF_CODE.fd /usr/share/ovmf/OVMF.fd; do
      if [[ -f "${candidate}" ]]; then
        ovmf_code="${candidate}"
        break
      fi
    done
  fi

  if [[ -z "${ovmf_vars}" ]]; then
    for candidate in /usr/share/OVMF/OVMF_VARS.fd /usr/share/OVMF/OVMF_VARS.ms.fd; do
      if [[ -f "${candidate}" ]]; then
        ovmf_vars="${runtime_dir}/OVMF_VARS.fd"
        cp "${candidate}" "${ovmf_vars}"
        break
      fi
    done
  fi

  if [[ -z "${ovmf_code}" || -z "${ovmf_vars}" ]]; then
    echo "firmware=uefi requires usable OVMF code and vars images" >&2
    exit 1
  fi
fi

cleanup() {
  if [[ -f "${pid_path}" ]]; then
    "${virtiofsd_runner[@]}" kill "$(cat "${pid_path}")" >/dev/null 2>&1 || true
  fi
  rm -rf "${runtime_dir}"
}
trap cleanup EXIT

virtiofsd_cmd=(
  "${virtiofsd_bin}"
  -f
  "--socket-path=${socket_path}"
  -o "source=${share}"
  -o cache=auto
  -o sandbox=chroot
)

"${virtiofsd_runner[@]}" bash -c 'echo $$ > "$1"; shift; exec "$@"' _ "${pid_path}" "${virtiofsd_cmd[@]}" &

for _ in $(seq 1 100); do
  if [[ -S "${socket_path}" ]]; then
    break
  fi
  sleep 0.1
done

qemu_cmd=(
  qemu-system-x86_64
  -enable-kvm
  -machine q35,accel=kvm:tcg
  -cpu host
  -m "${memory_mb}"M
  -smp "${cpus}"
  -object memory-backend-memfd,id=mem,size="${memory_mb}"M,share=on
  -numa node,memdev=mem
  -drive "if=virtio,file=${disk},format=${disk_format}"
  -netdev "user,id=net0,hostfwd=tcp::${ssh_port}-:22"
  -device virtio-net-pci,netdev=net0
  -chardev "socket,id=char0,path=${socket_path}"
  -device vhost-user-fs-pci,chardev=char0,tag=shared
  -device vhost-vsock-pci,guest-cid="${cid}"
  -nographic
)

if [[ -n "${seed_iso}" ]]; then
  qemu_cmd+=(-drive "file=${seed_iso},format=raw,if=virtio")
fi

if [[ "${firmware}" == "uefi" ]]; then
  qemu_cmd+=(
    -drive "if=pflash,format=raw,readonly=on,file=${ovmf_code}"
    -drive "if=pflash,format=raw,file=${ovmf_vars}"
  )
fi

qemu_cmd+=("${extra_args[@]}")

"${qemu_runner[@]}" "${qemu_cmd[@]}"

#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

usage() {
  cat <<'EOF'
usage: scripts/test/run.sh <quick|btrfs|vm|release|all> [--race]

Suites:
  quick   Run the default fast Go test suite.
  btrfs   Run the local real-reflink integration suite (requires btrfs).
  vm      Run the full virtiofs + vsock VM integration suite.
  release Run the release tarball/.deb build and smoke tests.
  all     Run quick, then btrfs, then vm.

Options:
  --race  Enable the Go race detector for quick or btrfs.

Examples:
  scripts/test/run.sh quick
  scripts/test/run.sh quick --race
  scripts/test/run.sh btrfs
  scripts/test/run.sh vm
  scripts/test/run.sh release
EOF
}

fail() {
  echo "$*" >&2
  exit 1
}

require_go() {
  if ! command -v go >/dev/null 2>&1; then
    fail "missing go"
  fi
}

require_btrfs_workspace() {
  local fs_type
  fs_type="$(stat -f -c %T "${repo_root}")"
  if [[ "${fs_type}" != "btrfs" ]]; then
    fail "btrfs suite requires the repository workspace to be on btrfs (found ${fs_type})"
  fi
}

run_quick() {
  local -a cmd=(go test)
  if [[ "${1}" == "1" ]]; then
    cmd+=(-race)
  fi
  cmd+=(./...)

  (
    cd "${repo_root}"
    "${cmd[@]}"
  )
}

run_btrfs() {
  local -a cmd=(go test)
  require_btrfs_workspace
  if [[ "${1}" == "1" ]]; then
    cmd+=(-race)
  fi
  cmd+=(-tags btrfstest ./internal/service)

  (
    cd "${repo_root}"
    "${cmd[@]}"
  )
}

run_vm() {
  if [[ "${1}" == "1" ]]; then
    fail "the vm suite does not support --race; use 'quick --race' or 'btrfs --race' instead"
  fi

  (
    cd "${repo_root}"
    "${repo_root}/scripts/test/vm/run.sh"
  )
}

run_release() {
  if [[ "${1}" == "1" ]]; then
    fail "the release suite does not support --race"
  fi

  (
    cd "${repo_root}"
    "${repo_root}/scripts/test/release/run.sh"
  )
}

suite="${1:-help}"
if [[ $# -gt 0 ]]; then
  shift
fi

race=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --race)
      race=1
      shift
      ;;
    -h|--help|help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

require_go

case "${suite}" in
  quick)
    run_quick "${race}"
    ;;
  btrfs)
    run_btrfs "${race}"
    ;;
  vm)
    run_vm "${race}"
    ;;
  release)
    run_release "${race}"
    ;;
  all)
    if [[ "${race}" == "1" ]]; then
      fail "the all suite does not support --race because the vm and release suites do not run with race; run quick --race and btrfs --race separately"
    fi
    run_quick 0
    run_btrfs 0
    run_vm 0
    ;;
  help|-h|--help|"")
    usage
    ;;
  *)
    fail "unknown suite: ${suite}"
    ;;
esac

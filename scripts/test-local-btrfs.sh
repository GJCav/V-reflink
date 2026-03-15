#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fs_type="$(stat -f -c %T "${repo_root}")"

if [[ "${fs_type}" != "btrfs" ]]; then
  echo "repository is on ${fs_type}, not btrfs" >&2
  exit 1
fi

cd "${repo_root}"
if [[ "${1:-}" == "--race" ]]; then
  go test -race ./...
else
  go test ./...
fi

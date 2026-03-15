#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
prefix="${PREFIX:-/usr/local}"
build_dir="${repo_root}/.tmp/install-guest"

mkdir -p "${build_dir}"

go build -o "${build_dir}/vreflink" "${repo_root}/cmd/vreflink"
sudo install -Dm755 "${build_dir}/vreflink" "${prefix}/bin/vreflink"

echo "installed guest binary to ${prefix}/bin/vreflink"

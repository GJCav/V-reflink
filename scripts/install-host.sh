#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
prefix="${PREFIX:-/usr/local}"
systemd_dir="${SYSTEMD_DIR:-/etc/systemd/system}"
defaults_dir="${DEFAULTS_DIR:-/etc/default}"
build_dir="${repo_root}/.tmp/install-host"

mkdir -p "${build_dir}"

go build -o "${build_dir}/vreflinkd" "${repo_root}/cmd/vreflinkd"

sudo install -Dm755 "${build_dir}/vreflinkd" "${prefix}/bin/vreflinkd"
sudo install -Dm644 "${repo_root}/packaging/systemd/vreflinkd.service" "${systemd_dir}/vreflinkd.service"
sudo install -Dm644 "${repo_root}/packaging/systemd/vreflinkd.env" "${defaults_dir}/vreflinkd"

echo "installed host binary to ${prefix}/bin/vreflinkd"
echo "installed systemd unit to ${systemd_dir}/vreflinkd.service"
echo "installed defaults file to ${defaults_dir}/vreflinkd"
echo "next steps:"
echo "  sudo systemctl daemon-reload"
echo "  sudo systemctl enable --now vreflinkd"

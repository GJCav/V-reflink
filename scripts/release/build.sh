#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
version=""
arch="amd64"
out_dir="${repo_root}/dist"

usage() {
  cat <<'EOF'
usage: scripts/release/build.sh --version VERSION [--arch amd64] [--out-dir DIR]

Build the local release artifacts:
  - vreflink_VERSION_linux_amd64.tar.gz
  - vreflink_VERSION_amd64.deb
  - vreflink_VERSION_sha256sums.txt
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      version="$2"
      shift 2
      ;;
    --arch)
      arch="$2"
      shift 2
      ;;
    --out-dir)
      out_dir="$2"
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

if [[ -z "${version}" ]]; then
  echo "--version is required" >&2
  usage >&2
  exit 1
fi

if [[ "${arch}" != "amd64" ]]; then
  echo "only amd64 is supported in the current release pipeline" >&2
  exit 1
fi

for bin in go dpkg-deb tar sha256sum sed; do
  if ! command -v "${bin}" >/dev/null 2>&1; then
    echo "missing ${bin}" >&2
    exit 1
  fi
done

mkdir -p "${repo_root}/.tmp" "${out_dir}"
work_root="$(mktemp -d "${repo_root}/.tmp/release-build.XXXXXX")"
trap 'rm -rf "${work_root}"' EXIT

build_root="${work_root}/build"
tar_root_name="vreflink_${version}_linux_${arch}"
tar_root="${work_root}/${tar_root_name}"
deb_root="${work_root}/deb-root"
deb_control_dir="${deb_root}/DEBIAN"

mkdir -p "${build_root}" "${tar_root}" "${deb_control_dir}"

(
  cd "${repo_root}"
  CGO_ENABLED=0 GOOS=linux GOARCH="${arch}" go build -o "${build_root}/vreflink" ./cmd/vreflink
  CGO_ENABLED=0 GOOS=linux GOARCH="${arch}" go build -o "${build_root}/vreflinkd" ./cmd/vreflinkd
)

mkdir -p \
  "${tar_root}/usr/bin" \
  "${tar_root}/lib/systemd/system" \
  "${tar_root}/etc/default" \
  "${tar_root}/share/vreflink"

cp "${build_root}/vreflink" "${tar_root}/usr/bin/vreflink"
cp "${build_root}/vreflinkd" "${tar_root}/usr/bin/vreflinkd"
cp "${repo_root}/packaging/systemd/vreflinkd.service" "${tar_root}/lib/systemd/system/vreflinkd.service"
cp "${repo_root}/packaging/systemd/vreflinkd.env" "${tar_root}/etc/default/vreflinkd"
cp "${repo_root}/packaging/config/vreflink.env" "${tar_root}/share/vreflink/vreflink.env"
cp "${repo_root}/README.md" "${tar_root}/README.md"
cp "${repo_root}/LICENSE" "${tar_root}/LICENSE"

mkdir -p \
  "${deb_root}/usr/bin" \
  "${deb_root}/lib/systemd/system" \
  "${deb_root}/etc/default"

cp "${build_root}/vreflink" "${deb_root}/usr/bin/vreflink"
cp "${build_root}/vreflinkd" "${deb_root}/usr/bin/vreflinkd"
cp "${repo_root}/packaging/systemd/vreflinkd.service" "${deb_root}/lib/systemd/system/vreflinkd.service"
cp "${repo_root}/packaging/systemd/vreflinkd.env" "${deb_root}/etc/default/vreflinkd"

sed \
  -e "s/@VERSION@/${version}/g" \
  -e "s/@ARCH@/${arch}/g" \
  "${repo_root}/packaging/deb/control.template" \
  > "${deb_control_dir}/control"

tarball_path="${out_dir}/${tar_root_name}.tar.gz"
deb_path="${out_dir}/vreflink_${version}_${arch}.deb"
checksums_path="${out_dir}/vreflink_${version}_sha256sums.txt"

(
  cd "${work_root}"
  tar -czf "${tarball_path}" "${tar_root_name}"
)

dpkg-deb --root-owner-group --build "${deb_root}" "${deb_path}" >/dev/null

(
  cd "${out_dir}"
  sha256sum "$(basename "${tarball_path}")" "$(basename "${deb_path}")" > "$(basename "${checksums_path}")"
)

echo "built ${tarball_path}"
echo "built ${deb_path}"
echo "wrote ${checksums_path}"

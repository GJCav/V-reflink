#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
version="0.0.0"
artifact_dir="${repo_root}/.tmp/release-test/out"
root_dir="${repo_root}/.tmp/release-test/root"

for bin in go dpkg dpkg-deb tar sha256sum; do
  if ! command -v "${bin}" >/dev/null 2>&1; then
    echo "missing ${bin}" >&2
    exit 1
  fi
done

rm -rf "${artifact_dir}" "${root_dir}"
mkdir -p "${artifact_dir}" "${root_dir}/var/lib/dpkg/updates" "${root_dir}/var/log"
: > "${root_dir}/var/lib/dpkg/status"

"${repo_root}/scripts/release/build.sh" --version "${version}" --out-dir "${artifact_dir}" >/dev/null

tarball="${artifact_dir}/vreflink_${version}_linux_amd64.tar.gz"
deb="${artifact_dir}/vreflink_${version}_amd64.deb"
checksums="${artifact_dir}/vreflink_${version}_sha256sums.txt"

test -f "${tarball}"
test -f "${deb}"
test -f "${checksums}"

(
  cd "${artifact_dir}"
  sha256sum -c "$(basename "${checksums}")"
)

dpkg-deb -I "${deb}" | grep -F "Package: vreflink" >/dev/null
dpkg-deb -I "${deb}" | grep -F "Architecture: amd64" >/dev/null
dpkg-deb -I "${deb}" | grep -F "Version: ${version}" >/dev/null

dpkg-deb -c "${deb}" | grep -F "./usr/bin/vreflink" >/dev/null
dpkg-deb -c "${deb}" | grep -F "./usr/bin/vreflinkd" >/dev/null
dpkg-deb -c "${deb}" | grep -F "./lib/systemd/system/vreflinkd.service" >/dev/null
dpkg-deb -c "${deb}" | grep -F "./etc/default/vreflinkd" >/dev/null

tar -tzf "${tarball}" | grep -F "usr/bin/vreflink" >/dev/null
tar -tzf "${tarball}" | grep -F "usr/bin/vreflinkd" >/dev/null
tar -tzf "${tarball}" | grep -F "lib/systemd/system/vreflinkd.service" >/dev/null
tar -tzf "${tarball}" | grep -F "etc/default/vreflinkd" >/dev/null
tar -tzf "${tarball}" | grep -F "share/vreflink/vreflink.env" >/dev/null

dpkg \
  --root="${root_dir}" \
  --admindir="${root_dir}/var/lib/dpkg" \
  --log="${root_dir}/var/log/dpkg.log" \
  --force-not-root \
  --force-bad-path \
  --install "${deb}" >/dev/null

test -x "${root_dir}/usr/bin/vreflink"
test -x "${root_dir}/usr/bin/vreflinkd"
test -f "${root_dir}/lib/systemd/system/vreflinkd.service"
test -f "${root_dir}/etc/default/vreflinkd"

"${root_dir}/usr/bin/vreflink" --help >/dev/null
"${root_dir}/usr/bin/vreflinkd" --help >/dev/null

if [[ -e "${root_dir}/etc/systemd/system/multi-user.target.wants/vreflinkd.service" ]]; then
  echo "service was enabled by default" >&2
  exit 1
fi

echo "release packaging test passed"

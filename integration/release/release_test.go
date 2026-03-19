//go:build releasetest

package release

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GJCav/V-reflink/internal/devsupport"
	"github.com/GJCav/V-reflink/internal/releasebuild"
	"github.com/GJCav/V-reflink/internal/testsupport"
)

func TestReleaseArtifacts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	for _, bin := range []string{"go", "dpkg", "dpkg-deb"} {
		if _, ok := devsupport.LookPathAny(bin); !ok {
			t.Fatalf("missing %s", bin)
		}
	}

	outDir := testsupport.RepoTempDir(t, "release-test-out")
	artifacts, err := releasebuild.Build(ctx, releasebuild.Options{
		Version: "0.0.0",
		OutDir:  outDir,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	for _, path := range []string{artifacts.TarballPath, artifacts.DebPath, artifacts.ChecksumsPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing artifact %s: %v", path, err)
		}
	}

	if err := releasebuild.VerifyChecksums(artifacts.ChecksumsPath, artifacts.TarballPath, artifacts.DebPath); err != nil {
		t.Fatalf("VerifyChecksums() error = %v", err)
	}

	result, err := devsupport.RunCommand(ctx, "", nil, "dpkg-deb", "-I", artifacts.DebPath)
	if err != nil {
		t.Fatalf("dpkg-deb -I error = %v\n%s", err, result.Stderr)
	}
	for _, want := range []string{"Package: vreflink", "Architecture: amd64", "Version: 0.0.0"} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("dpkg-deb -I output missing %q", want)
		}
	}

	result, err = devsupport.RunCommand(ctx, "", nil, "dpkg-deb", "-c", artifacts.DebPath)
	if err != nil {
		t.Fatalf("dpkg-deb -c error = %v\n%s", err, result.Stderr)
	}
	for _, want := range []string{
		"./usr/bin/vreflink",
		"./usr/bin/vreflinkd",
		"./lib/systemd/system/vreflinkd.service",
		"./etc/vreflinkd/config.toml",
		"./usr/share/vreflink/config.toml",
	} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("dpkg-deb -c output missing %q", want)
		}
	}

	controlDir := testsupport.RepoTempDir(t, "release-test-control")
	result, err = devsupport.RunCommand(ctx, "", nil, "dpkg-deb", "-e", artifacts.DebPath, controlDir)
	if err != nil {
		t.Fatalf("dpkg-deb -e error = %v\n%s", err, result.Stderr)
	}
	if data, err := os.ReadFile(filepath.Join(controlDir, "conffiles")); err != nil {
		t.Fatalf("os.ReadFile(conffiles) error = %v", err)
	} else if strings.TrimSpace(string(data)) != "/etc/vreflinkd/config.toml" {
		t.Fatalf("conffiles = %q, want %q", strings.TrimSpace(string(data)), "/etc/vreflinkd/config.toml")
	}
	for _, path := range []string{
		filepath.Join(controlDir, "prerm"),
		filepath.Join(controlDir, "postrm"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("os.Stat(%s) error = %v", path, err)
		}
		if info.Mode()&0o111 == 0 {
			t.Fatalf("%s is not executable", path)
		}
	}

	entries, err := releasebuild.ReadTarballEntries(artifacts.TarballPath)
	if err != nil {
		t.Fatalf("ReadTarballEntries() error = %v", err)
	}
	rootPrefix := "vreflink_0.0.0_linux_amd64/"
	for _, want := range []string{
		rootPrefix + "usr/bin/vreflink",
		rootPrefix + "usr/bin/vreflinkd",
		rootPrefix + "lib/systemd/system/vreflinkd.service",
		rootPrefix + "etc/vreflinkd/config.toml",
		rootPrefix + "share/vreflink/config.toml",
	} {
		if !contains(entries, want) {
			t.Fatalf("tarball missing %q", want)
		}
	}

	rootDir := testsupport.RepoTempDir(t, "release-test-root")
	for _, dir := range []string{
		filepath.Join(rootDir, "var", "lib", "dpkg", "updates"),
		filepath.Join(rootDir, "var", "log"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("os.MkdirAll() error = %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(rootDir, "var", "lib", "dpkg", "status"), nil, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	stubDir := writeMaintainerStubCommands(t)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	result, err = runDpkg(ctx, rootDir, "--install", artifacts.DebPath)
	if err != nil {
		t.Fatalf("dpkg install error = %v\n%s", err, result.Stderr)
	}

	for _, path := range []string{
		filepath.Join(rootDir, "usr", "bin", "vreflink"),
		filepath.Join(rootDir, "usr", "bin", "vreflinkd"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("os.Stat(%s) error = %v", path, err)
		}
		if info.Mode()&0o111 == 0 {
			t.Fatalf("%s is not executable", path)
		}
	}
	for _, path := range []string{
		filepath.Join(rootDir, "lib", "systemd", "system", "vreflinkd.service"),
		filepath.Join(rootDir, "etc", "vreflinkd", "config.toml"),
		filepath.Join(rootDir, "usr", "share", "vreflink", "config.toml"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("os.Stat(%s) error = %v", path, err)
		}
	}

	for _, bin := range []string{
		filepath.Join(rootDir, "usr", "bin", "vreflink"),
		filepath.Join(rootDir, "usr", "bin", "vreflinkd"),
	} {
		result, err = devsupport.RunCommand(ctx, "", nil, bin, "--help")
		if err != nil {
			t.Fatalf("%s --help error = %v\n%s", bin, err, result.Stderr)
		}
	}

	if _, err := os.Stat(filepath.Join(rootDir, "etc", "systemd", "system", "multi-user.target.wants", "vreflinkd.service")); !os.IsNotExist(err) {
		t.Fatalf("service was enabled by default")
	}

	configPath := filepath.Join(rootDir, "etc", "vreflinkd", "config.toml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%s) error = %v", configPath, err)
	}
	configData = append(configData, []byte("\n# admin-edited during release test\n")...)
	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", configPath, err)
	}

	runningMarker := filepath.Join(rootDir, "run", "vreflinkd.running")
	if err := os.MkdirAll(filepath.Dir(runningMarker), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(runningMarker, []byte("running"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", runningMarker, err)
	}

	enabledPath := filepath.Join(rootDir, "etc", "systemd", "system", "multi-user.target.wants", "vreflinkd.service")
	if err := os.MkdirAll(filepath.Dir(enabledPath), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.Symlink("/lib/systemd/system/vreflinkd.service", enabledPath); err != nil {
		t.Fatalf("os.Symlink() error = %v", err)
	}

	result, err = runDpkg(ctx, rootDir, "--remove", "vreflink")
	if err != nil {
		t.Fatalf("dpkg remove error = %v\n%s", err, result.Stderr)
	}

	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config file did not survive remove: %v", err)
	}
	removedConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%s) error = %v", configPath, err)
	}
	if !strings.Contains(string(removedConfig), "admin-edited during release test") {
		t.Fatalf("config file lost admin edit after remove")
	}
	if _, err := os.Stat(enabledPath); !os.IsNotExist(err) {
		t.Fatalf("service enablement survived remove")
	}
	if _, err := os.Stat(runningMarker); !os.IsNotExist(err) {
		t.Fatalf("running marker survived remove")
	}
	if _, err := os.Stat(filepath.Join(rootDir, "lib", "systemd", "system", "vreflinkd.service")); !os.IsNotExist(err) {
		t.Fatalf("unit file survived remove")
	}

	toolLog, err := os.ReadFile(filepath.Join(rootDir, "var", "log", "maintainer-tools.log"))
	if err != nil {
		t.Fatalf("os.ReadFile(maintainer-tools.log) error = %v", err)
	}
	for _, want := range []string{
		"deb-systemd-invoke stop vreflinkd.service",
		"deb-systemd-helper disable vreflinkd.service",
		"systemctl --system daemon-reload",
	} {
		if !strings.Contains(string(toolLog), want) {
			t.Fatalf("maintainer tool log = %q, want %q", string(toolLog), want)
		}
	}

	result, err = runDpkg(ctx, rootDir, "--purge", "vreflink")
	if err != nil {
		t.Fatalf("dpkg purge error = %v\n%s", err, result.Stderr)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("config file survived purge")
	}

	toolLog, err = os.ReadFile(filepath.Join(rootDir, "var", "log", "maintainer-tools.log"))
	if err != nil {
		t.Fatalf("os.ReadFile(maintainer-tools.log) error = %v", err)
	}
	if !strings.Contains(string(toolLog), "deb-systemd-helper purge vreflinkd.service") {
		t.Fatalf("maintainer tool log = %q, want purge entry", string(toolLog))
	}
}

func contains(entries []string, want string) bool {
	for _, entry := range entries {
		if entry == want {
			return true
		}
	}
	return false
}

func runDpkg(ctx context.Context, rootDir string, args ...string) (devsupport.CommandResult, error) {
	baseArgs := []string{
		"--root=" + rootDir,
		"--admindir=" + filepath.Join(rootDir, "var", "lib", "dpkg"),
		"--log=" + filepath.Join(rootDir, "var", "log", "dpkg.log"),
		"--force-not-root",
		"--force-bad-path",
		"--force-script-chrootless",
	}
	baseArgs = append(baseArgs, args...)
	return devsupport.RunCommand(ctx, "", nil, "dpkg", baseArgs...)
}

func writeMaintainerStubCommands(t *testing.T) string {
	t.Helper()

	dir := testsupport.RepoTempDir(t, "release-test-stubs")
	writeExecutable(t, filepath.Join(dir, "deb-systemd-helper"), `#!/bin/sh
set -eu
log="${DPKG_ROOT}/var/log/maintainer-tools.log"
mkdir -p "$(dirname "$log")"
printf 'deb-systemd-helper' >> "$log"
for arg in "$@"; do
	printf ' %s' "$arg" >> "$log"
done
printf '\n' >> "$log"

case "${1:-}" in
disable)
	rm -f "${DPKG_ROOT}/etc/systemd/system/multi-user.target.wants/${2:-}"
	;;
purge)
	mkdir -p "${DPKG_ROOT}/run"
	: > "${DPKG_ROOT}/run/deb-systemd-helper-purged"
	;;
esac
`)
	writeExecutable(t, filepath.Join(dir, "deb-systemd-invoke"), `#!/bin/sh
set -eu
log="${DPKG_ROOT}/var/log/maintainer-tools.log"
mkdir -p "$(dirname "$log")"
printf 'deb-systemd-invoke' >> "$log"
for arg in "$@"; do
	printf ' %s' "$arg" >> "$log"
done
printf '\n' >> "$log"

case "${1:-}" in
stop)
	rm -f "${DPKG_ROOT}/run/vreflinkd.running"
	;;
esac
`)
	writeExecutable(t, filepath.Join(dir, "systemctl"), `#!/bin/sh
set -eu
log="${DPKG_ROOT}/var/log/maintainer-tools.log"
mkdir -p "$(dirname "$log")"
printf 'systemctl' >> "$log"
for arg in "$@"; do
	printf ' %s' "$arg" >> "$log"
done
printf '\n' >> "$log"

command="${1:-}"
if [ "$command" = "--system" ]; then
	shift
	command="${1:-}"
	shift || true
fi

case "$command" in
stop)
	rm -f "${DPKG_ROOT}/run/vreflinkd.running"
	;;
disable)
	rm -f "${DPKG_ROOT}/etc/systemd/system/multi-user.target.wants/${1:-}"
	;;
daemon-reload)
	mkdir -p "${DPKG_ROOT}/run"
	: > "${DPKG_ROOT}/run/systemd-daemon-reload"
	;;
esac
`)

	return dir
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", path, err)
	}
}

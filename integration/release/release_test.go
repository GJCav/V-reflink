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
	} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("dpkg-deb -c output missing %q", want)
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

	result, err = devsupport.RunCommand(
		ctx,
		"",
		nil,
		"dpkg",
		"--root="+rootDir,
		"--admindir="+filepath.Join(rootDir, "var", "lib", "dpkg"),
		"--log="+filepath.Join(rootDir, "var", "log", "dpkg.log"),
		"--force-not-root",
		"--force-bad-path",
		"--install",
		artifacts.DebPath,
	)
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
}

func contains(entries []string, want string) bool {
	for _, entry := range entries {
		if entry == want {
			return true
		}
	}
	return false
}

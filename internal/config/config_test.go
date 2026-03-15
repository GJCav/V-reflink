package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadCLIDefaultsWithoutConfigFile(t *testing.T) {
	t.Parallel()

	cfg, err := loadCLI(fixedUserConfigDir(t.TempDir()), emptyLookupEnv)
	if err != nil {
		t.Fatalf("loadCLI() error = %v", err)
	}

	if cfg.GuestMountRoot != defaultMountRoot {
		t.Fatalf("GuestMountRoot = %q, want %q", cfg.GuestMountRoot, defaultMountRoot)
	}
	if cfg.HostCID != defaultHostCID {
		t.Fatalf("HostCID = %d, want %d", cfg.HostCID, defaultHostCID)
	}
	if cfg.VsockPort != defaultPort {
		t.Fatalf("VsockPort = %d, want %d", cfg.VsockPort, defaultPort)
	}
	if cfg.Timeout != defaultTimeout {
		t.Fatalf("Timeout = %s, want %s", cfg.Timeout, defaultTimeout)
	}
}

func TestLoadCLIFromXDGConfigFile(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	writeCLIConfig(t, configDir, `
VREFLINK_GUEST_MOUNT_ROOT=/mnt/shared
VREFLINK_HOST_CID=7
VREFLINK_VSOCK_PORT=20000
VREFLINK_CLIENT_TIMEOUT=7s
`)

	cfg, err := loadCLI(fixedUserConfigDir(configDir), emptyLookupEnv)
	if err != nil {
		t.Fatalf("loadCLI() error = %v", err)
	}

	if cfg.GuestMountRoot != "/mnt/shared" {
		t.Fatalf("GuestMountRoot = %q, want %q", cfg.GuestMountRoot, "/mnt/shared")
	}
	if cfg.HostCID != 7 {
		t.Fatalf("HostCID = %d, want %d", cfg.HostCID, 7)
	}
	if cfg.VsockPort != 20000 {
		t.Fatalf("VsockPort = %d, want %d", cfg.VsockPort, 20000)
	}
	if cfg.Timeout != 7*time.Second {
		t.Fatalf("Timeout = %s, want %s", cfg.Timeout, 7*time.Second)
	}
}

func TestLoadCLIUsesXDGConfigHome(t *testing.T) {
	configDir := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("VREFLINK_GUEST_MOUNT_ROOT", "")
	t.Setenv("VREFLINK_HOST_CID", "")
	t.Setenv("VREFLINK_VSOCK_PORT", "")
	t.Setenv("VREFLINK_CLIENT_TIMEOUT", "")

	writeCLIConfig(t, configDir, `
VREFLINK_HOST_CID=13
`)

	cfg, err := LoadCLI()
	if err != nil {
		t.Fatalf("LoadCLI() error = %v", err)
	}

	if cfg.HostCID != 13 {
		t.Fatalf("HostCID = %d, want %d", cfg.HostCID, 13)
	}
}

func TestLoadCLIEnvironmentOverridesConfigFile(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	writeCLIConfig(t, configDir, `
VREFLINK_GUEST_MOUNT_ROOT=/mnt/shared
VREFLINK_HOST_CID=7
VREFLINK_VSOCK_PORT=20000
VREFLINK_CLIENT_TIMEOUT=7s
`)

	cfg, err := loadCLI(
		fixedUserConfigDir(configDir),
		mapLookupEnv(map[string]string{
			"VREFLINK_GUEST_MOUNT_ROOT": "/env/shared",
			"VREFLINK_HOST_CID":         "9",
			"VREFLINK_VSOCK_PORT":       "21000",
			"VREFLINK_CLIENT_TIMEOUT":   "9s",
		}),
	)
	if err != nil {
		t.Fatalf("loadCLI() error = %v", err)
	}

	if cfg.GuestMountRoot != "/env/shared" {
		t.Fatalf("GuestMountRoot = %q, want %q", cfg.GuestMountRoot, "/env/shared")
	}
	if cfg.HostCID != 9 {
		t.Fatalf("HostCID = %d, want %d", cfg.HostCID, 9)
	}
	if cfg.VsockPort != 21000 {
		t.Fatalf("VsockPort = %d, want %d", cfg.VsockPort, 21000)
	}
	if cfg.Timeout != 9*time.Second {
		t.Fatalf("Timeout = %s, want %s", cfg.Timeout, 9*time.Second)
	}
}

func TestLoadCLIMissingKeysUseDefaults(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	writeCLIConfig(t, configDir, `
VREFLINK_HOST_CID=7
`)

	cfg, err := loadCLI(fixedUserConfigDir(configDir), emptyLookupEnv)
	if err != nil {
		t.Fatalf("loadCLI() error = %v", err)
	}

	if cfg.GuestMountRoot != defaultMountRoot {
		t.Fatalf("GuestMountRoot = %q, want %q", cfg.GuestMountRoot, defaultMountRoot)
	}
	if cfg.HostCID != 7 {
		t.Fatalf("HostCID = %d, want %d", cfg.HostCID, 7)
	}
	if cfg.VsockPort != defaultPort {
		t.Fatalf("VsockPort = %d, want %d", cfg.VsockPort, defaultPort)
	}
	if cfg.Timeout != defaultTimeout {
		t.Fatalf("Timeout = %s, want %s", cfg.Timeout, defaultTimeout)
	}
}

func TestLoadCLIMalformedFileErrors(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	writeCLIConfig(t, configDir, `
this is not valid
`)

	_, err := loadCLI(fixedUserConfigDir(configDir), emptyLookupEnv)
	if err == nil {
		t.Fatal("loadCLI() unexpectedly succeeded")
	}
}

func TestLoadCLIInvalidTypedValueErrors(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	writeCLIConfig(t, configDir, `
VREFLINK_HOST_CID=abc
`)

	_, err := loadCLI(fixedUserConfigDir(configDir), emptyLookupEnv)
	if err == nil {
		t.Fatal("loadCLI() unexpectedly succeeded")
	}
}

func TestLoadCLIAcceptsExportCommentsAndBlankLines(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	writeCLIConfig(t, configDir, `
# guest config

export VREFLINK_GUEST_MOUNT_ROOT=/config/shared
export VREFLINK_HOST_CID=12

VREFLINK_VSOCK_PORT=23000
VREFLINK_CLIENT_TIMEOUT=11s
IGNORED_KEY=ignored
`)

	cfg, err := loadCLI(fixedUserConfigDir(configDir), emptyLookupEnv)
	if err != nil {
		t.Fatalf("loadCLI() error = %v", err)
	}

	if cfg.GuestMountRoot != "/config/shared" {
		t.Fatalf("GuestMountRoot = %q, want %q", cfg.GuestMountRoot, "/config/shared")
	}
	if cfg.HostCID != 12 {
		t.Fatalf("HostCID = %d, want %d", cfg.HostCID, 12)
	}
	if cfg.VsockPort != 23000 {
		t.Fatalf("VsockPort = %d, want %d", cfg.VsockPort, 23000)
	}
	if cfg.Timeout != 11*time.Second {
		t.Fatalf("Timeout = %s, want %s", cfg.Timeout, 11*time.Second)
	}
}

func TestLoadCLIUserConfigDirFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("boom")
	_, err := loadCLI(
		func() (string, error) {
			return "", want
		},
		emptyLookupEnv,
	)
	if err == nil {
		t.Fatal("loadCLI() unexpectedly succeeded")
	}
	if !errors.Is(err, want) {
		t.Fatalf("loadCLI() error = %v, want wrapped %v", err, want)
	}
}

func fixedUserConfigDir(dir string) func() (string, error) {
	return func() (string, error) {
		return dir, nil
	}
}

func emptyLookupEnv(string) (string, bool) {
	return "", false
}

func mapLookupEnv(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func writeCLIConfig(t *testing.T, configDir, contents string) string {
	t.Helper()

	path := filepath.Join(configDir, cliConfigDirName, cliConfigFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	return path
}

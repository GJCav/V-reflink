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
	if cfg.AuthToken != "" {
		t.Fatalf("AuthToken = %q, want empty", cfg.AuthToken)
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
VREFLINK_AUTH_TOKEN=token-from-config
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
	if cfg.AuthToken != "token-from-config" {
		t.Fatalf("AuthToken = %q, want %q", cfg.AuthToken, "token-from-config")
	}
}

func TestLoadCLIUsesXDGConfigHome(t *testing.T) {
	configDir := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("VREFLINK_GUEST_MOUNT_ROOT", "")
	t.Setenv("VREFLINK_HOST_CID", "")
	t.Setenv("VREFLINK_VSOCK_PORT", "")
	t.Setenv("VREFLINK_CLIENT_TIMEOUT", "")
	t.Setenv("VREFLINK_AUTH_TOKEN", "")

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
VREFLINK_AUTH_TOKEN=config-token
`)

	cfg, err := loadCLI(
		fixedUserConfigDir(configDir),
		mapLookupEnv(map[string]string{
			"VREFLINK_GUEST_MOUNT_ROOT": "/env/shared",
			"VREFLINK_HOST_CID":         "9",
			"VREFLINK_VSOCK_PORT":       "21000",
			"VREFLINK_CLIENT_TIMEOUT":   "9s",
			"VREFLINK_AUTH_TOKEN":       "env-token",
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
	if cfg.AuthToken != "env-token" {
		t.Fatalf("AuthToken = %q, want %q", cfg.AuthToken, "env-token")
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
	if cfg.AuthToken != "" {
		t.Fatalf("AuthToken = %q, want empty", cfg.AuthToken)
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
VREFLINK_AUTH_TOKEN=group-token
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
	if cfg.AuthToken != "group-token" {
		t.Fatalf("AuthToken = %q, want %q", cfg.AuthToken, "group-token")
	}
}

func TestLoadDaemonDefaultsAndEnvironment(t *testing.T) {
	t.Setenv("VREFLINK_SHARE_ROOT", "")
	t.Setenv("VREFLINK_VSOCK_PORT", "")
	t.Setenv("VREFLINK_READ_TIMEOUT", "")
	t.Setenv("VREFLINK_WRITE_TIMEOUT", "")
	t.Setenv("VREFLINK_TOKEN_MAP_PATH", "")
	t.Setenv("VREFLINK_ALLOW_V1_FALLBACK", "")

	cfg := LoadDaemon()
	if cfg.ShareRoot != defaultShareRoot {
		t.Fatalf("ShareRoot = %q, want %q", cfg.ShareRoot, defaultShareRoot)
	}
	if cfg.TokenMapPath != defaultTokenMap {
		t.Fatalf("TokenMapPath = %q, want %q", cfg.TokenMapPath, defaultTokenMap)
	}
	if cfg.AllowV1Fallback {
		t.Fatalf("AllowV1Fallback = %t, want false", cfg.AllowV1Fallback)
	}

	t.Setenv("VREFLINK_SHARE_ROOT", "/srv/custom")
	t.Setenv("VREFLINK_VSOCK_PORT", "21000")
	t.Setenv("VREFLINK_READ_TIMEOUT", "9s")
	t.Setenv("VREFLINK_WRITE_TIMEOUT", "11s")
	t.Setenv("VREFLINK_TOKEN_MAP_PATH", "/etc/vreflinkd/custom.yaml")
	t.Setenv("VREFLINK_ALLOW_V1_FALLBACK", "true")

	cfg = LoadDaemon()
	if cfg.ShareRoot != "/srv/custom" {
		t.Fatalf("ShareRoot = %q, want %q", cfg.ShareRoot, "/srv/custom")
	}
	if cfg.VsockPort != 21000 {
		t.Fatalf("VsockPort = %d, want %d", cfg.VsockPort, 21000)
	}
	if cfg.ReadTimeout != 9*time.Second {
		t.Fatalf("ReadTimeout = %s, want %s", cfg.ReadTimeout, 9*time.Second)
	}
	if cfg.WriteTimeout != 11*time.Second {
		t.Fatalf("WriteTimeout = %s, want %s", cfg.WriteTimeout, 11*time.Second)
	}
	if cfg.TokenMapPath != "/etc/vreflinkd/custom.yaml" {
		t.Fatalf("TokenMapPath = %q, want %q", cfg.TokenMapPath, "/etc/vreflinkd/custom.yaml")
	}
	if !cfg.AllowV1Fallback {
		t.Fatalf("AllowV1Fallback = %t, want true", cfg.AllowV1Fallback)
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

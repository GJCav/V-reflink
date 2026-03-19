package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GJCav/V-reflink/internal/auth"
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

func TestLoadCLIFromTOMLConfigFile(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	writeCLITOMLConfig(t, configDir, `
version = 1
mount_root = "/mnt/shared"
host_cid = 7
port = 20000
timeout = "7s"
token = "token-from-config"
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

	writeCLITOMLConfig(t, configDir, `
version = 1
host_cid = 13
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
	writeCLITOMLConfig(t, configDir, `
version = 1
mount_root = "/mnt/shared"
host_cid = 7
port = 20000
timeout = "7s"
token = "config-token"
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
	writeCLITOMLConfig(t, configDir, `
version = 1
host_cid = 7
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

func TestLoadCLILegacyEnvFallback(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	writeCLILegacyConfig(t, configDir, `
VREFLINK_GUEST_MOUNT_ROOT=/legacy/shared
VREFLINK_HOST_CID=12
VREFLINK_VSOCK_PORT=23000
VREFLINK_CLIENT_TIMEOUT=11s
VREFLINK_AUTH_TOKEN=legacy-token
`)

	cfg, err := loadCLI(fixedUserConfigDir(configDir), emptyLookupEnv)
	if err != nil {
		t.Fatalf("loadCLI() error = %v", err)
	}

	if cfg.GuestMountRoot != "/legacy/shared" {
		t.Fatalf("GuestMountRoot = %q, want %q", cfg.GuestMountRoot, "/legacy/shared")
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
	if cfg.AuthToken != "legacy-token" {
		t.Fatalf("AuthToken = %q, want %q", cfg.AuthToken, "legacy-token")
	}
}

func TestLoadCLITOMLWinsOverLegacyEnv(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	writeCLITOMLConfig(t, configDir, `
version = 1
host_cid = 7
`)
	writeCLILegacyConfig(t, configDir, `
VREFLINK_HOST_CID=12
`)

	cfg, err := loadCLI(fixedUserConfigDir(configDir), emptyLookupEnv)
	if err != nil {
		t.Fatalf("loadCLI() error = %v", err)
	}

	if cfg.HostCID != 7 {
		t.Fatalf("HostCID = %d, want %d", cfg.HostCID, 7)
	}
}

func TestLoadCLIMalformedFileErrors(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	writeCLITOMLConfig(t, configDir, `
version = [not-valid
`)

	_, err := loadCLI(fixedUserConfigDir(configDir), emptyLookupEnv)
	if err == nil {
		t.Fatal("loadCLI() unexpectedly succeeded")
	}
}

func TestLoadCLIInvalidTypedValueErrors(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	writeCLITOMLConfig(t, configDir, `
version = 1
host_cid = 7
timeout = "not-a-duration"
`)

	_, err := loadCLI(fixedUserConfigDir(configDir), emptyLookupEnv)
	if err == nil {
		t.Fatal("loadCLI() unexpectedly succeeded")
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

func TestLoadDaemonDefaultsFromFile(t *testing.T) {
	path := writeDaemonConfig(t, `
version = 1
share_root = "/srv/labshare"
`)

	cfg, err := loadDaemon(path)
	if err != nil {
		t.Fatalf("loadDaemon() error = %v", err)
	}

	if cfg.ShareRoot != defaultShareRoot {
		t.Fatalf("ShareRoot = %q, want %q", cfg.ShareRoot, defaultShareRoot)
	}
	if cfg.VsockPort != defaultPort {
		t.Fatalf("VsockPort = %d, want %d", cfg.VsockPort, defaultPort)
	}
	if cfg.ReadTimeout != defaultTimeout {
		t.Fatalf("ReadTimeout = %s, want %s", cfg.ReadTimeout, defaultTimeout)
	}
	if cfg.WriteTimeout != defaultTimeout {
		t.Fatalf("WriteTimeout = %s, want %s", cfg.WriteTimeout, defaultTimeout)
	}
	if cfg.LogLevel != defaultLogLevel {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, defaultLogLevel)
	}
	if cfg.AllowV1Fallback {
		t.Fatalf("AllowV1Fallback = %t, want false", cfg.AllowV1Fallback)
	}
}

func TestLoadDaemonConfigFromTOML(t *testing.T) {
	path := writeDaemonConfig(t, `
version = 1
share_root = "/srv/custom"
port = 21000
read_timeout = "9s"
write_timeout = "11s"
log_level = "debug"
allow_v1_fallback = true

[[tokens]]
name = "project-a"
token = "token-a"
uid = 1001
gid = 1002
groups = [44, 1002, 44]
`)

	cfg, err := loadDaemon(path)
	if err != nil {
		t.Fatalf("loadDaemon() error = %v", err)
	}

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
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if !cfg.AllowV1Fallback {
		t.Fatalf("AllowV1Fallback = %t, want true", cfg.AllowV1Fallback)
	}
	if len(cfg.Tokens) != 1 || cfg.Tokens[0].Token != "token-a" {
		t.Fatalf("Tokens = %#v, want one token entry", cfg.Tokens)
	}
}

func TestLoadDaemonMalformedFileErrors(t *testing.T) {
	path := writeDaemonConfig(t, `
version = [not-valid
`)

	if _, err := loadDaemon(path); err == nil {
		t.Fatal("loadDaemon() unexpectedly succeeded")
	}
}

func TestLoadDaemonMissingFileErrors(t *testing.T) {
	if _, err := loadDaemon(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Fatal("loadDaemon() unexpectedly succeeded")
	}
}

func TestLoadDaemonUnknownFieldsRejected(t *testing.T) {
	path := writeDaemonConfig(t, `
version = 1
share_root = "/srv/labshare"
extra = "nope"
`)

	if _, err := loadDaemon(path); err == nil {
		t.Fatal("loadDaemon() unexpectedly succeeded")
	}
}

func TestLoadDaemonPathUsesDefaultWhenEmpty(t *testing.T) {
	cfg := DefaultDaemon()
	if cfg.ShareRoot != defaultShareRoot {
		t.Fatalf("DefaultDaemon().ShareRoot = %q, want %q", cfg.ShareRoot, defaultShareRoot)
	}
}

func TestDaemonTokenEntriesUseAuthShape(t *testing.T) {
	path := writeDaemonConfig(t, `
version = 1
share_root = "/srv/labshare"

[[tokens]]
token = "only-token"
uid = 1001
gid = 1001
`)

	cfg, err := loadDaemon(path)
	if err != nil {
		t.Fatalf("loadDaemon() error = %v", err)
	}

	tokenMap, err := auth.NewTokenMapFromEntries(path, cfg.Tokens)
	if err != nil {
		t.Fatalf("NewTokenMapFromEntries() error = %v", err)
	}
	if _, ok := tokenMap.Resolve("only-token"); !ok {
		t.Fatal("Resolve() unexpectedly failed")
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

func writeCLITOMLConfig(t *testing.T, configDir, contents string) string {
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

func writeCLILegacyConfig(t *testing.T, configDir, contents string) string {
	t.Helper()

	path := filepath.Join(configDir, cliConfigDirName, cliLegacyConfigFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	return path
}

func writeDaemonConfig(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	return path
}

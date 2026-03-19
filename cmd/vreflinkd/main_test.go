package main

import (
	"bytes"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GJCav/V-reflink/internal/service"
	pkgassets "github.com/GJCav/V-reflink/packaging"
)

func TestSystemdUnitCommandPrintsTemplate(t *testing.T) {
	cmd := newRootCmd()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs([]string{"systemd-unit"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if output.String() != string(pkgassets.SystemdUnitTemplate()) {
		t.Fatalf("systemd unit output mismatch")
	}
}

func TestInstallCopiesExecutableAndTemplates(t *testing.T) {
	executablePath, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}

	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	systemdDir := filepath.Join(root, "systemd")
	configPath := filepath.Join(root, "config", "vreflinkd.toml")

	cmd := newRootCmd()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs([]string{
		"install",
		"--bin-dir", binDir,
		"--systemd-dir", systemdDir,
		"--config-path", configPath,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	installedBinary := filepath.Join(binDir, "vreflinkd")
	gotBinary, err := os.ReadFile(installedBinary)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	wantBinary, err := os.ReadFile(executablePath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if !bytes.Equal(gotBinary, wantBinary) {
		t.Fatal("installed binary does not match current executable")
	}

	gotService, err := os.ReadFile(filepath.Join(systemdDir, "vreflinkd.service"))
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(gotService) != string(pkgassets.SystemdUnitTemplate()) {
		t.Fatal("installed systemd unit does not match canonical template")
	}

	gotConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(gotConfig) != string(pkgassets.DaemonConfigTemplate()) {
		t.Fatal("installed config file does not match canonical template")
	}
}

func TestRootCommandRejectsInvalidShareRootBeforeListen(t *testing.T) {
	missingRoot := filepath.Join(t.TempDir(), "missing")
	configPath := writeDaemonConfig(t, `
version = 1
share_root = "`+missingRoot+`"
allow_v1_fallback = true
`)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"--config", configPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() unexpectedly succeeded")
	}

	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("Execute() error = %v, want missing-root validation", err)
	}
}

func TestRootCommandRejectsMalformedConfigBeforeListen(t *testing.T) {
	originalValidate := validateShareRoot
	originalListen := listenVsock
	t.Cleanup(func() {
		validateShareRoot = originalValidate
		listenVsock = originalListen
	})

	validateShareRoot = func(string, service.Reflinker) error {
		return nil
	}
	listenVsock = func(uint32) (net.Listener, error) {
		return nil, errors.New("listen should not be reached")
	}

	configPath := writeDaemonConfig(t, `
version = [not-valid
`)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"--config", configPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() unexpectedly succeeded")
	}

	if !strings.Contains(err.Error(), "parse "+configPath) {
		t.Fatalf("Execute() error = %v, want config parse failure", err)
	}
}

func TestRootCommandRejectsMissingTokenConfigByDefault(t *testing.T) {
	originalValidate := validateShareRoot
	originalListen := listenVsock
	t.Cleanup(func() {
		validateShareRoot = originalValidate
		listenVsock = originalListen
	})

	validateShareRoot = func(string, service.Reflinker) error {
		return nil
	}
	listenVsock = func(uint32) (net.Listener, error) {
		return nil, errors.New("listen should not be reached")
	}

	configPath := writeDaemonConfig(t, `
version = 1
share_root = "/srv/labshare"
`)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"--config", configPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() unexpectedly succeeded")
	}

	if !strings.Contains(err.Error(), "token configuration is required unless allow_v1_fallback=true") {
		t.Fatalf("Execute() error = %v, want fail-closed token-config error", err)
	}
}

func TestRootCommandAllowsMissingTokenConfigWhenFallbackEnabled(t *testing.T) {
	originalValidate := validateShareRoot
	originalListen := listenVsock
	t.Cleanup(func() {
		validateShareRoot = originalValidate
		listenVsock = originalListen
	})

	validateShareRoot = func(string, service.Reflinker) error {
		return nil
	}

	wantErr := errors.New("listen reached")
	listenVsock = func(uint32) (net.Listener, error) {
		return nil, wantErr
	}

	configPath := writeDaemonConfig(t, `
version = 1
share_root = "/srv/labshare"
allow_v1_fallback = true
`)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"--config", configPath})

	err := cmd.Execute()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestRootCommandRejectsLegacyRuntimeFlags(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--share-root", "/srv/labshare"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("Execute() error = %v, want unknown flag", err)
	}
}

func writeDaemonConfig(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	return path
}

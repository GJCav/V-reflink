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
	defaultsPath := filepath.Join(root, "defaults", "vreflinkd")

	cmd := newRootCmd()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs([]string{
		"install",
		"--bin-dir", binDir,
		"--systemd-dir", systemdDir,
		"--defaults-path", defaultsPath,
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

	gotDefaults, err := os.ReadFile(defaultsPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(gotDefaults) != string(pkgassets.DaemonDefaultsTemplate()) {
		t.Fatal("installed defaults file does not match canonical template")
	}
}

func TestRootCommandRejectsInvalidShareRootBeforeListen(t *testing.T) {
	missingRoot := filepath.Join(t.TempDir(), "missing")

	cmd := newRootCmd()
	cmd.SetArgs([]string{"--share-root", missingRoot})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() unexpectedly succeeded")
	}

	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("Execute() error = %v, want missing-root validation", err)
	}
}

func TestRootCommandRejectsMalformedTokenMapBeforeListen(t *testing.T) {
	originalValidate := validateShareRoot
	originalListen := listenVsock
	t.Cleanup(func() {
		validateShareRoot = originalValidate
		listenVsock = originalListen
	})
	shareRoot := t.TempDir()

	validateShareRoot = func(string, service.Reflinker) error {
		return nil
	}
	listenVsock = func(uint32) (net.Listener, error) {
		return nil, errors.New("listen should not be reached")
	}

	tokenMapPath := filepath.Join(t.TempDir(), "tokens.yaml")
	if err := os.WriteFile(tokenMapPath, []byte("version: [not-valid"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"--share-root", shareRoot,
		"--token-map-path", tokenMapPath,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() unexpectedly succeeded")
	}

	if !strings.Contains(err.Error(), "parse token map") {
		t.Fatalf("Execute() error = %v, want token-map parse failure", err)
	}
}

func TestRootCommandRejectsMissingTokenMapByDefault(t *testing.T) {
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

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"--share-root", t.TempDir(),
		"--token-map-path", filepath.Join(t.TempDir(), "missing.yaml"),
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() unexpectedly succeeded")
	}

	if !strings.Contains(err.Error(), "is required unless VREFLINK_ALLOW_V1_FALLBACK=true") {
		t.Fatalf("Execute() error = %v, want fail-closed token-map error", err)
	}
}

func TestRootCommandAllowsMissingTokenMapWhenFallbackEnabled(t *testing.T) {
	originalValidate := validateShareRoot
	originalListen := listenVsock
	t.Cleanup(func() {
		validateShareRoot = originalValidate
		listenVsock = originalListen
	})

	t.Setenv("VREFLINK_ALLOW_V1_FALLBACK", "true")

	validateShareRoot = func(string, service.Reflinker) error {
		return nil
	}

	wantErr := errors.New("listen reached")
	listenVsock = func(uint32) (net.Listener, error) {
		return nil, wantErr
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{
		"--share-root", t.TempDir(),
		"--token-map-path", filepath.Join(t.TempDir(), "missing.yaml"),
	})

	err := cmd.Execute()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	pkgassets "github.com/GJCav/V-reflink/packaging"
)

func TestSystemdUnitCommandPrintsTemplate(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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

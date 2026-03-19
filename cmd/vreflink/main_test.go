package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GJCav/V-reflink/internal/config"
	"github.com/GJCav/V-reflink/internal/protocol"
	pkgassets "github.com/GJCav/V-reflink/packaging"
)

func TestResolveRuntimeConfigFlagsOverrideLoadedValues(t *testing.T) {
	t.Parallel()

	cmd := newRootCmdWithConfig(config.CLI{
		GuestMountRoot: "/shared",
		HostCID:        2,
		VsockPort:      19090,
		Timeout:        5 * time.Second,
		AuthToken:      "config-token",
	})

	if err := cmd.ParseFlags([]string{
		"--mount-root", "/override",
		"--cid", "7",
		"--port", "20000",
		"--timeout", "7s",
		"--token", "flag-token",
	}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	cfg, err := resolveRuntimeConfig(cmd, func() (config.CLI, error) {
		return config.CLI{
			GuestMountRoot: "/shared",
			HostCID:        2,
			VsockPort:      19090,
			Timeout:        5 * time.Second,
			AuthToken:      "config-token",
		}, nil
	}, "/override", 7, 20000, 7*time.Second, "flag-token")
	if err != nil {
		t.Fatalf("resolveRuntimeConfig() error = %v", err)
	}

	if cfg.GuestMountRoot != "/override" {
		t.Fatalf("GuestMountRoot = %q, want %q", cfg.GuestMountRoot, "/override")
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
	if cfg.AuthToken != "flag-token" {
		t.Fatalf("AuthToken = %q, want %q", cfg.AuthToken, "flag-token")
	}
}

func TestConfigInitWritesTemplate(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	cmd := newRootCmd()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs([]string{"config", "init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	path := filepath.Join(configHome, "vreflink", "config.toml")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	if string(got) != string(pkgassets.GuestConfigTemplate()) {
		t.Fatalf("config template mismatch")
	}
}

func TestConfigInitRefusesOverwriteWithoutForce(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	path := filepath.Join(configHome, "vreflink", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{"config", "init"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() unexpectedly succeeded")
	}
}

func TestConfigInitForceOverwritesMalformedConfig(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	path := filepath.Join(configHome, "vreflink", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("not valid"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{"config", "init", "--force"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(got) != string(pkgassets.GuestConfigTemplate()) {
		t.Fatalf("config template mismatch after --force overwrite")
	}
}

func TestInstallCopiesCurrentExecutable(t *testing.T) {
	t.Parallel()

	executablePath, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}

	binDir := filepath.Join(t.TempDir(), "bin")
	cmd := newRootCmd()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs([]string{"install", "--bin-dir", binDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	installedPath := filepath.Join(binDir, "vreflink")
	got, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	want, err := os.ReadFile(executablePath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	if !bytes.Equal(got, want) {
		t.Fatal("installed binary does not match current executable")
	}
}

func TestCommandResolvesRelativePathsFromWorkingDirectory(t *testing.T) {
	t.Parallel()

	cfg := config.CLI{
		GuestMountRoot: "/shared",
		HostCID:        2,
		VsockPort:      19090,
		Timeout:        5 * time.Second,
		AuthToken:      "guest-token",
	}

	var gotReq protocol.Request
	cmd := newRootCmdWithDependencies(
		func() (config.CLI, error) { return cfg, nil },
		func() (string, error) { return "/shared/project", nil },
		func(_ context.Context, _ config.CLI, req protocol.Request) (*protocol.Response, error) {
			gotReq = req
			return &protocol.Response{OK: true}, nil
		},
	)
	cmd.SetArgs([]string{"src.txt", "nested/dst.txt"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if gotReq.Src != "project/src.txt" {
		t.Fatalf("Src = %q, want %q", gotReq.Src, "project/src.txt")
	}
	if gotReq.Dst != "project/nested/dst.txt" {
		t.Fatalf("Dst = %q, want %q", gotReq.Dst, "project/nested/dst.txt")
	}
	if gotReq.Version != protocol.Version2 {
		t.Fatalf("Version = %d, want %d", gotReq.Version, protocol.Version2)
	}
	if gotReq.Token != "guest-token" {
		t.Fatalf("Token = %q, want %q", gotReq.Token, "guest-token")
	}
}

func TestBuildRequestUsesVersion1WithoutToken(t *testing.T) {
	t.Parallel()

	req, err := buildRequest(config.CLI{GuestMountRoot: "/shared"}, "/shared/project", "src.txt", "dst.txt", false)
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}

	if req.Version != protocol.Version1 {
		t.Fatalf("Version = %d, want %d", req.Version, protocol.Version1)
	}
	if req.Token != "" {
		t.Fatalf("Token = %q, want empty", req.Token)
	}
}

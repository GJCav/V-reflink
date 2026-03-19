package main

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GJCav/V-reflink/internal/releasebuild"
)

func TestQuickSuiteUsesGoTest(t *testing.T) {
	var calls [][]string

	cmd := newRootCmdWithDeps(deps{
		repoRoot: func() (string, error) { return "/repo", nil },
		runCommand: func(_ context.Context, dir string, _ []string, _ io.Writer, _ io.Writer, name string, args ...string) error {
			if name != "go" {
				t.Fatalf("name = %q, want go", name)
			}
			if dir != "/repo" {
				t.Fatalf("dir = %q, want /repo", dir)
			}
			calls = append(calls, append([]string{name}, args...))
			return nil
		},
	})
	cmd.SetArgs([]string{"test", "quick"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("runCommand calls = %d, want 1", len(calls))
	}
	got := strings.Join(calls[0], " ")
	want := "go test ./..."
	if got != want {
		t.Fatalf("go invocation = %q, want %q", got, want)
	}
}

func TestReflinkFSSuiteAddsRace(t *testing.T) {
	var calls [][]string
	var envs [][]string
	cleanupCalled := false

	cmd := newRootCmdWithDeps(deps{
		repoRoot: func() (string, error) { return "/repo", nil },
		runCommand: func(_ context.Context, _ string, env []string, _ io.Writer, _ io.Writer, _ string, args ...string) error {
			calls = append(calls, append([]string(nil), args...))
			envs = append(envs, append([]string(nil), env...))
			return nil
		},
		prepareReflinkSuite: func(context.Context, string) (suitePreparation, error) {
			return suitePreparation{
				env: []string{"VREFLINK_TEST_REFLINK_ROOT=/scratch"},
				cleanup: func(context.Context) error {
					cleanupCalled = true
					return nil
				},
			}, nil
		},
	})
	cmd.SetArgs([]string{"test", "--race", "reflinkfs"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("runCommand calls = %d, want 1", len(calls))
	}
	got := strings.Join(calls[0], " ")
	want := "test -race -tags reflinkfstest ./internal/service"
	if got != want {
		t.Fatalf("go args = %q, want %q", got, want)
	}
	if !cleanupCalled {
		t.Fatal("cleanup was not called")
	}
	if !containsEnv(envs[0], "VREFLINK_TEST_REFLINK_ROOT=/scratch") {
		t.Fatalf("env = %q, want reflink root override", envs[0])
	}
}

func TestVMSuiteRejectsRace(t *testing.T) {
	cmd := newRootCmdWithDeps(deps{})
	cmd.SetArgs([]string{"test", "--race", "vm"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "does not support --race") {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestReleaseSuiteUsesTaggedGoTest(t *testing.T) {
	var calls [][]string

	cmd := newRootCmdWithDeps(deps{
		repoRoot: func() (string, error) { return "/repo", nil },
		runCommand: func(_ context.Context, _ string, _ []string, _ io.Writer, _ io.Writer, _ string, args ...string) error {
			calls = append(calls, append([]string(nil), args...))
			return nil
		},
	})
	cmd.SetArgs([]string{"test", "release"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("runCommand calls = %d, want 1", len(calls))
	}
	got := strings.Join(calls[0], " ")
	want := "test -count=1 -tags releasetest ./integration/release"
	if got != want {
		t.Fatalf("go args = %q, want %q", got, want)
	}
}

func TestAllRunsExpectedSuites(t *testing.T) {
	var calls [][]string

	cmd := newRootCmdWithDeps(deps{
		repoRoot: func() (string, error) { return "/repo", nil },
		runCommand: func(_ context.Context, _ string, _ []string, _ io.Writer, _ io.Writer, _ string, args ...string) error {
			calls = append(calls, append([]string(nil), args...))
			return nil
		},
		prepareReflinkSuite: func(context.Context, string) (suitePreparation, error) {
			return suitePreparation{}, nil
		},
		prepareVMSuite: func(context.Context, string) (suitePreparation, error) {
			return suitePreparation{}, nil
		},
	})
	cmd.SetArgs([]string{"test", "all"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := []string{
		strings.Join(calls[0], " "),
		strings.Join(calls[1], " "),
		strings.Join(calls[2], " "),
	}
	want := []string{
		"test ./...",
		"test -tags reflinkfstest ./internal/service",
		"test -count=1 -tags vmtest ./integration/vm",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("call %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestVMSuitePreparesShareRootWhenUnset(t *testing.T) {
	t.Setenv("VREFLINK_VM_SHARE_ROOT", "")

	var envs [][]string
	cleanupCalled := false

	cmd := newRootCmdWithDeps(deps{
		repoRoot: func() (string, error) { return "/repo", nil },
		runCommand: func(_ context.Context, _ string, env []string, _ io.Writer, _ io.Writer, _ string, _ ...string) error {
			envs = append(envs, append([]string(nil), env...))
			return nil
		},
		prepareVMSuite: func(context.Context, string) (suitePreparation, error) {
			return suitePreparation{
				env: []string{"VREFLINK_VM_SHARE_ROOT=/vm-share"},
				cleanup: func(context.Context) error {
					cleanupCalled = true
					return nil
				},
			}, nil
		},
	})
	cmd.SetArgs([]string{"test", "vm"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(envs) != 1 {
		t.Fatalf("runCommand calls = %d, want 1", len(envs))
	}
	if !cleanupCalled {
		t.Fatal("cleanup was not called")
	}
	if !containsEnv(envs[0], "VREFLINK_VM_SHARE_ROOT=/vm-share") {
		t.Fatalf("env = %q, want vm share root override", envs[0])
	}
}

func TestReleaseBuildPrintsArtifacts(t *testing.T) {
	output := &bytes.Buffer{}

	cmd := newRootCmdWithDeps(deps{
		buildRelease: func(_ context.Context, opts releasebuild.Options) (releasebuild.Artifacts, error) {
			if opts.Version != "1.2.3" {
				t.Fatalf("Version = %q, want 1.2.3", opts.Version)
			}
			if opts.Arch != "amd64" {
				t.Fatalf("Arch = %q, want amd64", opts.Arch)
			}
			if opts.OutDir != "./dist" {
				t.Fatalf("OutDir = %q, want ./dist", opts.OutDir)
			}
			return releasebuild.Artifacts{
				TarballPath:   filepath.Join("/tmp", "a.tar.gz"),
				DebPath:       filepath.Join("/tmp", "a.deb"),
				ChecksumsPath: filepath.Join("/tmp", "checksums.txt"),
			}, nil
		},
	})
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetArgs([]string{"release", "build", "--version", "1.2.3", "--out-dir", "./dist"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	text := output.String()
	for _, want := range []string{"built /tmp/a.tar.gz", "built /tmp/a.deb", "wrote /tmp/checksums.txt"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output = %q, want %q", text, want)
		}
	}
}

func TestVMCheckPrereqsReportsIssues(t *testing.T) {
	stderr := &bytes.Buffer{}

	cmd := newRootCmdWithDeps(deps{
		checkVMPrereqs: func(context.Context) []string {
			return []string{"missing qemu-system-x86_64", "missing /dev/kvm"}
		},
	})
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"vm", "check-prereqs"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "not satisfied") {
		t.Fatalf("Execute() error = %v", err)
	}

	text := stderr.String()
	for _, want := range []string{"missing qemu-system-x86_64", "missing /dev/kvm"} {
		if !strings.Contains(text, want) {
			t.Fatalf("stderr = %q, want %q", text, want)
		}
	}
}

func containsEnv(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}

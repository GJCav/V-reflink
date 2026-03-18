package testsupport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePreparedReflinkTestRootRequiresEnv(t *testing.T) {
	t.Setenv(ReflinkTestRootEnv, "")

	_, err := ResolvePreparedReflinkTestRoot()
	if err == nil {
		t.Fatal("ResolvePreparedReflinkTestRoot() unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), ReflinkTestRootEnv) {
		t.Fatalf("error = %q, want %q", err, ReflinkTestRootEnv)
	}
	if !strings.Contains(err.Error(), "go run ./cmd/vreflink-dev test reflinkfs") {
		t.Fatalf("error = %q, want runner guidance", err)
	}
}

func TestResolvePreparedVMShareRootRejectsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "share-root-file")
	if err := os.WriteFile(path, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	t.Setenv("VREFLINK_VM_SHARE_ROOT", path)

	_, err := ResolvePreparedVMShareRoot()
	if err == nil {
		t.Fatal("ResolvePreparedVMShareRoot() unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "VREFLINK_VM_SHARE_ROOT") {
		t.Fatalf("error = %q, want env guidance", err)
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("error = %q, want directory validation", err)
	}
}

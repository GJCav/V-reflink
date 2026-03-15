package testsupport

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/GJCav/V-reflink/internal/protocol"
)

const btrfsSuperMagic = 0x9123683e

type CopyReflinker struct{}

func (CopyReflinker) Reflink(srcPath, dstPath string, mode fs.FileMode) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	return os.WriteFile(dstPath, data, mode.Perm())
}

func AssertCode(t *testing.T, err error, want string) {
	t.Helper()

	if got := CodeOf(err); got != want {
		t.Fatalf("error code = %q, want %q (err=%v)", got, want, err)
	}
}

func CodeOf(err error) string {
	if coded, ok := protocol.AsCoded(err); ok {
		return coded.Code
	}

	return ""
}

func RepoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func RepoTempDir(t *testing.T, suite string) string {
	t.Helper()

	base := filepath.Join(RepoRoot(t), ".tmp", suite)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	dir, err := os.MkdirTemp(base, "case-")
	if err != nil {
		t.Fatalf("os.MkdirTemp() error = %v", err)
	}

	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	return dir
}

func RequireBtrfs(t *testing.T, path string) {
	t.Helper()

	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		t.Fatalf("unix.Statfs() error = %v", err)
	}

	if stat.Type != btrfsSuperMagic {
		t.Skipf("%s is not on btrfs", path)
	}
}

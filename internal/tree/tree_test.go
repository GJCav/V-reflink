package tree

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/GJCav/V-reflink/internal/testsupport"
)

func TestReflinkTreeSuccess(t *testing.T) {
	root := t.TempDir()
	srcRoot := filepath.Join(root, "src")
	dstRoot := filepath.Join(root, "dst")

	if err := os.MkdirAll(filepath.Join(srcRoot, "nested"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcRoot, "nested", "file.txt"), []byte("tree"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	walker := Walker{Reflinker: testsupport.CopyReflinker{}}
	if err := walker.ReflinkTree(srcRoot, dstRoot); err != nil {
		t.Fatalf("ReflinkTree() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dstRoot, "nested", "file.txt"))
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	if string(got) != "tree" {
		t.Fatalf("dst file = %q, want %q", got, "tree")
	}
}

func TestReflinkTreeRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	srcRoot := filepath.Join(root, "src")
	dstRoot := filepath.Join(root, "dst")

	if err := os.MkdirAll(srcRoot, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcRoot, "real.txt"), []byte("data"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := os.Symlink("real.txt", filepath.Join(srcRoot, "link.txt")); err != nil {
		t.Fatalf("os.Symlink() error = %v", err)
	}

	walker := Walker{Reflinker: testsupport.CopyReflinker{}}
	if err := walker.ReflinkTree(srcRoot, dstRoot); err == nil {
		t.Fatal("ReflinkTree() unexpectedly succeeded")
	}
}

func TestReflinkTreePreservesExactDirectoryModes(t *testing.T) {
	root := t.TempDir()
	srcRoot := filepath.Join(root, "src")
	srcNested := filepath.Join(srcRoot, "nested")
	dstRoot := filepath.Join(root, "dst")

	withUmask(t, 0o022)

	if err := os.MkdirAll(srcNested, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcNested, "file.txt"), []byte("tree"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	wantRootMode := fs.FileMode(0o775) | fs.ModeSetgid
	wantNestedMode := fs.FileMode(0o770) | fs.ModeSticky
	setExactMode(t, srcRoot, wantRootMode)
	setExactMode(t, srcNested, wantNestedMode)

	walker := Walker{Reflinker: testsupport.CopyReflinker{}}
	if err := walker.ReflinkTree(srcRoot, dstRoot); err != nil {
		t.Fatalf("ReflinkTree() error = %v", err)
	}

	assertExactMode(t, dstRoot, wantRootMode)
	assertExactMode(t, filepath.Join(dstRoot, "nested"), wantNestedMode)
}

func TestReflinkTreeRestoresDirectoryModesAfterPartialFailure(t *testing.T) {
	root := t.TempDir()
	srcRoot := filepath.Join(root, "src")
	srcNested := filepath.Join(srcRoot, "nested")
	dstRoot := filepath.Join(root, "dst")
	failPath := filepath.Join(srcNested, "fail.txt")

	withUmask(t, 0o022)

	if err := os.MkdirAll(srcNested, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(failPath, []byte("boom"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	wantRootMode := fs.FileMode(0o775) | fs.ModeSetgid
	wantNestedMode := fs.FileMode(0o770) | fs.ModeSticky
	setExactMode(t, srcRoot, wantRootMode)
	setExactMode(t, srcNested, wantNestedMode)

	wantErr := errors.New("reflink failed")
	walker := Walker{Reflinker: failOnPathReflinker{
		failPath: failPath,
		err:      wantErr,
	}}

	err := walker.ReflinkTree(srcRoot, dstRoot)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ReflinkTree() error = %v, want %v", err, wantErr)
	}

	assertExactMode(t, dstRoot, wantRootMode)
	assertExactMode(t, filepath.Join(dstRoot, "nested"), wantNestedMode)
	if _, err := os.Lstat(filepath.Join(dstRoot, "nested", "fail.txt")); !os.IsNotExist(err) {
		t.Fatalf("failed output exists unexpectedly: %v", err)
	}
}

type failOnPathReflinker struct {
	failPath string
	err      error
}

func (r failOnPathReflinker) Reflink(srcPath, dstPath string, mode fs.FileMode) error {
	if srcPath == r.failPath {
		return r.err
	}

	return testsupport.CopyReflinker{}.Reflink(srcPath, dstPath, mode)
}

func withUmask(t *testing.T, mask int) {
	t.Helper()

	old := syscall.Umask(mask)
	t.Cleanup(func() {
		syscall.Umask(old)
	})
}

func setExactMode(t *testing.T, path string, mode fs.FileMode) {
	t.Helper()

	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("os.Chmod(%s) error = %v", path, err)
	}
}

func assertExactMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat(%s) error = %v", path, err)
	}

	if got := exactDirectoryMode(info.Mode()); got != want {
		t.Fatalf("%s mode = %#o, want %#o", path, got, want)
	}
}

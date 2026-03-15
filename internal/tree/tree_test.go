package tree

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

type copyReflinker struct{}

func (copyReflinker) Reflink(srcPath, dstPath string, mode fs.FileMode) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	return os.WriteFile(dstPath, data, mode.Perm())
}

func TestReflinkTreeSuccess(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	srcRoot := filepath.Join(root, "src")
	dstRoot := filepath.Join(root, "dst")

	if err := os.MkdirAll(filepath.Join(srcRoot, "nested"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcRoot, "nested", "file.txt"), []byte("tree"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	walker := Walker{Reflinker: copyReflinker{}}
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
	t.Parallel()

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

	walker := Walker{Reflinker: copyReflinker{}}
	if err := walker.ReflinkTree(srcRoot, dstRoot); err == nil {
		t.Fatal("ReflinkTree() unexpectedly succeeded")
	}
}

//go:build !reflinkfstest && !vmtest

package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GJCav/V-reflink/internal/protocol"
	"github.com/GJCav/V-reflink/internal/testsupport"
)

func TestExecuteSingleFile(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		root := t.TempDir()
		srcPath := filepath.Join(root, "src.txt")
		if err := os.WriteFile(srcPath, []byte("hello"), 0o644); err != nil {
			t.Fatalf("os.WriteFile() error = %v", err)
		}

		svc := newMockService(root)
		err := svc.Execute(protocol.Request{
			Version: protocol.Version1,
			Op:      protocol.OpReflink,
			Src:     "src.txt",
			Dst:     "dst.txt",
		})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		got, err := os.ReadFile(filepath.Join(root, "dst.txt"))
		if err != nil {
			t.Fatalf("os.ReadFile() error = %v", err)
		}

		if string(got) != "hello" {
			t.Fatalf("dst.txt = %q, want %q", got, "hello")
		}
	})

	t.Run("destination exists", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "src.txt"), []byte("hello"), 0o644); err != nil {
			t.Fatalf("os.WriteFile() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "dst.txt"), []byte("exists"), 0o644); err != nil {
			t.Fatalf("os.WriteFile() error = %v", err)
		}

		svc := newMockService(root)
		err := svc.Execute(protocol.Request{
			Version: protocol.Version1,
			Op:      protocol.OpReflink,
			Src:     "src.txt",
			Dst:     "dst.txt",
		})
		if err == nil {
			t.Fatal("Execute() unexpectedly succeeded")
		}

		testsupport.AssertCode(t, err, protocol.CodeEEXIST)
	})

	t.Run("rejects symlink source", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "real.txt"), []byte("hello"), 0o644); err != nil {
			t.Fatalf("os.WriteFile() error = %v", err)
		}
		if err := os.Symlink("real.txt", filepath.Join(root, "link.txt")); err != nil {
			t.Fatalf("os.Symlink() error = %v", err)
		}

		svc := newMockService(root)
		err := svc.Execute(protocol.Request{
			Version: protocol.Version1,
			Op:      protocol.OpReflink,
			Src:     "link.txt",
			Dst:     "dst.txt",
		})
		if err == nil {
			t.Fatal("Execute() unexpectedly succeeded")
		}

		testsupport.AssertCode(t, err, protocol.CodeEINVAL)
	})

	t.Run("rejects hardlink source", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "src.txt"), []byte("hello"), 0o644); err != nil {
			t.Fatalf("os.WriteFile() error = %v", err)
		}
		if err := os.Link(filepath.Join(root, "src.txt"), filepath.Join(root, "src-link.txt")); err != nil {
			t.Fatalf("os.Link() error = %v", err)
		}

		svc := newMockService(root)
		err := svc.Execute(protocol.Request{
			Version: protocol.Version1,
			Op:      protocol.OpReflink,
			Src:     "src.txt",
			Dst:     "dst.txt",
		})
		if err == nil {
			t.Fatal("Execute() unexpectedly succeeded")
		}

		testsupport.AssertCode(t, err, protocol.CodeEINVAL)
	})
}

func TestExecuteRecursive(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "src", "nested"), 0o755); err != nil {
			t.Fatalf("os.MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "nested", "file.txt"), []byte("tree"), 0o644); err != nil {
			t.Fatalf("os.WriteFile() error = %v", err)
		}

		svc := newMockService(root)
		err := svc.Execute(protocol.Request{
			Version:   protocol.Version1,
			Op:        protocol.OpReflink,
			Recursive: true,
			Src:       "src",
			Dst:       "dst",
		})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		got, err := os.ReadFile(filepath.Join(root, "dst", "nested", "file.txt"))
		if err != nil {
			t.Fatalf("os.ReadFile() error = %v", err)
		}
		if string(got) != "tree" {
			t.Fatalf("dst tree file = %q, want %q", got, "tree")
		}
	})

	t.Run("fails on unsupported entry", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatalf("os.MkdirAll() error = %v", err)
		}
		if err := os.Symlink("missing.txt", filepath.Join(root, "src", "link.txt")); err != nil {
			t.Fatalf("os.Symlink() error = %v", err)
		}

		svc := newMockService(root)
		err := svc.Execute(protocol.Request{
			Version:   protocol.Version1,
			Op:        protocol.OpReflink,
			Recursive: true,
			Src:       "src",
			Dst:       "dst",
		})
		if err == nil {
			t.Fatal("Execute() unexpectedly succeeded")
		}

		testsupport.AssertCode(t, err, protocol.CodeEINVAL)
		if _, err := os.Lstat(filepath.Join(root, "dst")); err != nil {
			t.Fatalf("os.Lstat() error = %v", err)
		}
	})
}

func newMockService(root string) *Service {
	return &Service{
		Root:      root,
		Reflinker: testsupport.CopyReflinker{},
	}
}

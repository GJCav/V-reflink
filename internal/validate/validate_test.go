package validate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GJCav/V-reflink/internal/protocol"
	"github.com/GJCav/V-reflink/internal/testsupport"
)

func TestGuestToRelative(t *testing.T) {
	t.Parallel()

	got, err := GuestToRelative("/shared", "/shared/data/file.bin")
	if err != nil {
		t.Fatalf("GuestToRelative() error = %v", err)
	}

	if got != "data/file.bin" {
		t.Fatalf("GuestToRelative() = %q, want %q", got, "data/file.bin")
	}
}

func TestGuestToRelativeRejectsEscape(t *testing.T) {
	t.Parallel()

	_, err := GuestToRelative("/shared", "/other/file.bin")
	if err == nil {
		t.Fatal("GuestToRelative() unexpectedly succeeded")
	}

	testsupport.AssertCode(t, err, protocol.CodeEPERM)
}

func TestResolveGuestArgument(t *testing.T) {
	t.Parallel()

	t.Run("absolute path", func(t *testing.T) {
		got, err := ResolveGuestArgument("/shared", "/shared/project", "/shared/data/file.bin")
		if err != nil {
			t.Fatalf("ResolveGuestArgument() error = %v", err)
		}

		if got != "data/file.bin" {
			t.Fatalf("ResolveGuestArgument() = %q, want %q", got, "data/file.bin")
		}
	})

	t.Run("relative path from working directory", func(t *testing.T) {
		got, err := ResolveGuestArgument("/shared", "/shared/project", "nested/file.bin")
		if err != nil {
			t.Fatalf("ResolveGuestArgument() error = %v", err)
		}

		if got != "project/nested/file.bin" {
			t.Fatalf("ResolveGuestArgument() = %q, want %q", got, "project/nested/file.bin")
		}
	})

	t.Run("mixed absolute and relative paths", func(t *testing.T) {
		src, err := ResolveGuestArgument("/shared", "/shared/project", "/shared/input/src.bin")
		if err != nil {
			t.Fatalf("ResolveGuestArgument() error = %v", err)
		}

		dst, err := ResolveGuestArgument("/shared", "/shared/project", "output/dst.bin")
		if err != nil {
			t.Fatalf("ResolveGuestArgument() error = %v", err)
		}

		if src != "input/src.bin" {
			t.Fatalf("src = %q, want %q", src, "input/src.bin")
		}
		if dst != "project/output/dst.bin" {
			t.Fatalf("dst = %q, want %q", dst, "project/output/dst.bin")
		}
	})

	t.Run("relative escape", func(t *testing.T) {
		_, err := ResolveGuestArgument("/shared", "/shared/project", "../../../outside.bin")
		if err == nil {
			t.Fatal("ResolveGuestArgument() unexpectedly succeeded")
		}

		testsupport.AssertCode(t, err, protocol.CodeEPERM)
	})
}

func TestResolveSourceRejectsSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "real.txt")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := os.Symlink("real.txt", filepath.Join(root, "link.txt")); err != nil {
		t.Fatalf("os.Symlink() error = %v", err)
	}

	_, _, err := ResolveSource(root, "link.txt")
	if err == nil {
		t.Fatal("ResolveSource() unexpectedly succeeded")
	}

	testsupport.AssertCode(t, err, protocol.CodeEINVAL)
}

func TestRejectHardlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	original := filepath.Join(root, "one.txt")
	linked := filepath.Join(root, "two.txt")

	if err := os.WriteFile(original, []byte("data"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := os.Link(original, linked); err != nil {
		t.Fatalf("os.Link() error = %v", err)
	}

	info, err := os.Lstat(original)
	if err != nil {
		t.Fatalf("os.Lstat() error = %v", err)
	}

	err = RejectHardlink(info)
	if err == nil {
		t.Fatal("RejectHardlink() unexpectedly succeeded")
	}

	testsupport.AssertCode(t, err, protocol.CodeEINVAL)
}

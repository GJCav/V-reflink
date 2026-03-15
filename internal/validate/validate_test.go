package validate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/GJCav/V-reflink/internal/protocol"
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

	assertCode(t, err, protocol.CodeEPERM)
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

	assertCode(t, err, protocol.CodeEINVAL)
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

	assertCode(t, err, protocol.CodeEINVAL)
}

func assertCode(t *testing.T, err error, want string) {
	t.Helper()

	var coded *protocol.CodedError
	if !errors.As(err, &coded) {
		t.Fatalf("error %T is not a *protocol.CodedError", err)
	}

	if coded.Code != want {
		t.Fatalf("coded.Code = %q, want %q", coded.Code, want)
	}
}

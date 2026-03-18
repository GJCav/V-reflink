package service

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GJCav/V-reflink/internal/protocol"
	"github.com/GJCav/V-reflink/internal/testsupport"
)

type errReflinker struct {
	err error
}

func (r errReflinker) Reflink(_, _ string, _ fs.FileMode) error {
	return r.err
}

func TestValidateShareRoot(t *testing.T) {
	t.Parallel()

	t.Run("missing root", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "missing")

		err := ValidateShareRoot(root, testsupport.CopyReflinker{})
		if err == nil {
			t.Fatal("ValidateShareRoot() unexpectedly succeeded")
		}

		if !strings.Contains(err.Error(), "does not exist") {
			t.Fatalf("ValidateShareRoot() error = %v, want missing-root error", err)
		}
	})

	t.Run("root is file", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "root-file")
		if err := os.WriteFile(root, []byte("file"), 0o644); err != nil {
			t.Fatalf("os.WriteFile() error = %v", err)
		}

		err := ValidateShareRoot(root, testsupport.CopyReflinker{})
		if err == nil {
			t.Fatal("ValidateShareRoot() unexpectedly succeeded")
		}

		if !strings.Contains(err.Error(), "must be a directory") {
			t.Fatalf("ValidateShareRoot() error = %v, want directory error", err)
		}
	})

	t.Run("reflink unsupported", func(t *testing.T) {
		root := t.TempDir()

		err := ValidateShareRoot(root, errReflinker{
			err: protocol.NewError(protocol.CodeEOPNOTSUPP, "reflink is not supported for this source and destination"),
		})
		if err == nil {
			t.Fatal("ValidateShareRoot() unexpectedly succeeded")
		}

		if !strings.Contains(err.Error(), "does not support reflink") {
			t.Fatalf("ValidateShareRoot() error = %v, want unsupported error", err)
		}
	})

	t.Run("success cleans up probe files", func(t *testing.T) {
		root := t.TempDir()

		if err := ValidateShareRoot(root, testsupport.CopyReflinker{}); err != nil {
			t.Fatalf("ValidateShareRoot() error = %v", err)
		}

		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("os.ReadDir() error = %v", err)
		}

		if len(entries) != 0 {
			t.Fatalf("share root entries = %d, want 0", len(entries))
		}
	})
}

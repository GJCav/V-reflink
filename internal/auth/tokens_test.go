package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTokenMap(t *testing.T) {
	t.Parallel()

	t.Run("missing file", func(t *testing.T) {
		tokenMap, err := LoadTokenMap(filepath.Join(t.TempDir(), "missing.yaml"))
		if err != nil {
			t.Fatalf("LoadTokenMap() error = %v", err)
		}
		if tokenMap != nil {
			t.Fatalf("LoadTokenMap() = %#v, want nil", tokenMap)
		}
	})

	t.Run("valid yaml", func(t *testing.T) {
		path := writeTokenMap(t, `
version: 1
tokens:
  - name: project-a
    token: token-a
    uid: 1001
    gid: 1002
    groups: [44, 1002, 44]
`)

		tokenMap, err := LoadTokenMap(path)
		if err != nil {
			t.Fatalf("LoadTokenMap() error = %v", err)
		}

		identity, ok := tokenMap.Resolve("token-a")
		if !ok {
			t.Fatal("Resolve() unexpectedly failed")
		}
		if identity.Name != "project-a" {
			t.Fatalf("Name = %q, want %q", identity.Name, "project-a")
		}
		if identity.UID != 1001 {
			t.Fatalf("UID = %d, want %d", identity.UID, 1001)
		}
		if identity.GID != 1002 {
			t.Fatalf("GID = %d, want %d", identity.GID, 1002)
		}
		if len(identity.Groups) != 1 || identity.Groups[0] != 44 {
			t.Fatalf("Groups = %#v, want %#v", identity.Groups, []uint32{44})
		}
	})

	t.Run("duplicate token rejected", func(t *testing.T) {
		path := writeTokenMap(t, `
version: 1
tokens:
  - token: dup
    uid: 1001
    gid: 1001
  - token: dup
    uid: 1002
    gid: 1002
`)

		if _, err := LoadTokenMap(path); err == nil {
			t.Fatal("LoadTokenMap() unexpectedly succeeded")
		}
	})

	t.Run("unknown field rejected", func(t *testing.T) {
		path := writeTokenMap(t, `
version: 1
tokens:
  - token: token-a
    uid: 1001
    gid: 1002
    extra: nope
`)

		if _, err := LoadTokenMap(path); err == nil {
			t.Fatal("LoadTokenMap() unexpectedly succeeded")
		}
	})

	t.Run("missing identity rejected", func(t *testing.T) {
		path := writeTokenMap(t, `
version: 1
tokens:
  - token: only-token
    gid: 1001
`)

		if _, err := LoadTokenMap(path); err == nil {
			t.Fatal("LoadTokenMap() unexpectedly succeeded")
		}
	})
}

func writeTokenMap(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "tokens.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	return path
}

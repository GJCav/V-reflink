package auth

import "testing"

func TestNewTokenMapFromEntries(t *testing.T) {
	t.Parallel()

	t.Run("empty entries returns nil", func(t *testing.T) {
		tokenMap, err := NewTokenMapFromEntries("config.toml", nil)
		if err != nil {
			t.Fatalf("NewTokenMapFromEntries() error = %v", err)
		}
		if tokenMap != nil {
			t.Fatalf("NewTokenMapFromEntries() = %#v, want nil", tokenMap)
		}
	})

	t.Run("valid entries", func(t *testing.T) {
		uid := uint32(1001)
		gid := uint32(1002)

		tokenMap, err := NewTokenMapFromEntries("config.toml", []Entry{{
			Name:   "project-a",
			Token:  "token-a",
			UID:    &uid,
			GID:    &gid,
			Groups: []uint32{44, 1002, 44},
		}})
		if err != nil {
			t.Fatalf("NewTokenMapFromEntries() error = %v", err)
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
		uid1 := uint32(1001)
		gid1 := uint32(1001)
		uid2 := uint32(1002)
		gid2 := uint32(1002)

		_, err := NewTokenMapFromEntries("config.toml", []Entry{
			{Token: "dup", UID: &uid1, GID: &gid1},
			{Token: "dup", UID: &uid2, GID: &gid2},
		})
		if err == nil {
			t.Fatal("NewTokenMapFromEntries() unexpectedly succeeded")
		}
	})

	t.Run("missing identity rejected", func(t *testing.T) {
		gid := uint32(1001)
		_, err := NewTokenMapFromEntries("config.toml", []Entry{{
			Token: "only-token",
			GID:   &gid,
		}})
		if err == nil {
			t.Fatal("NewTokenMapFromEntries() unexpectedly succeeded")
		}
	})
}

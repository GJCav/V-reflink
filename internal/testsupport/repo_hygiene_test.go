package testsupport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoContainsNoShellScripts(t *testing.T) {
	repoRoot := RepoRoot(t)
	err := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".tmp" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".sh" {
			t.Fatalf("unexpected shell script in repo: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir() error = %v", err)
	}
}

func TestRepoContainsNoLegacyReflinkTagReferences(t *testing.T) {
	repoRoot := RepoRoot(t)
	legacyTag := "btrfs" + "test"
	err := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".tmp" {
				return filepath.SkipDir
			}
			return nil
		}

		if filepath.Ext(path) == ".png" || filepath.Ext(path) == ".jpg" || filepath.Ext(path) == ".jpeg" || filepath.Ext(path) == ".gif" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), legacyTag) {
			t.Fatalf("unexpected legacy reflink tag reference in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir() error = %v", err)
	}
}

package testsupport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoContainsNoShellScripts(t *testing.T) {
	repoRoot := RepoRoot(t)
	for _, relPath := range TrackedFiles(t) {
		path := filepath.Join(repoRoot, relPath)
		if filepath.Ext(path) == ".sh" {
			t.Fatalf("unexpected shell script in repo: %s", path)
		}
	}
}

func TestRepoContainsNoLegacyReflinkTagReferences(t *testing.T) {
	repoRoot := RepoRoot(t)
	legacyTag := "btrfs" + "test"
	for _, relPath := range TrackedFiles(t) {
		path := filepath.Join(repoRoot, relPath)

		if filepath.Ext(path) == ".png" || filepath.Ext(path) == ".jpg" || filepath.Ext(path) == ".jpeg" || filepath.Ext(path) == ".gif" {
			continue
		}
		if path == filepath.Join(repoRoot, "dev-logs", "02-impl_plan_v1.md") {
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("os.ReadFile(%s) error = %v", path, err)
		}
		if strings.Contains(string(data), legacyTag) {
			t.Fatalf("unexpected legacy reflink tag reference in %s", path)
		}
	}
}

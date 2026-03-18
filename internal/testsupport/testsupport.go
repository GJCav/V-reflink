package testsupport

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GJCav/V-reflink/internal/devsupport"
	"github.com/GJCav/V-reflink/internal/protocol"
	"github.com/KarpelesLab/reflink"
)

type CopyReflinker struct{}
type CommandResult = devsupport.CommandResult
type Writer = io.Writer

func (CopyReflinker) Reflink(srcPath, dstPath string, mode fs.FileMode) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	return os.WriteFile(dstPath, data, mode.Perm())
}

func AssertCode(t *testing.T, err error, want string) {
	t.Helper()

	if got := CodeOf(err); got != want {
		t.Fatalf("error code = %q, want %q (err=%v)", got, want, err)
	}
}

func CodeOf(err error) string {
	if coded, ok := protocol.AsCoded(err); ok {
		return coded.Code
	}

	return ""
}

func SourceRepoRoot() (string, error) {
	return devsupport.SourceRepoRoot()
}

func RepoRoot(t *testing.T) string {
	t.Helper()

	root, err := SourceRepoRoot()
	if err != nil {
		t.Fatal(err)
	}

	return root
}

func RepoTempDir(t *testing.T, suite string) string {
	t.Helper()

	base := filepath.Join(RepoRoot(t), ".tmp", suite)
	return TempDirUnder(t, base)
}

func TempDirUnder(t *testing.T, base string) string {
	t.Helper()

	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	dir, err := os.MkdirTemp(base, "case-")
	if err != nil {
		t.Fatalf("os.MkdirTemp() error = %v", err)
	}

	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	return dir
}

func RequireReflinkFS(t *testing.T, path string) {
	t.Helper()

	ok, err := HasReflinkFS(path)
	if err != nil {
		t.Fatalf("HasReflinkFS() error = %v", err)
	}
	if !ok {
		t.Fatalf("%s does not support reflink", path)
	}
}

func HasReflinkFS(path string) (bool, error) {
	probeDir, err := os.MkdirTemp(path, ".reflinkfs-probe-*")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(probeDir)

	srcPath := filepath.Join(probeDir, "src")
	dstPath := filepath.Join(probeDir, "dst")
	if err := os.WriteFile(srcPath, []byte("probe"), 0o600); err != nil {
		return false, err
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return false, err
	}
	defer src.Close()

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false, err
	}
	defer dst.Close()

	if err := reflink.Reflink(dst, src, false); err != nil {
		return false, nil
	}

	return true, nil
}

func RunCommand(ctx context.Context, dir string, env []string, name string, args ...string) (CommandResult, error) {
	result, err := devsupport.RunCommand(ctx, dir, env, name, args...)
	return CommandResult(result), err
}

func RunCommandStreaming(
	ctx context.Context,
	dir string,
	env []string,
	stdout Writer,
	stderr Writer,
	name string,
	args ...string,
) error {
	return devsupport.RunCommandStreaming(ctx, dir, env, stdout, stderr, name, args...)
}

func LookPathAny(paths ...string) (string, bool) {
	return devsupport.LookPathAny(paths...)
}

func HashFile(path string) (string, error) {
	return devsupport.HashFile(path)
}

func TrackedFiles(t *testing.T) []string {
	t.Helper()

	root := RepoRoot(t)
	result, err := RunCommand(context.Background(), root, nil, "git", "ls-files")
	if err != nil {
		t.Fatalf("git ls-files failed: %v\n%s", err, result.Stderr)
	}

	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		files = append(files, line)
	}

	return files
}

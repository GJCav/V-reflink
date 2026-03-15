package service

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/GJCav/V-reflink/internal/protocol"
)

type copyReflinker struct{}

func (copyReflinker) Reflink(srcPath, dstPath string, mode fs.FileMode) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	return os.WriteFile(dstPath, data, mode.Perm())
}

func TestExecuteSingleFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	srcPath := filepath.Join(root, "src.txt")
	if err := os.WriteFile(srcPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	svc := &Service{
		Root:      root,
		Reflinker: copyReflinker{},
	}

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
}

func TestExecuteDestinationExists(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "src.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "dst.txt"), []byte("exists"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	svc := &Service{
		Root:      root,
		Reflinker: copyReflinker{},
	}

	err := svc.Execute(protocol.Request{
		Version: protocol.Version1,
		Op:      protocol.OpReflink,
		Src:     "src.txt",
		Dst:     "dst.txt",
	})
	if err == nil {
		t.Fatal("Execute() unexpectedly succeeded")
	}

	assertCode(t, err, protocol.CodeEEXIST)
}

func TestExecuteRejectsSymlinkSource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "real.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := os.Symlink("real.txt", filepath.Join(root, "link.txt")); err != nil {
		t.Fatalf("os.Symlink() error = %v", err)
	}

	svc := &Service{
		Root:      root,
		Reflinker: copyReflinker{},
	}

	err := svc.Execute(protocol.Request{
		Version: protocol.Version1,
		Op:      protocol.OpReflink,
		Src:     "link.txt",
		Dst:     "dst.txt",
	})
	if err == nil {
		t.Fatal("Execute() unexpectedly succeeded")
	}

	assertCode(t, err, protocol.CodeEINVAL)
}

func TestExecuteRejectsHardlinkSource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "src.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := os.Link(filepath.Join(root, "src.txt"), filepath.Join(root, "src-link.txt")); err != nil {
		t.Fatalf("os.Link() error = %v", err)
	}

	svc := &Service{
		Root:      root,
		Reflinker: copyReflinker{},
	}

	err := svc.Execute(protocol.Request{
		Version: protocol.Version1,
		Op:      protocol.OpReflink,
		Src:     "src.txt",
		Dst:     "dst.txt",
	})
	if err == nil {
		t.Fatal("Execute() unexpectedly succeeded")
	}

	assertCode(t, err, protocol.CodeEINVAL)
}

func TestExecuteRecursiveSuccess(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "nested"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "nested", "file.txt"), []byte("tree"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	svc := &Service{
		Root:      root,
		Reflinker: copyReflinker{},
	}

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
}

func TestExecuteRealReflinkOnBtrfs(t *testing.T) {
	root := repoTempDir(t)

	var stat unix.Statfs_t
	if err := unix.Statfs(root, &stat); err != nil {
		t.Fatalf("unix.Statfs() error = %v", err)
	}

	const btrfsSuperMagic = 0x9123683e
	if stat.Type != btrfsSuperMagic {
		t.Skip("repository temp area is not on btrfs")
	}

	shareRoot := filepath.Join(root, "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	srcPath := filepath.Join(shareRoot, "src.bin")
	original := []byte("abcdefghijklmnop")
	if err := os.WriteFile(srcPath, original, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	svc := New(shareRoot)
	err := svc.Execute(protocol.Request{
		Version: protocol.Version1,
		Op:      protocol.OpReflink,
		Src:     "src.bin",
		Dst:     "dst.bin",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	dstPath := filepath.Join(shareRoot, "dst.bin")
	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	if string(got) != string(original) {
		t.Fatalf("dst.bin = %q, want %q", got, original)
	}

	f, err := os.OpenFile(dstPath, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("os.OpenFile() error = %v", err)
	}

	if _, err := f.WriteAt([]byte("Z"), 0); err != nil {
		_ = f.Close()
		t.Fatalf("WriteAt() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	srcAfter, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	if string(srcAfter) != string(original) {
		t.Fatalf("src.bin changed after dst write: got %q want %q", srcAfter, original)
	}
}

func TestExecuteConcurrentSameDestination(t *testing.T) {
	root := repoTempDir(t)

	var stat unix.Statfs_t
	if err := unix.Statfs(root, &stat); err != nil {
		t.Fatalf("unix.Statfs() error = %v", err)
	}

	const btrfsSuperMagic = 0x9123683e
	if stat.Type != btrfsSuperMagic {
		t.Skip("repository temp area is not on btrfs")
	}

	shareRoot := filepath.Join(root, "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(shareRoot, "A.bin"), []byte("AAAA"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(shareRoot, "B.bin"), []byte("BBBB"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	svc := New(shareRoot)
	reqs := []protocol.Request{
		{Version: protocol.Version1, Op: protocol.OpReflink, Src: "A.bin", Dst: "X.bin"},
		{Version: protocol.Version1, Op: protocol.OpReflink, Src: "B.bin", Dst: "X.bin"},
	}

	start := make(chan struct{})
	results := make(chan error, len(reqs))

	var wg sync.WaitGroup
	for _, req := range reqs {
		req := req
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- svc.Execute(req)
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	successes := 0
	eexist := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case codeOf(err) == protocol.CodeEEXIST:
			eexist++
		default:
			t.Fatalf("unexpected concurrent result: %v", err)
		}
	}

	if successes != 1 || eexist != 1 {
		t.Fatalf("got %d successes and %d EEXIST errors, want 1 and 1", successes, eexist)
	}
}

func repoTempDir(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	base := filepath.Join(repoRoot, ".tmp", "service-tests")
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

func assertCode(t *testing.T, err error, want string) {
	t.Helper()

	if got := codeOf(err); got != want {
		t.Fatalf("error code = %q, want %q (err=%v)", got, want, err)
	}
}

func codeOf(err error) string {
	var coded *protocol.CodedError
	if errors.As(err, &coded) {
		return coded.Code
	}

	return ""
}

//go:build reflinkfstest

package service

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"

	"github.com/GJCav/V-reflink/internal/protocol"
	"github.com/GJCav/V-reflink/internal/testsupport"
)

var reflinkSuiteRoot string

func TestMain(m *testing.M) {
	if err := testsupport.RequirePrivilegedSuiteAccess(context.Background(), "reflinkfs", "go run ./cmd/vreflink-dev test reflinkfs"); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	root, err := testsupport.ResolvePreparedReflinkTestRoot()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	reflinkSuiteRoot = root

	os.Exit(m.Run())
}

func TestExecuteRealReflinkOnReflinkFS(t *testing.T) {
	root := testsupport.TempDirUnder(t, reflinkSuiteRoot)

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
	root := testsupport.TempDirUnder(t, reflinkSuiteRoot)

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
		case testsupport.CodeOf(err) == protocol.CodeEEXIST, protocol.CodeFromError(err) == protocol.CodeEEXIST:
			eexist++
		default:
			t.Fatalf("unexpected concurrent result: %v", err)
		}
	}

	if successes != 1 || eexist != 1 {
		t.Fatalf("got %d successes and %d EEXIST errors, want 1 and 1", successes, eexist)
	}
}

func TestExecuteRecursivePreservesDirectoryModes(t *testing.T) {
	root := testsupport.TempDirUnder(t, reflinkSuiteRoot)

	shareRoot := filepath.Join(root, "share")
	srcRoot := filepath.Join(shareRoot, "src")
	srcNested := filepath.Join(srcRoot, "nested")
	dstRoot := filepath.Join(shareRoot, "dst")

	if err := os.MkdirAll(srcNested, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcNested, "file.txt"), []byte("tree"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	wantRootMode := fs.FileMode(0o775) | fs.ModeSetgid
	wantNestedMode := fs.FileMode(0o770) | fs.ModeSticky
	if err := os.Chmod(srcRoot, wantRootMode); err != nil {
		t.Fatalf("os.Chmod(%s) error = %v", srcRoot, err)
	}
	if err := os.Chmod(srcNested, wantNestedMode); err != nil {
		t.Fatalf("os.Chmod(%s) error = %v", srcNested, err)
	}

	oldUmask := syscall.Umask(0o022)
	defer syscall.Umask(oldUmask)

	svc := New(shareRoot)
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

	assertDirectoryMode(t, dstRoot, wantRootMode)
	assertDirectoryMode(t, filepath.Join(dstRoot, "nested"), wantNestedMode)
}

func assertDirectoryMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat(%s) error = %v", path, err)
	}

	got := info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	if got != want {
		t.Fatalf("%s mode = %#o, want %#o", path, got, want)
	}
}

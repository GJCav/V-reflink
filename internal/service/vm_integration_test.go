//go:build vmtest

package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestVMIntegration(t *testing.T) {
	if os.Getenv("VREFLINK_VM_RUN") == "" {
		t.Skip("set VREFLINK_VM_RUN=1 to execute the VM-backed integration test")
	}

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	script := filepath.Join(repoRoot, "scripts", "vm", "run-integration-test.sh")

	cmd := exec.Command(script)
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("vm integration failed: %v\n%s", err, output)
	}
}

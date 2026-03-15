//go:build vmtest

package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/GJCav/V-reflink/internal/testsupport"
)

func TestVMIntegration(t *testing.T) {
	if os.Getenv("VREFLINK_VM_RUN") == "" {
		t.Skip("set VREFLINK_VM_RUN=1 to execute the VM-backed integration test")
	}

	repoRoot := testsupport.RepoRoot(t)
	script := filepath.Join(repoRoot, "scripts", "test", "vm", "run.sh")

	cmd := exec.Command(script)
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("vm integration failed: %v\n%s", err, output)
	}
}

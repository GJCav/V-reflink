package testsupport

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/GJCav/V-reflink/internal/devsupport"
)

func ResolveVirtiofsdPath() (string, bool) {
	return devsupport.LookPathAny("virtiofsd", "/usr/lib/qemu/virtiofsd")
}

func HasSupplementaryGroup() bool {
	primary := os.Getgid()
	groups, err := os.Getgroups()
	if err != nil {
		return false
	}

	for _, group := range groups {
		if group != primary {
			return true
		}
	}

	return false
}

func FirstSupplementaryGroup() (uint32, bool) {
	primary := os.Getgid()
	groups, err := os.Getgroups()
	if err != nil {
		return 0, false
	}

	for _, group := range groups {
		if group >= 0 && group != primary {
			return uint32(group), true
		}
	}

	return 0, false
}

func NonInteractiveSudoAvailable(ctx context.Context) bool {
	if os.Geteuid() == 0 {
		return true
	}

	result, err := devsupport.RunCommand(ctx, "", nil, "sudo", "-n", "true")
	if err != nil {
		_ = result
		return false
	}

	return true
}

func CheckVMPrereqs(ctx context.Context) []string {
	var issues []string

	for _, bin := range []string{"go", "ssh", "ssh-keygen", "qemu-system-x86_64", "qemu-img", "cloud-localds"} {
		if _, ok := devsupport.LookPathAny(bin); !ok {
			issues = append(issues, fmt.Sprintf("missing %s", bin))
		}
	}

	if os.Getenv("VREFLINK_VM_SHARE_ROOT") == "" {
		if _, ok := devsupport.LookPathAny("mkfs.btrfs"); !ok {
			issues = append(issues, "missing mkfs.btrfs (install btrfs-progs or set VREFLINK_VM_SHARE_ROOT to an existing reflink-capable export)")
		}
	}

	if _, ok := ResolveVirtiofsdPath(); !ok {
		issues = append(issues, "missing virtiofsd (expected in PATH or /usr/lib/qemu/virtiofsd)")
	}

	if _, err := os.Stat("/dev/kvm"); err != nil {
		issues = append(issues, "missing /dev/kvm")
	}
	if _, err := os.Stat("/dev/vhost-vsock"); err != nil {
		issues = append(issues, "missing /dev/vhost-vsock (try: sudo modprobe vhost_vsock)")
	}

	if !HasSupplementaryGroup() {
		issues = append(issues, "missing supplementary host groups (the vm suite uses one to verify token-mapped group access)")
	}

	if err := RequirePrivilegedSuiteAccess(ctx, "vm", "go run ./cmd/vreflink-dev test vm"); err != nil {
		issues = append(issues, err.Error())
	}

	return issues
}

func SudoPrefix(ctx context.Context) ([]string, error) {
	if os.Geteuid() == 0 {
		return nil, nil
	}
	if !NonInteractiveSudoAvailable(ctx) {
		return nil, fmt.Errorf("non-interactive sudo is required")
	}
	return []string{"sudo", "-n"}, nil
}

func FormatCommand(args ...string) string {
	var quoted []string
	for _, arg := range args {
		quoted = append(quoted, strconv.Quote(arg))
	}
	return strings.Join(quoted, " ")
}

func RunCommandOrStderr(ctx context.Context, dir string, env []string, name string, args ...string) error {
	result, err := devsupport.RunCommand(ctx, dir, env, name, args...)
	if err != nil {
		stderr := strings.TrimSpace(result.Stderr)
		if stderr == "" {
			return err
		}
		return fmt.Errorf("%w\n%s", err, stderr)
	}
	return nil
}

func StartCommand(ctx context.Context, dir string, env []string, stdout, stderr *os.File, name string, args ...string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if env != nil {
		cmd.Env = env
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd, cmd.Start()
}

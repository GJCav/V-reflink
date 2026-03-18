package testsupport

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	ReflinkTestRootEnv = "VREFLINK_TEST_REFLINK_ROOT"

	defaultReflinkFixtureSize = 256 << 20
	defaultVMShareFixtureSize = 512 << 20
)

func RequirePrivilegedSuiteAccess(ctx context.Context, suite, command string) error {
	if os.Geteuid() == 0 || NonInteractiveSudoAvailable(ctx) {
		return nil
	}

	return fmt.Errorf("%s tests require root or non-interactive sudo; use '%s'", suite, command)
}

func CheckReflinkFSPrereqs(ctx context.Context) []string {
	var issues []string

	if _, ok := LookPathAny("mkfs.btrfs"); !ok {
		issues = append(issues, "missing mkfs.btrfs (install btrfs-progs to let the runner provision a reflink-capable scratch root)")
	}

	if err := RequirePrivilegedSuiteAccess(ctx, "reflinkfs", "go run ./cmd/vreflink-dev test reflinkfs"); err != nil {
		issues = append(issues, err.Error())
	}

	return issues
}

func ResolvePreparedReflinkTestRoot() (string, error) {
	return resolvePreparedReflinkRoot(
		ReflinkTestRootEnv,
		"go run ./cmd/vreflink-dev test reflinkfs",
		"reflinkfs",
	)
}

func ResolvePreparedVMShareRoot() (string, error) {
	return resolvePreparedReflinkRoot(
		"VREFLINK_VM_SHARE_ROOT",
		"go run ./cmd/vreflink-dev test vm",
		"vm",
	)
}

func ValidateReflinkRoot(root string) error {
	cleaned := filepath.Clean(root)

	info, err := os.Stat(cleaned)
	if err != nil {
		return fmt.Errorf("%s is not usable as a reflink-capable directory: %w", cleaned, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", cleaned)
	}

	ok, err := HasReflinkFS(cleaned)
	if err != nil {
		return fmt.Errorf("%s is not usable as a reflink-capable directory: %w", cleaned, err)
	}
	if !ok {
		return fmt.Errorf("%s does not support reflink", cleaned)
	}

	return nil
}

func PrepareReflinkTestRoot(ctx context.Context, repoRoot string) (string, func(context.Context) error, error) {
	return prepareLoopbackReflinkRoot(ctx, repoRoot, "reflinkfs-fixtures", defaultReflinkFixtureSize, "reflinkfs", "go run ./cmd/vreflink-dev test reflinkfs")
}

func PrepareVMShareRoot(ctx context.Context, repoRoot string) (string, func(context.Context) error, error) {
	return prepareLoopbackReflinkRoot(ctx, repoRoot, "vm-share-fixtures", defaultVMShareFixtureSize, "vm", "go run ./cmd/vreflink-dev test vm")
}

func resolvePreparedReflinkRoot(envName, command, suite string) (string, error) {
	root := strings.TrimSpace(os.Getenv(envName))
	if root == "" {
		return "", fmt.Errorf("%s tests require %s to point at a prepared reflink-capable directory; use '%s'", suite, envName, command)
	}
	if err := ValidateReflinkRoot(root); err != nil {
		return "", fmt.Errorf("%s=%q is invalid: %w", envName, root, err)
	}

	return filepath.Clean(root), nil
}

func prepareLoopbackReflinkRoot(ctx context.Context, repoRoot, tempDirName string, sizeBytes int64, suite, command string) (string, func(context.Context) error, error) {
	if err := RequirePrivilegedSuiteAccess(ctx, suite, command); err != nil {
		return "", nil, err
	}

	if _, ok := LookPathAny("mkfs.btrfs"); !ok {
		return "", nil, fmt.Errorf("mkfs.btrfs is required to provision the %s scratch root", suite)
	}

	sudoPrefix, err := SudoPrefix(ctx)
	if err != nil {
		return "", nil, err
	}

	base := filepath.Join(repoRoot, ".tmp", tempDirName)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", nil, err
	}

	runRoot, err := os.MkdirTemp(base, "run.")
	if err != nil {
		return "", nil, err
	}

	shareRoot := filepath.Join(runRoot, "root")
	shareImage := filepath.Join(runRoot, "scratch.btrfs.img")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		_ = os.RemoveAll(runRoot)
		return "", nil, err
	}

	file, err := os.Create(shareImage)
	if err != nil {
		_ = os.RemoveAll(runRoot)
		return "", nil, err
	}
	if err := file.Truncate(sizeBytes); err != nil {
		_ = file.Close()
		_ = os.RemoveAll(runRoot)
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		_ = os.RemoveAll(runRoot)
		return "", nil, err
	}

	if err := RunCommandOrStderr(ctx, repoRoot, nil, "mkfs.btrfs", "-q", shareImage); err != nil {
		_ = os.RemoveAll(runRoot)
		return "", nil, err
	}
	if err := runPrefixedCommandOrStderr(ctx, repoRoot, nil, sudoPrefix, "mount", "-o", "loop", shareImage, shareRoot); err != nil {
		_ = os.RemoveAll(runRoot)
		return "", nil, err
	}

	mounted := true
	cleanup := func(cleanupCtx context.Context) error {
		var errs []error
		if mounted {
			if err := runPrefixedCommandOrStderr(cleanupCtx, repoRoot, nil, sudoPrefix, "umount", shareRoot); err != nil {
				errs = append(errs, err)
			} else {
				mounted = false
			}
		}
		if err := os.RemoveAll(runRoot); err != nil {
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	}

	if err := runPrefixedCommandOrStderr(ctx, repoRoot, nil, sudoPrefix, "chown", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), shareRoot); err != nil {
		_ = cleanup(context.Background())
		return "", nil, err
	}
	if err := ValidateReflinkRoot(shareRoot); err != nil {
		_ = cleanup(context.Background())
		return "", nil, err
	}

	return shareRoot, cleanup, nil
}

func runPrefixedCommandOrStderr(ctx context.Context, dir string, env, prefix []string, name string, args ...string) error {
	command := append(append([]string(nil), prefix...), name)
	command = append(command, args...)
	if len(command) == 0 {
		return nil
	}

	return RunCommandOrStderr(ctx, dir, env, command[0], command[1:]...)
}

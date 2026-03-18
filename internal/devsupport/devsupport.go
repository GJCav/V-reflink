package devsupport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type CommandResult struct {
	Stdout string
	Stderr string
}

func SourceRepoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime.Caller() failed")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..")), nil
}

func RunCommand(ctx context.Context, dir string, env []string, name string, args ...string) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if env != nil {
		cmd.Env = env
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return CommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}, err
}

func RunCommandStreaming(
	ctx context.Context,
	dir string,
	env []string,
	stdout io.Writer,
	stderr io.Writer,
	name string,
	args ...string,
) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if env != nil {
		cmd.Env = env
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func LookPathAny(paths ...string) (string, bool) {
	for _, path := range paths {
		switch {
		case strings.ContainsRune(path, os.PathSeparator):
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path, true
			}
		default:
			if resolved, err := exec.LookPath(path); err == nil {
				return resolved, true
			}
		}
	}

	return "", false
}

func HashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

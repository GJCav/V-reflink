package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	GuestBinaryName = "vreflink"
	HostBinaryName  = "vreflinkd"
)

type HostResult struct {
	BinaryPath   string
	SystemdPath  string
	DefaultsPath string
}

func InstallBinary(executablePath, binDir, binaryName string) (string, error) {
	if executablePath == "" {
		return "", fmt.Errorf("executable path is required")
	}
	if binDir == "" {
		return "", fmt.Errorf("binary directory is required")
	}

	dstPath := filepath.Join(binDir, binaryName)
	if err := copyFile(executablePath, dstPath, 0o755); err != nil {
		return "", err
	}

	return dstPath, nil
}

func InstallHost(executablePath, binDir, systemdDir, defaultsPath string, systemdUnit, daemonDefaults []byte) (HostResult, error) {
	if systemdDir == "" {
		return HostResult{}, fmt.Errorf("systemd directory is required")
	}
	if defaultsPath == "" {
		return HostResult{}, fmt.Errorf("defaults path is required")
	}

	binaryPath, err := InstallBinary(executablePath, binDir, HostBinaryName)
	if err != nil {
		return HostResult{}, err
	}

	systemdPath := filepath.Join(systemdDir, HostBinaryName+".service")
	if err := WriteTemplate(systemdPath, systemdUnit, 0o644, true); err != nil {
		return HostResult{}, err
	}
	if err := WriteTemplate(defaultsPath, daemonDefaults, 0o644, true); err != nil {
		return HostResult{}, err
	}

	return HostResult{
		BinaryPath:   binaryPath,
		SystemdPath:  systemdPath,
		DefaultsPath: defaultsPath,
	}, nil
}

func WriteTemplate(path string, content []byte, mode os.FileMode, overwrite bool) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}

	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return os.ErrExist
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}

	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := tempFile.Write(content); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Chmod(mode); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}

	cleanup = false
	return nil
}

func copyFile(srcPath, dstPath string, mode os.FileMode) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	if err := dst.Chmod(mode); err != nil {
		_ = dst.Close()
		return err
	}

	return dst.Close()
}

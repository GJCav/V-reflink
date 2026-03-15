package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMountRoot = "/shared"
	defaultShareRoot = "/srv/labshare"
	defaultHostCID   = 2
	defaultPort      = 19090
	defaultTimeout   = 5 * time.Second

	cliConfigDirName  = "vreflink"
	cliConfigFileName = "env"
)

type CLI struct {
	GuestMountRoot string
	HostCID        uint32
	VsockPort      uint32
	Timeout        time.Duration
}

type Daemon struct {
	ShareRoot    string
	VsockPort    uint32
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func LoadCLI() (CLI, error) {
	return loadCLI(os.UserConfigDir, os.LookupEnv)
}

func DefaultCLI() CLI {
	return defaultCLIConfig()
}

func CLIConfigPath() (string, error) {
	return cliConfigPath(os.UserConfigDir)
}

func loadCLI(userConfigDir func() (string, error), lookupEnv func(string) (string, bool)) (CLI, error) {
	cfg := defaultCLIConfig()

	configPath, err := cliConfigPath(userConfigDir)
	if err != nil {
		return CLI{}, err
	}

	if err := loadCLIFile(configPath, &cfg); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return CLI{}, err
		}
	}

	cfg.GuestMountRoot = stringLookupEnv(lookupEnv, "VREFLINK_GUEST_MOUNT_ROOT", cfg.GuestMountRoot)
	cfg.HostCID = uint32LookupEnv(lookupEnv, "VREFLINK_HOST_CID", cfg.HostCID)
	cfg.VsockPort = uint32LookupEnv(lookupEnv, "VREFLINK_VSOCK_PORT", cfg.VsockPort)
	cfg.Timeout = durationLookupEnv(lookupEnv, "VREFLINK_CLIENT_TIMEOUT", cfg.Timeout)

	return cfg, nil
}

func defaultCLIConfig() CLI {
	return CLI{
		GuestMountRoot: defaultMountRoot,
		HostCID:        defaultHostCID,
		VsockPort:      defaultPort,
		Timeout:        defaultTimeout,
	}
}

func LoadDaemon() Daemon {
	return Daemon{
		ShareRoot:    stringEnv("VREFLINK_SHARE_ROOT", defaultShareRoot),
		VsockPort:    uint32Env("VREFLINK_VSOCK_PORT", defaultPort),
		ReadTimeout:  durationEnv("VREFLINK_READ_TIMEOUT", defaultTimeout),
		WriteTimeout: durationEnv("VREFLINK_WRITE_TIMEOUT", defaultTimeout),
	}
}

func cliConfigPath(userConfigDir func() (string, error)) (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}

	return filepath.Join(dir, cliConfigDirName, cliConfigFileName), nil
}

func loadCLIFile(path string, cfg *CLI) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		key, value, ok, err := parseEnvLine(scanner.Text())
		if err != nil {
			return fmt.Errorf("parse %s:%d: %w", path, lineNo, err)
		}

		if !ok {
			continue
		}

		if err := applyCLIFileEntry(cfg, key, value); err != nil {
			return fmt.Errorf("parse %s:%d: %w", path, lineNo, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	return nil
}

func parseEnvLine(line string) (string, string, bool, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false, nil
	}

	if strings.HasPrefix(line, "export ") || strings.HasPrefix(line, "export\t") {
		line = strings.TrimSpace(line[len("export"):])
	}

	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", false, errors.New("expected KEY=value")
	}

	key := strings.TrimSpace(parts[0])
	if key == "" {
		return "", "", false, errors.New("expected KEY=value")
	}
	if strings.ContainsAny(key, " \t") {
		return "", "", false, fmt.Errorf("invalid key %q", key)
	}

	return key, strings.TrimSpace(parts[1]), true, nil
}

func applyCLIFileEntry(cfg *CLI, key, value string) error {
	switch key {
	case "VREFLINK_GUEST_MOUNT_ROOT":
		cfg.GuestMountRoot = value
	case "VREFLINK_HOST_CID":
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return fmt.Errorf("%s must be an unsigned integer: %w", key, err)
		}
		cfg.HostCID = uint32(parsed)
	case "VREFLINK_VSOCK_PORT":
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return fmt.Errorf("%s must be an unsigned integer: %w", key, err)
		}
		cfg.VsockPort = uint32(parsed)
	case "VREFLINK_CLIENT_TIMEOUT":
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("%s must be a duration: %w", key, err)
		}
		cfg.Timeout = parsed
	}

	return nil
}

func stringEnv(key, fallback string) string {
	return stringLookupEnv(os.LookupEnv, key, fallback)
}

func uint32Env(key string, fallback uint32) uint32 {
	return uint32LookupEnv(os.LookupEnv, key, fallback)
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	return durationLookupEnv(os.LookupEnv, key, fallback)
}

func stringLookupEnv(lookupEnv func(string) (string, bool), key, fallback string) string {
	value, ok := lookupEnv(key)
	if !ok || value == "" {
		return fallback
	}

	return value
}

func uint32LookupEnv(lookupEnv func(string) (string, bool), key string, fallback uint32) uint32 {
	value, ok := lookupEnv(key)
	if !ok || value == "" {
		return fallback
	}

	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return fallback
	}

	return uint32(parsed)
}

func durationLookupEnv(lookupEnv func(string) (string, bool), key string, fallback time.Duration) time.Duration {
	value, ok := lookupEnv(key)
	if !ok || value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return parsed
}

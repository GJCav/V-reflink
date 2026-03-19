package config

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/GJCav/V-reflink/internal/auth"
)

const (
	defaultMountRoot        = "/shared"
	defaultShareRoot        = "/srv/labshare"
	defaultDaemonConfigPath = "/etc/vreflinkd/config.toml"
	defaultHostCID          = 2
	defaultPort             = 19090
	defaultTimeout          = 5 * time.Second
	defaultLogLevel         = "info"

	configVersion1 = 1

	cliConfigDirName        = "vreflink"
	cliConfigFileName       = "config.toml"
	cliLegacyConfigFileName = "env"
)

type CLI struct {
	GuestMountRoot string
	HostCID        uint32
	VsockPort      uint32
	Timeout        time.Duration
	AuthToken      string
}

type Daemon struct {
	ShareRoot       string
	VsockPort       uint32
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	LogLevel        string
	AllowV1Fallback bool
	Tokens          []auth.Entry
}

type cliFile struct {
	Version   int     `toml:"version"`
	MountRoot string  `toml:"mount_root"`
	HostCID   *uint32 `toml:"host_cid"`
	Port      *uint32 `toml:"port"`
	Timeout   string  `toml:"timeout"`
	Token     string  `toml:"token"`
}

type daemonFile struct {
	Version         int          `toml:"version"`
	ShareRoot       string       `toml:"share_root"`
	Port            *uint32      `toml:"port"`
	ReadTimeout     string       `toml:"read_timeout"`
	WriteTimeout    string       `toml:"write_timeout"`
	LogLevel        string       `toml:"log_level"`
	AllowV1Fallback bool         `toml:"allow_v1_fallback"`
	Tokens          []auth.Entry `toml:"tokens"`
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

func LoadDaemon() (Daemon, error) {
	return loadDaemon(defaultDaemonConfigPath)
}

func LoadDaemonPath(path string) (Daemon, error) {
	return loadDaemon(path)
}

func DefaultDaemonConfigPath() string {
	return defaultDaemonConfigPath
}

func DefaultDaemon() Daemon {
	return defaultDaemonConfig()
}

func loadCLI(userConfigDir func() (string, error), lookupEnv func(string) (string, bool)) (CLI, error) {
	cfg := defaultCLIConfig()

	configPath, err := cliConfigPath(userConfigDir)
	if err != nil {
		return CLI{}, err
	}

	if err := loadCLITOMLFile(configPath, &cfg); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return CLI{}, err
		}

		legacyPath, legacyErr := cliLegacyConfigPath(userConfigDir)
		if legacyErr != nil {
			return CLI{}, legacyErr
		}
		if err := loadCLILegacyEnvFile(legacyPath, &cfg); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return CLI{}, err
		}
	}

	cfg.GuestMountRoot = stringLookupEnv(lookupEnv, "VREFLINK_GUEST_MOUNT_ROOT", cfg.GuestMountRoot)
	cfg.HostCID = uint32LookupEnv(lookupEnv, "VREFLINK_HOST_CID", cfg.HostCID)
	cfg.VsockPort = uint32LookupEnv(lookupEnv, "VREFLINK_VSOCK_PORT", cfg.VsockPort)
	cfg.Timeout = durationLookupEnv(lookupEnv, "VREFLINK_CLIENT_TIMEOUT", cfg.Timeout)
	cfg.AuthToken = stringLookupEnv(lookupEnv, "VREFLINK_AUTH_TOKEN", cfg.AuthToken)

	return cfg, nil
}

func loadDaemon(path string) (Daemon, error) {
	if strings.TrimSpace(path) == "" {
		path = defaultDaemonConfigPath
	}

	cfg := defaultDaemonConfig()
	if err := loadDaemonFile(path, &cfg); err != nil {
		return Daemon{}, err
	}

	return cfg, nil
}

func defaultCLIConfig() CLI {
	return CLI{
		GuestMountRoot: defaultMountRoot,
		HostCID:        defaultHostCID,
		VsockPort:      defaultPort,
		Timeout:        defaultTimeout,
		AuthToken:      "",
	}
}

func defaultDaemonConfig() Daemon {
	return Daemon{
		ShareRoot:       defaultShareRoot,
		VsockPort:       defaultPort,
		ReadTimeout:     defaultTimeout,
		WriteTimeout:    defaultTimeout,
		LogLevel:        defaultLogLevel,
		AllowV1Fallback: false,
	}
}

func cliConfigPath(userConfigDir func() (string, error)) (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}

	return filepath.Join(dir, cliConfigDirName, cliConfigFileName), nil
}

func cliLegacyConfigPath(userConfigDir func() (string, error)) (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}

	return filepath.Join(dir, cliConfigDirName, cliLegacyConfigFileName), nil
}

func loadCLITOMLFile(path string, cfg *CLI) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var file cliFile
	if err := decodeTOML(path, data, &file); err != nil {
		return err
	}
	if file.Version != configVersion1 {
		return fmt.Errorf("parse %s: unsupported version: %d", path, file.Version)
	}

	if strings.TrimSpace(file.MountRoot) != "" {
		cfg.GuestMountRoot = strings.TrimSpace(file.MountRoot)
	}
	if file.HostCID != nil {
		cfg.HostCID = *file.HostCID
	}
	if file.Port != nil {
		cfg.VsockPort = *file.Port
	}
	if strings.TrimSpace(file.Timeout) != "" {
		parsed, err := time.ParseDuration(strings.TrimSpace(file.Timeout))
		if err != nil {
			return fmt.Errorf("parse %s: timeout must be a duration: %w", path, err)
		}
		cfg.Timeout = parsed
	}
	cfg.AuthToken = file.Token

	return nil
}

func loadCLILegacyEnvFile(path string, cfg *CLI) error {
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

		if err := applyCLILegacyEnvEntry(cfg, key, value); err != nil {
			return fmt.Errorf("parse %s:%d: %w", path, lineNo, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	return nil
}

func loadDaemonFile(path string, cfg *Daemon) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read daemon config %s: %w", path, err)
	}

	var file daemonFile
	if err := decodeTOML(path, data, &file); err != nil {
		return err
	}
	if file.Version != configVersion1 {
		return fmt.Errorf("parse %s: unsupported version: %d", path, file.Version)
	}

	if strings.TrimSpace(file.ShareRoot) != "" {
		cfg.ShareRoot = strings.TrimSpace(file.ShareRoot)
	}
	if file.Port != nil {
		cfg.VsockPort = *file.Port
	}
	if strings.TrimSpace(file.ReadTimeout) != "" {
		parsed, err := time.ParseDuration(strings.TrimSpace(file.ReadTimeout))
		if err != nil {
			return fmt.Errorf("parse %s: read_timeout must be a duration: %w", path, err)
		}
		cfg.ReadTimeout = parsed
	}
	if strings.TrimSpace(file.WriteTimeout) != "" {
		parsed, err := time.ParseDuration(strings.TrimSpace(file.WriteTimeout))
		if err != nil {
			return fmt.Errorf("parse %s: write_timeout must be a duration: %w", path, err)
		}
		cfg.WriteTimeout = parsed
	}
	if strings.TrimSpace(file.LogLevel) != "" {
		cfg.LogLevel = strings.TrimSpace(file.LogLevel)
	}
	cfg.AllowV1Fallback = file.AllowV1Fallback
	cfg.Tokens = append([]auth.Entry(nil), file.Tokens...)

	return nil
}

func decodeTOML(path string, data []byte, target any) error {
	decoder := toml.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
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

func applyCLILegacyEnvEntry(cfg *CLI, key, value string) error {
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
	case "VREFLINK_AUTH_TOKEN":
		cfg.AuthToken = value
	}

	return nil
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

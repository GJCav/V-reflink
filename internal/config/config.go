package config

import (
	"os"
	"strconv"
	"time"
)

const (
	defaultMountRoot = "/shared"
	defaultShareRoot = "/srv/labshare"
	defaultHostCID   = 2
	defaultPort      = 19090
	defaultTimeout   = 5 * time.Second
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

func LoadCLI() CLI {
	return CLI{
		GuestMountRoot: stringEnv("VREFLINK_GUEST_MOUNT_ROOT", defaultMountRoot),
		HostCID:        uint32Env("VREFLINK_HOST_CID", defaultHostCID),
		VsockPort:      uint32Env("VREFLINK_VSOCK_PORT", defaultPort),
		Timeout:        durationEnv("VREFLINK_CLIENT_TIMEOUT", defaultTimeout),
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

func stringEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func uint32Env(key string, fallback uint32) uint32 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return fallback
	}

	return uint32(parsed)
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return parsed
}

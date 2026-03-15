package main

import (
	"testing"
	"time"

	"github.com/GJCav/V-reflink/internal/config"
)

func TestNewRootCmdWithConfigFlagsOverrideLoadedValues(t *testing.T) {
	t.Parallel()

	cmd := newRootCmdWithConfig(config.CLI{
		GuestMountRoot: "/shared",
		HostCID:        2,
		VsockPort:      19090,
		Timeout:        5 * time.Second,
	})

	if err := cmd.ParseFlags([]string{
		"--mount-root", "/override",
		"--cid", "7",
		"--port", "20000",
		"--timeout", "7s",
	}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	if got, _ := cmd.Flags().GetString("mount-root"); got != "/override" {
		t.Fatalf("mount-root = %q, want %q", got, "/override")
	}
	if got, _ := cmd.Flags().GetUint32("cid"); got != 7 {
		t.Fatalf("cid = %d, want %d", got, 7)
	}
	if got, _ := cmd.Flags().GetUint32("port"); got != 20000 {
		t.Fatalf("port = %d, want %d", got, 20000)
	}
	if got, _ := cmd.Flags().GetDuration("timeout"); got != 7*time.Second {
		t.Fatalf("timeout = %s, want %s", got, 7*time.Second)
	}
}

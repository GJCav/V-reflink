//go:build vmtest

package vm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/GJCav/V-reflink/internal/devsupport"
	"github.com/GJCav/V-reflink/internal/testsupport"
)

const (
	defaultVMBaseImageURL = "https://cloud-images.ubuntu.com/minimal/releases/jammy/release/ubuntu-22.04-minimal-cloudimg-amd64.img"
	defaultVMUser         = "vreflink"
	defaultVMDiskFormat   = "qcow2"
	defaultVMFirmware     = "uefi"
	defaultVMCID          = 4
	defaultVMSSHPort      = 2222
	defaultVMHostPort     = 19090
)

type vmConfig struct {
	RepoRoot       string
	AssetRoot      string
	BaseImageURL   string
	Disk           string
	DiskFormat     string
	Firmware       string
	CID            uint32
	CIDExplicit    bool
	SSHPort        uint32
	HostPort       uint32
	InvalidPort    uint32
	FallbackPort   uint32
	MissingMapPort uint32
	RequestedShare string
	SSHUser        string
	SSHKey         string
}

type vmProcess struct {
	virtiofsdCmd        *exec.Cmd
	virtiofsdKillPrefix []string
	qemuCmd             *exec.Cmd
	qemuKillPrefix      []string
	runtimeDir          string
}

func TestMain(m *testing.M) {
	if err := testsupport.RequirePrivilegedSuiteAccess(context.Background(), "vm", "go run ./cmd/vreflink-dev test vm"); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if _, err := testsupport.ResolvePreparedVMShareRoot(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if issues := testsupport.CheckVMPrereqs(context.Background()); len(issues) != 0 {
		_, _ = fmt.Fprintln(os.Stderr, "vm prerequisites are not satisfied:")
		for _, issue := range issues {
			_, _ = fmt.Fprintln(os.Stderr, issue)
		}
		os.Exit(2)
	}

	os.Exit(m.Run())
}

func TestVMIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	repoRoot, err := devsupport.SourceRepoRoot()
	if err != nil {
		t.Fatalf("SourceRepoRoot() error = %v", err)
	}

	cfg, err := resolveVMConfig(ctx, repoRoot)
	if err != nil {
		t.Fatalf("resolveVMConfig() error = %v", err)
	}

	buildRoot := filepath.Join(repoRoot, ".tmp", "vm-integration", "build")
	runtimeRoot := filepath.Join(repoRoot, ".tmp", "vm-integration", "runtime")
	for _, dir := range []string{buildRoot, runtimeRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("os.MkdirAll() error = %v", err)
		}
	}

	runRoot, err := os.MkdirTemp(runtimeRoot, "run.")
	if err != nil {
		t.Fatalf("os.MkdirTemp() error = %v", err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			_ = os.RemoveAll(runRoot)
		}
	})

	hostUID := os.Getuid()
	hostGID := os.Getgid()
	hostGroups, err := supplementaryGroups()
	if err != nil {
		t.Fatalf("supplementaryGroups() error = %v", err)
	}
	if len(hostGroups) == 0 {
		t.Fatalf("vm suite requires the host user to have at least one supplementary group for group-based access coverage")
	}

	sudoPrefix, err := testsupport.SudoPrefix(ctx)
	if err != nil {
		t.Fatalf("SudoPrefix() error = %v", err)
	}

	shareRoot := cfg.RequestedShare
	if shareRoot == "" {
		t.Fatal("VREFLINK_VM_SHARE_ROOT is required; use 'go run ./cmd/vreflink-dev test vm'")
	}
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	groupParent := filepath.Join(shareRoot, "group-access")
	t.Cleanup(func() {
		_ = runPrefixedCommand(context.Background(), repoRoot, nil, sudoPrefix, "rm", "-rf", groupParent)
	})

	for _, path := range []string{
		filepath.Join(shareRoot, "bin"),
		filepath.Join(shareRoot, "data"),
		groupParent,
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("os.MkdirAll() error = %v", err)
		}
	}

	overlayDisk := filepath.Join(runRoot, "guest-overlay.qcow2")
	if err := runCommand(ctx, repoRoot, nil, "qemu-img", "create", "-f", "qcow2", "-F", cfg.DiskFormat, "-b", cfg.Disk, overlayDisk); err != nil {
		t.Fatalf("qemu-img create error = %v", err)
	}

	seedISO := filepath.Join(runRoot, "seed.iso")
	metaData := filepath.Join(runRoot, "meta-data")
	userData := filepath.Join(runRoot, "user-data")
	publicKey, err := os.ReadFile(cfg.SSHKey + ".pub")
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	instanceID := fmt.Sprintf("vreflink-%d-%d", time.Now().Unix(), os.Getpid())
	if err := os.WriteFile(metaData, []byte(fmt.Sprintf("instance-id: %s\nlocal-hostname: vreflink-vm\n", instanceID)), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	userDataBody := fmt.Sprintf(`#cloud-config
users:
  - name: %s
    gecos: vreflink test user
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    groups: [adm, sudo]
    ssh_authorized_keys:
      - %s
package_update: false
package_upgrade: false
ssh_pwauth: false
disable_root: true
`, cfg.SSHUser, strings.TrimSpace(string(publicKey)))
	if err := os.WriteFile(userData, []byte(userDataBody), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := runCommand(ctx, repoRoot, nil, "cloud-localds", seedISO, userData, metaData); err != nil {
		t.Fatalf("cloud-localds error = %v", err)
	}

	authToken := "vreflink-vm-token-" + instanceID
	tokenMap := filepath.Join(runRoot, "tokens.yaml")
	groupsCSV := joinUint32CSV(hostGroups)
	tokenMapBody := fmt.Sprintf("version: 1\ntokens:\n  - name: vm-integration\n    token: %s\n    uid: %d\n    gid: %d\n    groups: [%s]\n", authToken, hostUID, hostGID, groupsCSV)
	if err := os.WriteFile(tokenMap, []byte(tokenMapBody), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	for _, path := range []string{
		filepath.Join(shareRoot, "data", "B"),
		filepath.Join(shareRoot, "data", "rel-B"),
		filepath.Join(shareRoot, "data", "fallback-B"),
		filepath.Join(shareRoot, "data", "no-token-B"),
		filepath.Join(shareRoot, "data", "bad-token-B"),
		filepath.Join(shareRoot, "data", "escape-B"),
		filepath.Join(shareRoot, "data", "tree-A"),
		filepath.Join(groupParent, "tree-B"),
	} {
		_ = os.RemoveAll(path)
		_ = runPrefixedCommand(context.Background(), repoRoot, nil, sudoPrefix, "rm", "-rf", path)
	}

	if err := runCommand(ctx, repoRoot, nil, "go", "build", "-o", filepath.Join(buildRoot, "vreflink"), "./cmd/vreflink"); err != nil {
		t.Fatalf("go build vreflink error = %v", err)
	}
	if err := runCommand(ctx, repoRoot, nil, "go", "build", "-o", filepath.Join(buildRoot, "vreflinkd"), "./cmd/vreflinkd"); err != nil {
		t.Fatalf("go build vreflinkd error = %v", err)
	}
	if err := copyFile(filepath.Join(buildRoot, "vreflink"), filepath.Join(shareRoot, "bin", "vreflink"), 0o755); err != nil {
		t.Fatalf("copy guest binary error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(shareRoot, "data", "A"), []byte("vm integration reflink payload\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(shareRoot, "data", "rel-A"), []byte("vm integration relative reflink payload\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(shareRoot, "data", "tree-A", "nested"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(shareRoot, "data", "tree-A", "nested", "file.txt"), []byte("vm integration recursive reflink payload\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := runPrefixedCommand(ctx, repoRoot, nil, sudoPrefix, "chown", fmt.Sprintf("root:%d", hostGroups[0]), groupParent); err != nil {
		t.Fatalf("chown group parent error = %v", err)
	}
	if err := runPrefixedCommand(ctx, repoRoot, nil, sudoPrefix, "chmod", "2770", groupParent); err != nil {
		t.Fatalf("chmod group parent error = %v", err)
	}

	invalidShareRoot := filepath.Join(runRoot, "missing-share")
	invalidOutput, timedOut, err := runTimedCommand(5*time.Second, repoRoot, nil, filepath.Join(buildRoot, "vreflinkd"), "--share-root", invalidShareRoot, "--token-map-path", tokenMap, "--port", strconv.FormatUint(uint64(cfg.InvalidPort), 10))
	if err == nil {
		t.Fatalf("daemon unexpectedly started with an invalid share root")
	}
	if timedOut {
		t.Fatalf("daemon did not fail fast for an invalid share root:\n%s", invalidOutput)
	}
	if !strings.Contains(invalidOutput, "does not exist") {
		t.Fatalf("invalid share root error did not mention the missing path:\n%s", invalidOutput)
	}

	missingTokenOutput, timedOut, err := runTimedCommand(5*time.Second, repoRoot, nil, filepath.Join(buildRoot, "vreflinkd"), "--share-root", shareRoot, "--token-map-path", filepath.Join(runRoot, "missing-tokens.yaml"), "--port", strconv.FormatUint(uint64(cfg.MissingMapPort), 10))
	if err == nil {
		t.Fatalf("daemon unexpectedly started without a token map in fail-closed mode")
	}
	if timedOut {
		t.Fatalf("daemon did not fail fast for a missing token map:\n%s", missingTokenOutput)
	}
	if !strings.Contains(missingTokenOutput, "VREFLINK_ALLOW_V1_FALLBACK=true") {
		t.Fatalf("missing token map error did not explain the explicit fallback override:\n%s", missingTokenOutput)
	}

	mainDaemonLog := filepath.Join(runRoot, "vreflinkd.log")
	fallbackDaemonLog := filepath.Join(runRoot, "vreflinkd-fallback.log")
	mainDaemon, err := startLoggedProcess(repoRoot, nil, mainDaemonLog, filepath.Join(buildRoot, "vreflinkd"), "--share-root", shareRoot, "--token-map-path", tokenMap, "--port", strconv.FormatUint(uint64(cfg.HostPort), 10))
	if err != nil {
		t.Fatalf("start main daemon error = %v", err)
	}
	t.Cleanup(func() {
		stopProcess(mainDaemon)
	})
	fallbackEnv := append(os.Environ(), "VREFLINK_ALLOW_V1_FALLBACK=true")
	fallbackDaemon, err := startLoggedProcess(repoRoot, fallbackEnv, fallbackDaemonLog, filepath.Join(buildRoot, "vreflinkd"), "--share-root", shareRoot, "--token-map-path", filepath.Join(runRoot, "missing-tokens.yaml"), "--port", strconv.FormatUint(uint64(cfg.FallbackPort), 10))
	if err != nil {
		t.Fatalf("start fallback daemon error = %v", err)
	}
	t.Cleanup(func() {
		stopProcess(fallbackDaemon)
	})

	vmProc, err := startVMWithRetry(ctx, repoRoot, filepath.Join(runRoot, "qemu.log"), overlayDisk, cfg.DiskFormat, seedISO, cfg.Firmware, shareRoot, cfg.CID, cfg.CIDExplicit, cfg.SSHPort)
	if err != nil {
		t.Fatalf("startVMWithRetry() error = %v", err)
	}
	t.Cleanup(func() {
		vmProc.stop()
	})

	sshBase := []string{
		"ssh",
		"-i", cfg.SSHKey,
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-p", strconv.FormatUint(uint64(cfg.SSHPort), 10),
		cfg.SSHUser + "@127.0.0.1",
	}

	if err := waitForSSH(ctx, sshBase); err != nil {
		t.Fatalf("waitForSSH() error = %v", err)
	}
	if err := waitForGuestShare(ctx, sshBase); err != nil {
		t.Fatalf("waitForGuestShare() error = %v", err)
	}

	if _, err := runGuest(ctx, sshBase, fmt.Sprintf("chmod +x /shared/bin/vreflink && /shared/bin/vreflink --token %s --mount-root /shared --cid 2 --port %d /shared/data/A /shared/data/B", authToken, cfg.HostPort)); err != nil {
		t.Fatalf("token-auth reflink error = %v", err)
	}
	assertSameFileContents(t, filepath.Join(shareRoot, "data", "A"), filepath.Join(shareRoot, "data", "B"))
	assertOwnership(t, filepath.Join(shareRoot, "data", "B"), uint32(hostUID), uint32(hostGID))

	if _, err := runGuest(ctx, sshBase, fmt.Sprintf("/shared/bin/vreflink --mount-root /shared --cid 2 --port %d /shared/data/A /shared/data/fallback-B", cfg.FallbackPort)); err != nil {
		t.Fatalf("fallback reflink error = %v", err)
	}
	assertSameFileContents(t, filepath.Join(shareRoot, "data", "A"), filepath.Join(shareRoot, "data", "fallback-B"))

	if _, err := runGuest(ctx, sshBase, fmt.Sprintf("set +e; /shared/bin/vreflink --mount-root /shared --cid 2 --port %d /shared/data/A /shared/data/no-token-B > /tmp/vreflink-no-token.log 2>&1; status=$?; set -e; test $status -ne 0; grep -q 'protocol version 2' /tmp/vreflink-no-token.log", cfg.HostPort)); err != nil {
		t.Fatalf("missing-token guest check error = %v", err)
	}
	if _, err := runGuest(ctx, sshBase, fmt.Sprintf("set +e; /shared/bin/vreflink --token wrong-token --mount-root /shared --cid 2 --port %d /shared/data/A /shared/data/bad-token-B > /tmp/vreflink-bad-token.log 2>&1; status=$?; set -e; test $status -ne 0; grep -q 'invalid authentication token' /tmp/vreflink-bad-token.log", cfg.HostPort)); err != nil {
		t.Fatalf("bad-token guest check error = %v", err)
	}
	if _, err := runGuest(ctx, sshBase, fmt.Sprintf("cd /shared/data && /shared/bin/vreflink --token %s --mount-root /shared --cid 2 --port %d rel-A rel-B", authToken, cfg.HostPort)); err != nil {
		t.Fatalf("relative-path reflink error = %v", err)
	}
	assertSameFileContents(t, filepath.Join(shareRoot, "data", "rel-A"), filepath.Join(shareRoot, "data", "rel-B"))
	if _, err := runGuest(ctx, sshBase, fmt.Sprintf("cd /shared/data && set +e; /shared/bin/vreflink --token %s --mount-root /shared --cid 2 --port %d ../../../etc/passwd escape-B > /tmp/vreflink-relative-escape.log 2>&1; status=$?; set -e; test $status -ne 0; grep -q 'guest mount root' /tmp/vreflink-relative-escape.log", authToken, cfg.HostPort)); err != nil {
		t.Fatalf("relative escape guest check error = %v", err)
	}
	if _, err := runGuest(ctx, sshBase, fmt.Sprintf("/shared/bin/vreflink -r --token %s --mount-root /shared --cid 2 --port %d /shared/data/tree-A /shared/group-access/tree-B", authToken, cfg.HostPort)); err != nil {
		t.Fatalf("recursive reflink error = %v", err)
	}
	assertSameFileContents(t, filepath.Join(shareRoot, "data", "tree-A", "nested", "file.txt"), filepath.Join(groupParent, "tree-B", "nested", "file.txt"))
	assertOwnership(t, filepath.Join(groupParent, "tree-B"), uint32(hostUID), hostGroups[0])
	assertOwnership(t, filepath.Join(groupParent, "tree-B", "nested", "file.txt"), uint32(hostUID), hostGroups[0])

	file, err := os.OpenFile(filepath.Join(shareRoot, "data", "B"), os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("os.OpenFile() error = %v", err)
	}
	if _, err := file.WriteAt([]byte("Z"), 0); err != nil {
		_ = file.Close()
		t.Fatalf("WriteAt() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	srcData, err := os.ReadFile(filepath.Join(shareRoot, "data", "A"))
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if len(srcData) > 0 && srcData[0] == 'Z' {
		t.Fatalf("source changed after destination write")
	}
}

func resolveVMConfig(ctx context.Context, repoRoot string) (vmConfig, error) {
	cfg := vmConfig{
		RepoRoot:       repoRoot,
		AssetRoot:      envOrDefault("VREFLINK_VM_ASSET_ROOT", filepath.Join(repoRoot, ".tmp", "vm-assets", "ubuntu-minimal")),
		BaseImageURL:   envOrDefault("VREFLINK_VM_BASE_IMAGE_URL", defaultVMBaseImageURL),
		Disk:           os.Getenv("VREFLINK_VM_DISK"),
		DiskFormat:     os.Getenv("VREFLINK_VM_DISK_FORMAT"),
		Firmware:       envOrDefault("VREFLINK_VM_FIRMWARE", defaultVMFirmware),
		RequestedShare: os.Getenv("VREFLINK_VM_SHARE_ROOT"),
		SSHUser:        os.Getenv("VREFLINK_VM_SSH_USER"),
		SSHKey:         os.Getenv("VREFLINK_VM_SSH_KEY"),
	}

	cid, explicit, err := resolveCIDEnv("VREFLINK_VM_CID", defaultVMCID)
	if err != nil {
		return vmConfig{}, err
	}
	cfg.CID = cid
	cfg.CIDExplicit = explicit

	sshPort, err := resolvePortEnv("VREFLINK_VM_SSH_PORT", defaultVMSSHPort)
	if err != nil {
		return vmConfig{}, err
	}
	cfg.SSHPort = sshPort

	hostPort, err := resolvePortEnv("VREFLINK_VM_HOST_PORT", defaultVMHostPort)
	if err != nil {
		return vmConfig{}, err
	}
	cfg.HostPort = hostPort
	if os.Getenv("VREFLINK_VM_HOST_PORT") != "" {
		cfg.InvalidPort = cfg.HostPort + 1
		cfg.FallbackPort = cfg.HostPort + 2
		cfg.MissingMapPort = cfg.HostPort + 3
	} else {
		auxPorts, err := pickFreeTCPPorts(3)
		if err != nil {
			return vmConfig{}, err
		}
		cfg.InvalidPort = auxPorts[0]
		cfg.FallbackPort = auxPorts[1]
		cfg.MissingMapPort = auxPorts[2]
	}

	if cfg.Disk == "" || cfg.SSHUser == "" || cfg.SSHKey == "" {
		prepared, err := prepareImage(ctx, cfg.AssetRoot, cfg.BaseImageURL)
		if err != nil {
			return vmConfig{}, err
		}
		cfg.Disk = prepared.Disk
		cfg.DiskFormat = prepared.DiskFormat
		cfg.Firmware = prepared.Firmware
		cfg.SSHUser = prepared.SSHUser
		cfg.SSHKey = prepared.SSHKey
	}
	if cfg.DiskFormat == "" {
		cfg.DiskFormat = defaultVMDiskFormat
	}

	return cfg, nil
}

type preparedImage struct {
	Disk       string
	DiskFormat string
	Firmware   string
	SSHUser    string
	SSHKey     string
}

func prepareImage(ctx context.Context, assetRoot, imageURL string) (preparedImage, error) {
	baseDisk := filepath.Join(assetRoot, "ubuntu-22.04-minimal-cloudimg-amd64.img")
	sshKey := filepath.Join(assetRoot, "id_ed25519")
	if err := os.MkdirAll(assetRoot, 0o755); err != nil {
		return preparedImage{}, err
	}

	if _, err := os.Stat(baseDisk); os.IsNotExist(err) {
		tmpPath := baseDisk + ".tmp"
		if err := downloadFile(ctx, imageURL, tmpPath); err != nil {
			return preparedImage{}, err
		}
		if err := os.Rename(tmpPath, baseDisk); err != nil {
			return preparedImage{}, err
		}
	} else if err != nil {
		return preparedImage{}, err
	}

	if _, err := os.Stat(sshKey); os.IsNotExist(err) {
		if err := runCommand(ctx, "", nil, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", sshKey); err != nil {
			return preparedImage{}, err
		}
	} else if err != nil {
		return preparedImage{}, err
	}

	return preparedImage{
		Disk:       baseDisk,
		DiskFormat: defaultVMDiskFormat,
		Firmware:   defaultVMFirmware,
		SSHUser:    defaultVMUser,
		SSHKey:     sshKey,
	}, nil
}

func downloadFile(ctx context.Context, url, path string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: unexpected status %s", url, response.Status)
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, response.Body)
	return err
}

func runCommand(ctx context.Context, dir string, env []string, name string, args ...string) error {
	result, err := devsupport.RunCommand(ctx, dir, env, name, args...)
	if err != nil {
		output := strings.TrimSpace(result.Stderr)
		if output == "" {
			output = strings.TrimSpace(result.Stdout)
		}
		if output != "" {
			return fmt.Errorf("%w\n%s", err, output)
		}
		return err
	}
	return nil
}

func runPrefixedCommand(ctx context.Context, dir string, env, prefix []string, name string, args ...string) error {
	command := append(append([]string(nil), prefix...), name)
	command = append(command, args...)
	if len(command) == 0 {
		return nil
	}
	return runCommand(ctx, dir, env, command[0], command[1:]...)
}

func runTimedCommand(timeout time.Duration, dir string, env []string, name string, args ...string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := devsupport.RunCommand(ctx, dir, env, name, args...)
	output := strings.TrimSpace(strings.Join([]string{result.Stdout, result.Stderr}, "\n"))
	if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
		return output, true, err
	}
	return output, false, err
}

func startLoggedProcess(dir string, env []string, logPath string, name string, args ...string) (*exec.Cmd, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	_ = logFile.Close()
	return cmd, nil
}

func stopProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

func terminateProcess(cmd *exec.Cmd, killPrefix []string, group bool) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	target := strconv.Itoa(cmd.Process.Pid)
	if group {
		target = "-" + target
	}

	if len(killPrefix) == 0 {
		if group {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		} else {
			_ = cmd.Process.Kill()
		}
	} else {
		args := append(append([]string(nil), killPrefix...), "kill", "-TERM", "--", target)
		_ = exec.Command(args[0], args[1:]...).Run()
	}
	_ = cmd.Wait()
}

func startVM(ctx context.Context, repoRoot, qemuLogPath, disk, diskFormat, seedISO, firmware, shareRoot string, cid, sshPort uint32) (*vmProcess, error) {
	virtiofsdBin, ok := testsupport.ResolveVirtiofsdPath()
	if !ok {
		return nil, fmt.Errorf("virtiofsd not found")
	}

	if _, err := os.Stat("/dev/vhost-vsock"); err != nil && testsupport.NonInteractiveSudoAvailable(ctx) {
		for _, module := range []string{"vhost_vsock", "vsock", "vmw_vsock_virtio_transport"} {
			_ = runCommand(ctx, repoRoot, nil, "sudo", "-n", "modprobe", module)
		}
	}

	qemuPrefix := []string(nil)
	virtiofsdPrefix := []string(nil)
	if !isWritable("/dev/kvm") || !isWritable("/dev/vhost-vsock") {
		prefix, err := testsupport.SudoPrefix(ctx)
		if err != nil {
			return nil, fmt.Errorf("KVM or vhost-vsock requires elevated access: %w", err)
		}
		qemuPrefix = prefix
		virtiofsdPrefix = prefix
	}

	runtimeDir, err := os.MkdirTemp(filepath.Dir(qemuLogPath), "qemu-runtime.")
	if err != nil {
		return nil, err
	}

	socketPath := filepath.Join(runtimeDir, "virtiofsd.sock")
	virtiofsdLogPath := filepath.Join(runtimeDir, "virtiofsd.log")
	virtiofsdLog, err := os.Create(virtiofsdLogPath)
	if err != nil {
		return nil, err
	}

	virtiofsdArgs := append(append([]string(nil), virtiofsdPrefix...), virtiofsdBin,
		"-f",
		"--socket-path="+socketPath,
		"-o", "source="+shareRoot,
		"-o", "cache=auto",
		"-o", "sandbox=chroot",
	)
	virtiofsdCmd := exec.CommandContext(ctx, virtiofsdArgs[0], virtiofsdArgs[1:]...)
	virtiofsdCmd.Stdout = virtiofsdLog
	virtiofsdCmd.Stderr = virtiofsdLog
	if err := virtiofsdCmd.Start(); err != nil {
		_ = virtiofsdLog.Close()
		return nil, err
	}
	_ = virtiofsdLog.Close()

	if err := waitForSocket(socketPath, 10*time.Second); err != nil {
		terminateProcess(virtiofsdCmd, virtiofsdPrefix, false)
		return nil, err
	}

	qemuLog, err := os.Create(qemuLogPath)
	if err != nil {
		terminateProcess(virtiofsdCmd, virtiofsdPrefix, false)
		return nil, err
	}

	qemuArgs := append(append([]string(nil), qemuPrefix...), "qemu-system-x86_64",
		"-enable-kvm",
		"-machine", "q35,accel=kvm:tcg",
		"-cpu", "host",
		"-m", "1024M",
		"-smp", "2",
		"-object", "memory-backend-memfd,id=mem,size=1024M,share=on",
		"-numa", "node,memdev=mem",
		"-drive", fmt.Sprintf("if=virtio,file=%s,format=%s", disk, diskFormat),
		"-netdev", fmt.Sprintf("user,id=net0,hostfwd=tcp::%d-:22", sshPort),
		"-device", "virtio-net-pci,netdev=net0",
		"-chardev", fmt.Sprintf("socket,id=char0,path=%s", socketPath),
		"-device", "vhost-user-fs-pci,chardev=char0,tag=shared",
		"-device", fmt.Sprintf("vhost-vsock-pci,guest-cid=%d", cid),
		"-nographic",
	)
	if seedISO != "" {
		qemuArgs = append(qemuArgs, "-drive", fmt.Sprintf("file=%s,format=raw,if=virtio", seedISO))
	}

	if firmware == "uefi" {
		ovmfCode, ovmfVars, err := resolveOVMF(runtimeDir)
		if err != nil {
			terminateProcess(virtiofsdCmd, virtiofsdPrefix, false)
			return nil, err
		}
		qemuArgs = append(qemuArgs,
			"-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", ovmfCode),
			"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", ovmfVars),
		)
	}

	qemuCmd := exec.CommandContext(ctx, qemuArgs[0], qemuArgs[1:]...)
	qemuCmd.Stdout = qemuLog
	qemuCmd.Stderr = qemuLog
	qemuCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := qemuCmd.Start(); err != nil {
		terminateProcess(virtiofsdCmd, virtiofsdPrefix, false)
		_ = qemuLog.Close()
		return nil, err
	}
	_ = qemuLog.Close()

	time.Sleep(300 * time.Millisecond)
	if err := qemuCmd.Process.Signal(syscall.Signal(0)); err != nil {
		output, _ := os.ReadFile(qemuLogPath)
		_ = qemuCmd.Wait()
		terminateProcess(virtiofsdCmd, virtiofsdPrefix, false)
		return nil, fmt.Errorf("qemu exited during startup: %s", strings.TrimSpace(string(output)))
	}

	return &vmProcess{
		virtiofsdCmd:        virtiofsdCmd,
		virtiofsdKillPrefix: virtiofsdPrefix,
		qemuCmd:             qemuCmd,
		qemuKillPrefix:      qemuPrefix,
		runtimeDir:          runtimeDir,
	}, nil
}

func startVMWithRetry(ctx context.Context, repoRoot, qemuLogPath, disk, diskFormat, seedISO, firmware, shareRoot string, cid uint32, explicitCID bool, sshPort uint32) (*vmProcess, error) {
	attemptCID := cid
	for attempt := 0; attempt < 8; attempt++ {
		vmProc, err := startVM(ctx, repoRoot, qemuLogPath, disk, diskFormat, seedISO, firmware, shareRoot, attemptCID, sshPort)
		if err == nil {
			return vmProc, nil
		}
		if explicitCID || !strings.Contains(err.Error(), "guest cid") {
			return nil, err
		}
		attemptCID++
	}

	return nil, fmt.Errorf("could not find a free guest CID starting from %d", cid)
}

func (p *vmProcess) stop() {
	if p == nil {
		return
	}
	terminateProcess(p.qemuCmd, p.qemuKillPrefix, true)
	terminateProcess(p.virtiofsdCmd, p.virtiofsdKillPrefix, false)
	if p.runtimeDir != "" {
		_ = os.RemoveAll(p.runtimeDir)
	}
}

func resolveOVMF(runtimeDir string) (string, string, error) {
	codeCandidates := []string{
		"/usr/share/OVMF/OVMF_CODE.fd",
		"/usr/share/OVMF/OVMF_CODE_4M.fd",
		"/usr/share/OVMF/OVMF_CODE_4M.ms.fd",
		"/usr/share/ovmf/OVMF.fd",
	}

	var ovmfCode string
	for _, candidate := range codeCandidates {
		if _, err := os.Stat(candidate); err == nil {
			ovmfCode = candidate
			break
		}
	}
	if ovmfCode == "" {
		return "", "", fmt.Errorf("firmware=uefi requires a usable OVMF code image")
	}

	var varsCandidates []string
	switch filepath.Base(ovmfCode) {
	case "OVMF_CODE_4M.fd":
		varsCandidates = []string{"/usr/share/OVMF/OVMF_VARS_4M.fd", "/usr/share/OVMF/OVMF_VARS.fd", "/usr/share/OVMF/OVMF_VARS_4M.ms.fd", "/usr/share/OVMF/OVMF_VARS.ms.fd"}
	case "OVMF_CODE_4M.ms.fd", "OVMF_CODE_4M.secboot.fd", "OVMF_CODE_4M.snakeoil.fd":
		varsCandidates = []string{"/usr/share/OVMF/OVMF_VARS_4M.ms.fd", "/usr/share/OVMF/OVMF_VARS_4M.fd", "/usr/share/OVMF/OVMF_VARS.ms.fd", "/usr/share/OVMF/OVMF_VARS.fd"}
	default:
		varsCandidates = []string{"/usr/share/OVMF/OVMF_VARS.fd", "/usr/share/OVMF/OVMF_VARS.ms.fd", "/usr/share/OVMF/OVMF_VARS_4M.fd", "/usr/share/OVMF/OVMF_VARS_4M.ms.fd"}
	}

	ovmfVars := filepath.Join(runtimeDir, "OVMF_VARS.fd")
	for _, candidate := range varsCandidates {
		if _, err := os.Stat(candidate); err == nil {
			if err := copyFile(candidate, ovmfVars, 0o644); err != nil {
				return "", "", err
			}
			return ovmfCode, ovmfVars, nil
		}
	}

	return "", "", fmt.Errorf("firmware=uefi requires a usable OVMF vars image")
}

func waitForSocket(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, err := os.Stat(path)
		if err == nil && info.Mode()&os.ModeSocket != 0 {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for virtiofsd socket %s", path)
}

func waitForSSH(ctx context.Context, base []string) error {
	var lastErr error
	for range 90 {
		_, err := runGuest(ctx, base, "true")
		if err == nil {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("guest SSH did not come up in time: %w", lastErr)
}

func waitForGuestShare(ctx context.Context, base []string) error {
	command := "sudo mkdir -p /shared && if ! grep -q ' /shared virtiofs ' /proc/mounts; then sudo mount -t virtiofs shared /shared; fi; grep ' /shared ' /proc/mounts || true; ls -la /shared || true; test -x /shared/bin/vreflink"

	var lastErr error
	for range 30 {
		_, err := runGuest(ctx, base, command)
		if err == nil {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}

	return fmt.Errorf("guest share did not become ready: %w", lastErr)
}

func runGuest(ctx context.Context, base []string, remote string) (string, error) {
	args := append(append([]string(nil), base[1:]...), remote)
	result, err := devsupport.RunCommand(ctx, "", nil, base[0], args...)
	if err != nil {
		output := strings.TrimSpace(strings.Join([]string{result.Stdout, result.Stderr}, "\n"))
		if output != "" {
			return output, fmt.Errorf("%w\n%s", err, output)
		}
		return output, err
	}
	return result.Stdout, nil
}

func supplementaryGroups() ([]uint32, error) {
	primary := os.Getgid()
	groups, err := os.Getgroups()
	if err != nil {
		return nil, err
	}

	var out []uint32
	for _, group := range groups {
		if group >= 0 && group != primary {
			out = append(out, uint32(group))
		}
	}
	return out, nil
}

func joinUint32CSV(values []uint32) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatUint(uint64(value), 10))
	}
	return strings.Join(parts, ",")
}

func assertSameFileContents(t *testing.T, left, right string) {
	t.Helper()

	leftData, err := os.ReadFile(left)
	if err != nil {
		t.Fatalf("os.ReadFile(%s) error = %v", left, err)
	}
	rightData, err := os.ReadFile(right)
	if err != nil {
		t.Fatalf("os.ReadFile(%s) error = %v", right, err)
	}
	if string(leftData) != string(rightData) {
		t.Fatalf("%s and %s differ", left, right)
	}
}

func assertOwnership(t *testing.T, path string, wantUID, wantGID uint32) {
	t.Helper()

	gotUID, gotGID, err := ownership(path)
	if err != nil {
		t.Fatalf("ownership(%s) error = %v", path, err)
	}
	if gotUID != wantUID || gotGID != wantGID {
		t.Fatalf("%s ownership = %d:%d, want %d:%d", path, gotUID, gotGID, wantUID, wantGID)
	}
}

func ownership(path string) (uint32, uint32, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("unexpected stat type %T", info.Sys())
	}
	return stat.Uid, stat.Gid, nil
}

func copyFile(srcPath, dstPath string, mode os.FileMode) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	return os.WriteFile(dstPath, data, mode)
}

func envOrDefault(key, fallback string) string {
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

func resolvePortEnv(key string, fallback uint32) (uint32, error) {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return 0, err
		}
		return uint32(parsed), nil
	}

	port, err := pickFreeTCPPort()
	if err != nil {
		return 0, err
	}
	if port == 0 {
		return fallback, nil
	}
	return port, nil
}

func resolveCIDEnv(key string, fallback uint32) (uint32, bool, error) {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return 0, true, err
		}
		return uint32(parsed), true, nil
	}

	return fallback, false, nil
}

func pickFreeTCPPort() (uint32, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address %T", listener.Addr())
	}

	return uint32(addr.Port), nil
}

func pickFreeTCPPorts(count int) ([]uint32, error) {
	ports := make([]uint32, 0, count)
	seen := make(map[uint32]struct{}, count)
	for len(ports) < count {
		port, err := pickFreeTCPPort()
		if err != nil {
			return nil, err
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
	}
	return ports, nil
}

func isWritable(path string) bool {
	return unixAccess(path, 0o2) == nil
}

func unixAccess(path string, mode uint32) error {
	return syscall.Access(path, mode)
}

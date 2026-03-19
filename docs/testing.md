# Testing Guide

Use the unified Go runner:

```bash
go run ./cmd/vreflink-dev test <quick|reflinkfs|vm|release|all> [--race]
```

`quick` is the default suite. Run `reflinkfs` when you need real local reflink
coverage and `vm` when you need the full guest/host virtiofs + vsock path. The
runner is the supported interface for both privileged suites and will provision
their temporary reflink-capable scratch roots for you.

## Suite Matrix

| Suite | Command | Purpose | Requires |
| --- | --- | --- | --- |
| `quick` | `go run ./cmd/vreflink-dev test quick` | Fast default Go tests | `go` |
| `reflinkfs` | `go run ./cmd/vreflink-dev test reflinkfs` | Real local reflink + COW checks | root or non-interactive `sudo`, `mkfs.btrfs` |
| `vm` | `go run ./cmd/vreflink-dev test vm` | Full virtiofs + vsock integration | root or non-interactive `sudo`, QEMU + VM prerequisites |
| `release` | `go run ./cmd/vreflink-dev test release` | Tarball + `.deb` build and install smoke tests | `dpkg`, `dpkg-deb` |
| `all` | `go run ./cmd/vreflink-dev test all` | Run `quick`, then `reflinkfs`, then `vm` | all of the above |

Race detector support:

```bash
go run ./cmd/vreflink-dev test quick --race
go run ./cmd/vreflink-dev test reflinkfs --race
```

The VM and release suites reject `--race`.

## Default Contributor Flow

Run these in order:

```bash
go run ./cmd/vreflink-dev test quick
go run ./cmd/vreflink-dev test reflinkfs
go run ./cmd/vreflink-dev test vm
```

Most changes only need `quick`. Reach for `reflinkfs` when touching reflink or
filesystem behavior. Reach for `vm` when touching transport, CLI/daemon wiring,
or anything that depends on the real guest/host boundary. Reach for `release`
when touching installation, packaging, release workflows, or systemd assets.

The contributor runner provisions and tears down the privileged suite fixtures:

- `reflinkfs` creates a temporary loopback-backed btrfs scratch root and passes
  it to the tagged tests via `VREFLINK_TEST_REFLINK_ROOT`.
- `vm` creates a temporary loopback-backed btrfs host share root when
  `VREFLINK_VM_SHARE_ROOT` is not already set.

## Direct Go Commands

`go test ./...` is the quick suite entrypoint. The tagged privileged suites are
expert-mode commands and fail fast unless you pre-arrange the required
environment yourself:

```bash
go test ./...
VREFLINK_TEST_REFLINK_ROOT=/prepared/reflink-root go test -tags reflinkfstest ./internal/service
VREFLINK_VM_SHARE_ROOT=/prepared/share-root go test -tags vmtest ./integration/vm
go test -tags releasetest ./integration/release
```

If you do not already have those prepared roots, use `go run ./cmd/vreflink-dev
test reflinkfs` or `go run ./cmd/vreflink-dev test vm` instead.

## Temporary Artifacts

Temporary test assets live under `.tmp/`:

- `.tmp/reflinkfs-fixtures/` for the runner-managed reflinkfs scratch mounts
- `.tmp/vm-assets/ubuntu-minimal/` for the cached cloud image and SSH keypair
- `.tmp/vm-integration/build/` for the temporary test binaries
- `.tmp/vm-integration/runtime/` for per-run logs, overlays, and cloud-init data
- `.tmp/vm-share-fixtures/` for the runner-managed VM host share mounts

These paths are ignored by git.

## VM Prerequisites

Check the VM prerequisites with:

```bash
go run ./cmd/vreflink-dev vm check-prereqs
```

The VM suite expects:

- `go`
- `mkfs.btrfs` unless `VREFLINK_VM_SHARE_ROOT` points to your own reflink-capable export
- `ssh`
- `ssh-keygen`
- `qemu-system-x86_64`
- `qemu-img`
- `cloud-localds`
- `virtiofsd` or `/usr/lib/qemu/virtiofsd`
- `/dev/kvm`
- `/dev/vhost-vsock`
- root or non-interactive `sudo`

The VM runner will prepare the Ubuntu Minimal image on demand, build the test
binaries, create a temporary loopback btrfs share root unless
`VREFLINK_VM_SHARE_ROOT` is already set, boot the guest, mount virtiofs, run
`vreflink`, generate a temporary daemon TOML config that maps a bearer token to
the current host user's `uid`, `gid`, and supplementary groups, and verify
post-write copy-on-write plus token-authenticated ownership behavior. It also
checks the default fail-closed startup path when token configuration is missing
and the explicit legacy fallback mode.

The VM suite also expects the current host user to have at least one
supplementary group so it can cover the group-based access case.

## VM Environment Variables

The VM suite supports these overrides:

- `VREFLINK_VM_ASSET_ROOT` for the cached image and SSH key directory
- `VREFLINK_VM_BASE_IMAGE_URL` for the Ubuntu Minimal cloud image URL
- `VREFLINK_VM_DISK` for a pre-existing guest disk image
- `VREFLINK_VM_DISK_FORMAT` for the guest disk backing format
- `VREFLINK_VM_FIRMWARE` for `bios` or `uefi`
- `VREFLINK_VM_CID` for the guest vsock CID
- `VREFLINK_VM_SSH_PORT` for the host-to-guest SSH forward port
- `VREFLINK_VM_SHARE_ROOT` for the host virtiofs export directory
- `VREFLINK_VM_HOST_PORT` for the host daemon listen port
- `VREFLINK_VM_SSH_USER` for the guest SSH username
- `VREFLINK_VM_SSH_KEY` for the guest SSH private key path

If `VREFLINK_VM_DISK`, `VREFLINK_VM_SSH_USER`, or `VREFLINK_VM_SSH_KEY` are not
set, the VM suite will populate them automatically by preparing the cached
image and SSH key under `VREFLINK_VM_ASSET_ROOT`.

## Troubleshooting

- Missing `virtiofsd`: install it or ensure `/usr/lib/qemu/virtiofsd` exists.
- Missing `/dev/kvm`: enable KVM support for the host kernel and device access.
- Missing `/dev/vhost-vsock`: load the module with `sudo modprobe vhost_vsock`.
- Missing root or non-interactive `sudo`: the `reflinkfs` and `vm` suites are
  runner-managed privileged suites.
- Missing supplementary groups: the VM suite needs at least one host
  supplementary group to verify token-mapped group authorization.

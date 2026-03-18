# Testing Guide

Use the unified test runner:

```bash
scripts/test/run.sh <quick|btrfs|vm|all> [--race]
```

`quick` is the default suite. Run `btrfs` when you need real local reflink
coverage, and run `vm` when you need the full guest/host virtiofs + vsock path.

## Suite Matrix

| Suite | Command | Purpose | Requires |
| --- | --- | --- | --- |
| `quick` | `scripts/test/run.sh quick` | Fast default Go tests | `go` |
| `btrfs` | `scripts/test/run.sh btrfs` | Real local reflink + COW checks | workspace on btrfs |
| `vm` | `scripts/test/run.sh vm` | Full virtiofs + vsock integration | QEMU + VM prerequisites |
| `release` | `scripts/test/run.sh release` | Tarball + `.deb` build and install smoke tests | `dpkg`, `dpkg-deb`, `tar` |
| `all` | `scripts/test/run.sh all` | Run `quick`, then `btrfs`, then `vm` | all of the above |

Race detector support:

```bash
scripts/test/run.sh quick --race
scripts/test/run.sh btrfs --race
```

The VM suite rejects `--race`.

## Default Contributor Flow

Run these in order:

```bash
scripts/test/run.sh quick
scripts/test/run.sh btrfs
scripts/test/run.sh vm
```

Most changes only need `quick`. Reach for `btrfs` when touching reflink or
filesystem behavior. Reach for `vm` when touching transport, CLI/daemon wiring,
or anything that depends on the real guest/host boundary. Reach for `release`
when touching installation, packaging, release scripts, or systemd assets.

## Direct Go Commands

The runner is the canonical interface, but these direct Go invocations back it:

```bash
go test ./...
go test -tags btrfstest ./internal/service
VREFLINK_VM_RUN=1 go test -tags vmtest ./internal/service
```

## Temporary Artifacts

Temporary test assets live under `.tmp/`:

- `.tmp/service-btrfs-tests/` for the local btrfs service suite
- `.tmp/vm-assets/ubuntu-minimal/` for the cached cloud image and SSH keypair
- `.tmp/vm-integration/share/` for the virtiofs export used by the VM suite
- `.tmp/vm-integration/build/` for the temporary test binaries
- `.tmp/vm-integration/runtime/` for per-run logs, overlays, and cloud-init data

These paths are ignored by git.

## VM Prerequisites

Check the VM prerequisites with:

```bash
scripts/test/vm/check-prereqs.sh
```

The VM suite expects:

- `go`
- `mkfs.btrfs` unless `VREFLINK_VM_SHARE_ROOT` points to your own reflink-capable export
- `ssh`
- `setsid`
- `wget`
- `ssh-keygen`
- `qemu-system-x86_64`
- `qemu-img`
- `cloud-localds`
- `virtiofsd` or `/usr/lib/qemu/virtiofsd`
- `/dev/kvm`
- `/dev/vhost-vsock`

The VM runner will prepare the Ubuntu Minimal image on demand, build the test
binaries, create a temporary loopback btrfs share root unless
`VREFLINK_VM_SHARE_ROOT` is already set, boot the guest, mount virtiofs, run
`vreflink`, and verify post-write copy-on-write behavior.

## VM Environment Variables

The VM scripts support these overrides:

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
set, the VM suite will populate them automatically by calling
`scripts/test/vm/prepare-image.sh`.

## Proxy Use

If your environment needs a proxy for image downloads or package fetches, set
the standard proxy variables before running the scripts:

```bash
export HTTP_PROXY=http://your-proxy:port
export HTTPS_PROXY=http://your-proxy:port
```

The VM helper scripts inherit those environment variables automatically.

## Troubleshooting

- Missing `virtiofsd`: install it or ensure `/usr/lib/qemu/virtiofsd` exists.
- Missing `/dev/kvm`: enable KVM support for the host kernel and device access.
- Missing `/dev/vhost-vsock`: load the module with `sudo modprobe vhost_vsock`.
- Non-btrfs workspace: `scripts/test/run.sh btrfs` requires the repository
  workspace to live on btrfs.

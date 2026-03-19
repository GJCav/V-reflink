# Test Environment

## Host System

| Field | Value |
|---|---|
| OS | Ubuntu 24.04.3 LTS |
| Kernel | 6.14.0-1017-azure (x86\_64) |
| Workspace filesystem | ext4 |
| Go version | go1.25.0 linux/amd64 |
| dpkg version | 1.22.6 (amd64) |
| btrfs-progs | 6.6.3 |

## Available Tools

| Tool | Available |
|---|---|
| dpkg / dpkg-deb | Yes |
| mkfs.btrfs | Yes |
| qemu-system-x86\_64 | **No** |
| virtiofsd | **No** |
| ssh | Yes |

## Environment Constraints

QEMU and virtiofsd were not available in this environment. This means the full
end-to-end VM-based test path (virtiofs mount + vsock RPC + actual reflink
operation) could not be exercised. The following were tested instead:

- Package installation and uninstallation from the provided `.deb`
- CLI help output and flag validation
- `vreflink config init` and config file behavior
- `vreflinkd systemd-unit` template output
- Daemon startup error cases:
  - missing config file
  - nonexistent share root
  - ext4 share root (no reflink support)
  - loopback-mounted btrfs share root (starts successfully)
- Guest CLI path validation error cases
- Package conffile preservation on `dpkg -r` / deletion on `dpkg -P`
- Package maintainer script lifecycle (service stop/disable/purge)

## Reflink-Capable Filesystem

For startup testing, a 200 MiB loopback-mounted btrfs image was created:

```bash
dd if=/dev/zero bs=1M count=200 of=/tmp/btrfs.img
mkfs.btrfs /tmp/btrfs.img
sudo mount -o loop /tmp/btrfs.img /tmp/btrfs-mnt
```

Btrfs on loopback supports reflink. The daemon accepted this as a valid
`share_root` and began listening.

## Version Tested

`vreflink_0.1.2_amd64.deb` and `vreflink_0.1.2_linux_amd64.tar.gz`, built
from source at tag `v0.1.2`.

## Raw Environment Output

See `artifacts/environment-raw.txt`.

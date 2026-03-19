# vreflink

`vreflink` is a guest-side CLI and `vreflinkd` is a host-side daemon for
requesting true host-side reflinks over a virtiofs share.

The data plane is the shared virtiofs mount. The control plane is a single
request/response RPC over AF_VSOCK stream sockets.

## Background

This project is for the common "one VM per project" workflow in scientific and
systems research.

Those projects often need system-wide dependencies, kernel tweaks, or
conflicting toolchains, so putting each project in its own VM keeps the
environment reproducible and isolated. At the same time, large datasets,
checkpoints, and intermediate outputs are expensive to duplicate, so it is
common to expose a host directory into many guest VMs with virtiofs.

That setup works well until you want reflink or copy-on-write semantics inside
the guest. Even when the host filesystem supports reflinks, a guest using the
virtiofs mount cannot reliably create them directly. `vreflink` fills that gap:
the guest still reads and writes through virtiofs, but the reflink operation is
asked of the host daemon, which runs on the real backing filesystem.

Why not just do the reflink inside the guest?

- Reflink on Linux is typically exposed through clone ioctls such as
  `FICLONE`/`FICLONERANGE`, but upstream virtio-fs work on ioctl forwarding has
  explicitly been limited to a small metadata-oriented subset rather than
  general clone ioctls. In the upstream patch series discussed by LWN, the new
  virtiofsd ioctl support is described as "only `FS_IOC_SETFLAGS` and
  `FS_IOC_FSSETXATTR`". That is a strong sign that guest-triggered reflink is
  not part of the supported interface here.
- The current upstream virtiofsd documentation still describes virtio-fs as a
  FUSE-style shared filesystem with documented support for general VFS mount
  options, xattrs, ACLs, DAX, migration, and selected ioctls, but it does not
  document reflink or clone support for guest requests.
- As an inference from those upstream materials: there is no visible upstream
  documentation, interface, or release note today that advertises direct
  reflink support from a virtiofs guest, so users should not expect `cp
  --reflink`, `FICLONE`, or similar guest-side clone requests to reach the host
  backing filesystem.

More background:

- Linux kernel virtio-fs documentation:
  <https://www.kernel.org/doc/html/v6.6/filesystems/virtiofs.html>
- Current upstream virtiofsd documentation:
  <https://docs.rs/crate/virtiofsd/latest>
- Upstream ioctl-support discussion summarized by LWN:
  <https://lwn.net/Articles/872521/>

## Topology

```text
+---------------------------------------------------------------+
| Host                                                          |
|                                                               |
|   backing filesystem: reflink-capable fs                      |
|   share root: /srv/labshare                                   |
|                                                               |
|   +-------------------+          +-------------------------+  |
|   | virtiofsd         |          | vreflinkd               |  |
|   | exports share     |          | performs host reflinks  |  |
|   +---------+---------+          +-----------+-------------+  |
|             |                                ^                |
+-------------|--------------------------------|----------------+
              | virtiofs                       | AF_VSOCK RPC
              v                                |
+-------------+--------------------------------+----------------+
| Guest VM                                                       |
|                                                                |
|   mount: /shared                                               |
|                                                                |
|   apps read/write files on /shared                             |
|   vreflink asks the host to reflink /shared/A -> /shared/B     |
|                                                                |
+----------------------------------------------------------------+
```

In short:

- virtiofs moves the file data.
- vsock carries the reflink request.
- the host filesystem decides whether the reflink is valid and supported.

## Installation

### Prerequisites

`vreflink` is designed for an existing host/guest workflow rather than
provisioning one for you. In the intended setup, the host share is backed by a
reflink-capable filesystem, the guest reaches that share through virtiofs, and
the guest and host can communicate over vsock.

KVM/QEMU with `virtiofsd` is a typical way to provide that environment, but
those pieces are workflow assumptions rather than extra requirements that
`vreflink` installs or manages for you.

Recommended: install the Debian package on both the host and the guest.

```bash
sudo dpkg -i ./vreflink_<version>_amd64.deb
```

The package installs:

- `/usr/bin/vreflink`
- `/usr/bin/vreflinkd`
- `/lib/systemd/system/vreflinkd.service`
- `/etc/vreflinkd/config.toml`
- `/usr/share/vreflink/config.toml`

The package intentionally ships both the guest-side CLI and the host-side
daemon on both sides. That keeps installation symmetrical between host and
guest, and it keeps the packaged templates and documented file layout aligned
wherever you inspect the package.

The service is installed but disabled by default, so edit
`/etc/vreflinkd/config.toml` before enabling it.

Manual install is also available. Recommended manual source: download the
pre-built binaries and templates from GitHub Releases. If you want to build the
artifacts yourself, use the workflow documented in
[`docs/releasing.md`](docs/releasing.md).

Guest manual install:

```bash
sudo install -m 0755 ./vreflink /usr/local/bin/vreflink
/usr/local/bin/vreflink config init
```

Host manual install:

```bash
sudo install -m 0755 ./vreflinkd /usr/local/bin/vreflinkd
/usr/local/bin/vreflinkd systemd-unit | sudo tee /etc/systemd/system/vreflinkd.service >/dev/null
sudo install -d -m 0755 /etc/vreflinkd
sudo cp /path/to/config.toml /etc/vreflinkd/config.toml

sudo systemctl daemon-reload
sudo systemctl enable --now vreflinkd
```

`vreflink config init` writes the guest config template to
`$XDG_CONFIG_HOME/vreflink/config.toml` and refuses to overwrite an existing
file unless `--force` is used. `vreflinkd systemd-unit` prints the canonical
systemd unit template to stdout; it does not install anything. For the daemon
config file, copy the packaged template from the release tarball or the source
tree's `packaging/systemd/vreflinkd.toml`, then edit it for your environment.

## Quick Start

Host config:

```toml
version = 1
share_root = "/srv/labshare"
port = 19090
read_timeout = "5s"
write_timeout = "5s"
log_level = "info"
allow_v1_fallback = false

[[tokens]]
name = "project-a"
token = "project-a-token"
uid = 1001
gid = 1001
groups = [44]
```

Guest config:

```toml
version = 1
mount_root = "/shared"
host_cid = 2
port = 19090
timeout = "5s"
token = "project-a-token"
```

Start the daemon on the host:

```bash
vreflinkd -c /etc/vreflinkd/config.toml
```

Run reflinks from the guest:

```bash
vreflink /shared/A /shared/B
vreflink -r /shared/dirA /shared/dirB
cd /shared/project
vreflink data/A data/B
```

Relative `SRC` and `DST` arguments are resolved from the current working
directory, but the resolved paths must still stay within the configured guest
mount root.

## Authentication

Protocol v2 uses a bearer token from the guest and a host-side token map in
`vreflinkd`'s TOML config. The guest does not claim a `uid` or `gid`. Instead,
each `[[tokens]]` entry maps a token to the host identity that will run the
actual reflink work.

That means:

- the guest sends only the token;
- the host chooses `uid`, `gid`, and supplementary `groups`;
- the reflink is executed under that mapped host identity;
- `groups` is supplementary-group data only and should not repeat the primary
  `gid`.

This is intentionally a trusted-guest design for project VMs, not a per-user
attestation scheme inside the guest. Because the daemon config contains bearer
tokens, recommended permissions are owner `root` and mode `0600`.

## Commands

Common guest commands:

```bash
vreflink SRC DST
vreflink -r SRC DST
vreflink --token TOKEN SRC DST
```

Common host command:

```bash
vreflinkd -c /etc/vreflinkd/config.toml
```

Success means the host executed a real reflink. There is no copy fallback.

## Configuration

Guest CLI settings can come from built-in defaults, the XDG config file,
environment variables, or explicit flags. Precedence is:

```text
flags > environment > $XDG_CONFIG_HOME/vreflink/config.toml > defaults
```

During migration, if `config.toml` is absent, `vreflink` still falls back to
the legacy `~/.config/vreflink/env` file.

Guest TOML keys:

- `mount_root` default: `/shared`
- `host_cid` default: `2`
- `port` default: `19090`
- `timeout` default: `5s`
- `token` default: empty

Compatible guest environment variables:

- `VREFLINK_GUEST_MOUNT_ROOT` default: `/shared`
- `VREFLINK_HOST_CID` default: `2`
- `VREFLINK_VSOCK_PORT` default: `19090`
- `VREFLINK_CLIENT_TIMEOUT` default: `5s`
- `VREFLINK_AUTH_TOKEN` default: empty

Without a config file, you can still use explicit guest flags:

```bash
vreflink --token project-a-token --mount-root /shared --cid 2 --port 19090 /shared/A /shared/B
vreflink -r --token project-a-token --mount-root /shared --cid 2 --port 19090 /shared/dirA /shared/dirB
```

If the guest XDG config file exists but is malformed, `vreflink` exits with a
clear startup error.

`vreflinkd` is config-file-driven. By default it loads
`/etc/vreflinkd/config.toml`, or another path passed with `-c`.

Daemon TOML keys:

- `share_root` default: `/srv/labshare`
- `port` default: `19090`
- `read_timeout` default: `5s`
- `write_timeout` default: `5s`
- `log_level` default: `info`
- `allow_v1_fallback` default: `false`

`vreflinkd` validates `share_root` before it starts listening. Startup fails if
the path does not exist, is not a directory, is not writable for the probe
files, or cannot complete a reflink probe. By default, startup also fails
closed if there are no usable token entries. Set `allow_v1_fallback = true`
only if you explicitly want unauthenticated legacy v1 mode.

## Failure Modes

- Destination already exists: the request fails with `EEXIST`.
- Symlinks, hardlinks, device nodes, FIFOs, and sockets are rejected.
- Recursive mode is fail-fast and non-transactional, so a partial destination
  tree may remain after the first error.
- The daemon refuses startup if the configured share root is not a usable
  reflink-capable directory.
- Raw filesystem authorization failures are reported as `permission denied`,
  while path-containment failures keep their explicit shared-root messages.
- Missing or unknown authentication tokens fail once a token map is configured.
- There is no fallback copy path. If the host filesystem does not support
  reflinks for the requested source and destination, the request fails with
  `EOPNOTSUPP`.

## Development

Contributor workflows are documented separately:

- testing: [`docs/testing.md`](docs/testing.md)
- packaging and release flow: [`docs/releasing.md`](docs/releasing.md)

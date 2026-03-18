# vreflink

`vreflink` is a guest-side CLI and `vreflinkd` is a host-side daemon for
requesting true host-side btrfs reflinks over a virtiofs share.

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
|   backing filesystem: btrfs                                   |
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

## Commands

```bash
vreflink SRC DST
vreflink -r SRC DST
```

Success means the host executed a real reflink. There is no copy fallback.

## Usage

Host:

```bash
vreflinkd --share-root /srv/labshare --port 19090
```

Guest:

```bash
vreflink /shared/A /shared/B
vreflink -r /shared/dirA /shared/dirB
cd /shared/project
vreflink data/A data/B
```

`vreflink` can auto-load its common guest-side settings from
`$XDG_CONFIG_HOME/vreflink/env`, which is typically `~/.config/vreflink/env`.
Without that file, you can still use explicit flags:

```bash
vreflink --mount-root /shared --cid 2 --port 19090 /shared/A /shared/B
vreflink -r --mount-root /shared --cid 2 --port 19090 /shared/dirA /shared/dirB
```

Relative `SRC` and `DST` arguments are resolved from the current working
directory, but the resolved paths must still stay within the configured guest
mount root.

## Build

```bash
go build ./...
```

## Configuration

`vreflink` CLI settings can come from built-in defaults, the XDG config file,
environment variables, or explicit flags. Precedence is:

```text
flags > environment > $XDG_CONFIG_HOME/vreflink/env > defaults
```

Example guest config file:

```bash
# ~/.config/vreflink/env
VREFLINK_GUEST_MOUNT_ROOT=/shared
VREFLINK_HOST_CID=2
VREFLINK_VSOCK_PORT=19090
```

CLI keys:

- `VREFLINK_GUEST_MOUNT_ROOT` default: `/shared`
- `VREFLINK_HOST_CID` default: `2`
- `VREFLINK_VSOCK_PORT` default: `19090`
- `VREFLINK_CLIENT_TIMEOUT` default: `5s`

If the XDG config file exists but is malformed, `vreflink` exits with a clear
startup error.

Daemon environment variables:

`vreflinkd` does not auto-load the XDG guest config file. It still uses the
daemon environment variables below, typically through systemd.

- `VREFLINK_SHARE_ROOT` default: `/srv/labshare`
- `VREFLINK_VSOCK_PORT` default: `19090`
- `VREFLINK_READ_TIMEOUT` default: `5s`
- `VREFLINK_WRITE_TIMEOUT` default: `5s`

`vreflinkd` validates `VREFLINK_SHARE_ROOT` before it starts listening. Startup
fails if the path does not exist, is not a directory, is not writable for the
probe files, or cannot complete a reflink probe.

## Testing

```bash
scripts/test/run.sh quick
scripts/test/run.sh btrfs
scripts/test/run.sh vm
scripts/test/run.sh release
```

`quick` is the default contributor path. Use `btrfs` for real local reflink
validation, `vm` for the full guest/host virtiofs + vsock path, and `release`
for packaging/install verification. The full testing guide lives in
[`docs/testing.md`](docs/testing.md).

## Deployment

Release artifacts:

- GitHub Releases will publish:
  - `vreflink_<version>_linux_amd64.tar.gz`
  - `vreflink_<version>_amd64.deb`
  - `vreflink_<version>_sha256sums.txt`
- The Debian package is directly installable with `dpkg -i`; no PPA or package
  registry is required.

Debian/Ubuntu:

```bash
sudo dpkg -i ./vreflink_<version>_amd64.deb
```

The package installs:

- `/usr/bin/vreflink`
- `/usr/bin/vreflinkd`
- `/lib/systemd/system/vreflinkd.service`
- `/etc/default/vreflinkd`

The service is installed but disabled by default.

Manual binary install:

```bash
sudo ./vreflink install
./vreflink config init

sudo ./vreflinkd install
./vreflinkd systemd-unit
```

`vreflink config init` writes the guest config template to
`$XDG_CONFIG_HOME/vreflink/env` and refuses to overwrite an existing file
unless `--force` is used.

`vreflinkd systemd-unit` prints the canonical systemd unit to stdout so it can
be reviewed or customized before installation.

Local artifact build details live in [`docs/releasing.md`](docs/releasing.md).

## Failure Modes

- Destination already exists: the request fails with `EEXIST`.
- Symlinks, hardlinks, device nodes, FIFOs, and sockets are rejected.
- Recursive mode is fail-fast and non-transactional, so a partial destination
  tree may remain after the first error.
- The daemon refuses startup if the configured share root is not a usable
  reflink-capable directory.
- There is no fallback copy path. If the host filesystem does not support
  reflinks for the requested source and destination, the request fails with
  `EOPNOTSUPP`.

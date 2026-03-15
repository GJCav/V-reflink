# vreflink

`vreflink` is a guest-side CLI and `vreflinkd` is a host-side daemon for
requesting true host-side btrfs reflinks over a virtiofs share.

The data plane is the shared virtiofs mount. The control plane is a single
request/response RPC over AF_VSOCK stream sockets.

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
```

`vreflink` can auto-load its common guest-side settings from
`$XDG_CONFIG_HOME/vreflink/env`, which is typically `~/.config/vreflink/env`.
Without that file, you can still use explicit flags:

```bash
vreflink --mount-root /shared --cid 2 --port 19090 /shared/A /shared/B
vreflink -r --mount-root /shared --cid 2 --port 19090 /shared/dirA /shared/dirB
```

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

## Testing

```bash
scripts/test/run.sh quick
scripts/test/run.sh btrfs
scripts/test/run.sh vm
```

`quick` is the default contributor path. Use `btrfs` for real local reflink
validation and `vm` for the full guest/host virtiofs + vsock path. The full
testing guide lives in [`docs/testing.md`](docs/testing.md).

## Deployment

Host install helpers:

- [`scripts/install-host.sh`](scripts/install-host.sh)
- [`packaging/systemd/vreflinkd.service`](packaging/systemd/vreflinkd.service)
- [`packaging/systemd/vreflinkd.env`](packaging/systemd/vreflinkd.env)

Guest install helper:

- [`scripts/install-guest.sh`](scripts/install-guest.sh)

## Failure Modes

- Destination already exists: the request fails with `EEXIST`.
- Symlinks, hardlinks, device nodes, FIFOs, and sockets are rejected.
- Recursive mode is fail-fast and non-transactional, so a partial destination
  tree may remain after the first error.
- There is no fallback copy path. If the host filesystem does not support
  reflinks for the requested source and destination, the request fails with
  `EOPNOTSUPP`.

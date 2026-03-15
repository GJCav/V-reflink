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
vreflink --mount-root /shared --cid 2 --port 19090 /shared/A /shared/B
vreflink -r --mount-root /shared --cid 2 --port 19090 /shared/dirA /shared/dirB
```

## Build

```bash
go build ./...
```

## Configuration

CLI environment variables:

- `VREFLINK_GUEST_MOUNT_ROOT` default: `/shared`
- `VREFLINK_HOST_CID` default: `2`
- `VREFLINK_VSOCK_PORT` default: `19090`
- `VREFLINK_CLIENT_TIMEOUT` default: `5s`

Daemon environment variables:

- `VREFLINK_SHARE_ROOT` default: `/srv/labshare`
- `VREFLINK_VSOCK_PORT` default: `19090`
- `VREFLINK_READ_TIMEOUT` default: `5s`
- `VREFLINK_WRITE_TIMEOUT` default: `5s`

## Testing

```bash
go test ./...
scripts/test-local-btrfs.sh
scripts/test-local-btrfs.sh --race
```

The repo also includes lightweight VM helper scripts and notes in
[`docs/vm-testing.md`](/home/jcav/V-reflink/docs/vm-testing.md) for virtiofs +
vsock integration work without pulling in a full libvirt stack.

## Deployment

Host install helpers:

- [`scripts/install-host.sh`](/home/jcav/V-reflink/scripts/install-host.sh)
- [`packaging/systemd/vreflinkd.service`](/home/jcav/V-reflink/packaging/systemd/vreflinkd.service)
- [`packaging/systemd/vreflinkd.env`](/home/jcav/V-reflink/packaging/systemd/vreflinkd.env)

Guest install helper:

- [`scripts/install-guest.sh`](/home/jcav/V-reflink/scripts/install-guest.sh)

## Failure Modes

- Destination already exists: the request fails with `EEXIST`.
- Symlinks, hardlinks, device nodes, FIFOs, and sockets are rejected.
- Recursive mode is fail-fast and non-transactional, so a partial destination
  tree may remain after the first error.
- There is no fallback copy path. If the host filesystem does not support
  reflinks for the requested source and destination, the request fails with
  `EOPNOTSUPP`.

# VM Testing Notes

The local workspace is already on btrfs, so most day-to-day verification can
use `go test ./...` directly. For full guest/host validation over virtiofs +
vsock, this repo includes lightweight QEMU helpers instead of a heavier
libvirt-based stack.

## Prerequisites

- `qemu-system-x86_64`
- `virtiofsd` or `/usr/lib/qemu/virtiofsd`
- `/dev/kvm`
- `/dev/vhost-vsock`

Check them with:

```bash
scripts/vm/check-prereqs.sh
```

For a self-preparing VM-backed integration run, use:

```bash
scripts/vm/run-integration-test.sh
```

A Go wrapper for that flow is also available behind the `vmtest` build tag:

```bash
VREFLINK_VM_RUN=1 go test -tags vmtest ./internal/service
```

The integration script will, by default:

- download Ubuntu Minimal cloud image assets once into `.tmp/vm-assets`
- generate an SSH key for the VM test user
- create a fresh qcow2 overlay and cloud-init seed per run
- boot the guest with virtiofs and vsock enabled
- run the guest CLI against the host daemon
- verify that the destination content is correct and that post-write copy-on-write behavior holds

## Start a Guest

```bash
scripts/vm/run-qemu-vsock-virtiofs.sh \
  --disk ./.tmp/vm-assets/ubuntu-minimal/ubuntu-22.04-minimal-cloudimg-amd64.img \
  --seed-iso ./seed.iso \
  --firmware uefi \
  --share /srv/labshare \
  --cid 4 \
  --memory 1024 \
  --ssh-port 2222
```

The script:

- starts `virtiofsd`
- exposes the share as the guest tag `shared`
- enables vsock with the requested guest CID
- forwards host TCP port `2222` to guest SSH `22`

Mount the share inside the guest with:

```bash
sudo mkdir -p /shared
sudo mount -t virtiofs shared /shared
```

Then run the host daemon and guest CLI:

```bash
sudo ./vreflinkd --share-root /srv/labshare --port 19090
./vreflink --mount-root /shared --cid 2 --port 19090 /shared/A /shared/B
```

## Proxy Use

If the guest or host needs a proxy during image setup or dependency fetches:

```bash
export HTTP_PROXY=http://192.168.55.1:7890
export HTTPS_PROXY=http://192.168.55.1:7890
```

The helper scripts respect those environment variables automatically.

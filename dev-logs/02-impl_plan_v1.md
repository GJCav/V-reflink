# 2026-03-15 Development Log

## Setup

- Installed Go 1.24 from Ubuntu packages.
- Configured Go module downloads to use `https://goproxy.cn,direct`.
- Used the supplied proxy `http://192.168.55.1:7890` for network-sensitive steps.

## Implementation

- Created the Go module and the v1 package layout from the pinned plan.
- Implemented:
  - Cobra-based `vreflink` CLI
  - `vreflinkd` vsock daemon
  - length-prefixed JSON framing
  - request/response protocol and coded error mapping
  - guest-path to relative-path conversion
  - host root-anchored path validation with `filepath-securejoin`
  - single-file reflink and recursive tree walking
  - symlink and hardlink rejection
  - structured daemon logging
- Added lightweight scripts and docs for local btrfs testing and QEMU-based VM testing.
- Expanded the test matrix to cover request validation, destination-exists behavior, service-level symlink and hardlink rejection, recursive success, a same-destination concurrency race, a client/server round-trip, and a VM integration harness behind an explicit build tag.
- Added deployment artifacts: host and guest install scripts plus a systemd unit/defaults file for `vreflinkd`.
- Installed QEMU, OVMF, cloud image tools, and supporting utilities on the host to execute the full VM-backed test path.
- Added a reusable Ubuntu Minimal image-preparation helper and upgraded the VM runner to support cloud-init seed media, UEFI, common `virtiofsd` install paths, and sudo fallback when `/dev/kvm` or `/dev/vhost-vsock` require it.

## Verification Plan

- Run `gofmt` and `go test ./...`.
- Use local btrfs integration tests because the repo itself is on btrfs.
- Keep VM testing scripts ready for full virtiofs + vsock verification without requiring libvirt.

## Verification Results

- `gofmt -w $(find cmd internal -name '*.go')`
- `go mod tidy`
- `go test ./...`
- `bash -n scripts/test-local-btrfs.sh scripts/vm/check-prereqs.sh scripts/vm/run-qemu-vsock-virtiofs.sh`
- `go test -race ./...`
- `scripts/vm/run-integration-test.sh`
- `VREFLINK_VM_RUN=1 go test -tags vmtest ./internal/service`
- `scripts/test-local-btrfs.sh`
- `scripts/test-local-btrfs.sh --race`

All Go tests passed.

Installed host-side VM/test packages:

- `qemu-system-x86`
- `qemu-utils`
- `cloud-image-utils`
- `ovmf`

VM-backed integration status:

- Downloaded and cached a Ubuntu Minimal cloud image under `.tmp/vm-assets`
- Booted a guest with UEFI, virtiofs, and vsock enabled
- Verified guest `vreflink` -> host `vreflinkd` over vsock
- Verified destination content matches source
- Verified post-write copy-on-write behavior by mutating the destination and confirming the source remained unchanged

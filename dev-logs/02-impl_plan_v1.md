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
- Added XDG guest config loading plus `vreflink config init` for bootstrapping the guest config file.
- Replaced the standalone guest/host install shell scripts with binary subcommands:
  - `vreflink install`
  - `vreflink config init`
  - `vreflinkd install`
  - `vreflinkd systemd-unit`
- Standardized installed binary paths on `/usr/bin` and updated the systemd unit template accordingly.
- Added local release packaging for:
  - a combined `linux-amd64` tarball
  - a combined local-installable `amd64` `.deb`
  - release checksums
- Added GitHub Actions CI/release workflows that call the same local release scripts used for manual builds.

## Verification Plan

- Run `gofmt` and `go test ./...`.
- Use local btrfs integration tests because the repo itself is on btrfs.
- Keep VM testing scripts ready for full virtiofs + vsock verification without requiring libvirt.

## Verification Results

- `gofmt -w $(find cmd internal -name '*.go')`
- `go mod tidy`
- `go test ./...`
- `go test -tags btrfstest ./internal/service`
- `bash -n scripts/test/run.sh scripts/test/vm/check-prereqs.sh scripts/test/vm/boot-qemu.sh scripts/test/vm/prepare-image.sh scripts/test/vm/run.sh`
- `go test -race ./...`
- `scripts/test/run.sh quick`
- `scripts/test/run.sh quick --race`
- `scripts/test/run.sh btrfs`
- `scripts/test/run.sh btrfs --race`
- `scripts/test/run.sh vm`
- `scripts/test/run.sh release`
- `VREFLINK_VM_RUN=1 go test -tags vmtest ./internal/service`

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

## Test Refactor

- Split the service tests into explicit quick, `btrfstest`, and `vmtest` suites.
- Added `internal/testsupport` to remove duplicated mock reflinker, coded-error assertions, and repo temp helpers.
- Replaced ad hoc test scripts with the unified `scripts/test/run.sh` entrypoint and moved VM helpers under `scripts/test/vm/`.
- Consolidated the testing notes into `docs/testing.md`.

## Release Packaging

- Added embedded canonical packaging assets so the binaries, release scripts, and `.deb` all share the same guest config and host systemd/defaults templates.
- Added a local release builder in `scripts/release/build.sh`.
- Added a release validation stage in `scripts/test/release/run.sh` that:
  - builds the tarball and `.deb`
  - checks package metadata and contents with `dpkg-deb`
  - installs the `.deb` into a temporary package root with `dpkg`
  - verifies both installed binaries run with `--help`
  - verifies the service is not enabled by default

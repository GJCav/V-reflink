# vreflink User Trial Report

## Executive Summary

I evaluated `vreflink` as a Linux-based research user who is comfortable with normal CLI work but not already fluent in virtiofs, vsock, or reflink internals. I followed the repository `README.md` as the primary source of truth, installed only the packaged artifacts from `./dist`, exercised the main workflows end to end in a real host/guest setup, and then uninstalled everything.

Bottom line: the core product works. Once a valid virtiofs + vsock environment existed, `vreflink` successfully created real host-side reflinks from inside the guest. The CLI was understandable, `config init` was helpful, several validation errors were clear, and the daemon correctly failed closed on unsupported host filesystems.

I would still hesitate to adopt it today because the packaging and cleanup story does not yet feel trustworthy:

- the Debian package contents do not match the README;
- `dpkg -r` removed `/etc/vreflinkd/config.toml` immediately instead of preserving it for purge;
- removing the enabled host package left the daemon process running from a deleted binary and left systemd in a stale state that required manual cleanup.

Those are not cosmetic issues. They affect operator trust, reversibility, and safe adoption.

I did not inspect the source code during this trial.

## Test Environment

See `environment.md` for the full environment summary and `artifacts/environment-raw.txt` for raw command output.

High-level environment:

- Host OS: Ubuntu 24.04.4 LTS
- Host kernel: `6.17.0-19-generic`
- Host workspace filesystem: `ext4`
- Reflink-capable share used for testing: temporary 2 GiB loopback-mounted `btrfs`
- Virtualization: QEMU 8.2.2 with KVM, virtiofs, and vhost-vsock
- Guest OS: Ubuntu 24.04.4 LTS cloud image
- Guest kernel: `6.8.0-101-generic`

## What I Attempted

1. Read the README first and used it as the main guide.
2. Inspected the provided `.deb` and tarball in `./dist`.
3. Installed the `.deb` on the host with `dpkg -i`.
4. Verified the installed files, help output, config behavior, and service state.
5. Tested host daemon startup with:
   - the packaged default config;
   - an existing but unsupported `ext4` share root;
   - a working `btrfs` share root.
6. Built a temporary end-to-end environment:
   - loopback-mounted `btrfs` share;
   - `virtiofsd`;
   - Ubuntu guest VM with vsock and virtiofs.
7. Mounted the shared directory in the guest and installed the same `.deb` there from `/shared/dist`.
8. Ran guest workflows:
   - `vreflink /shared/testdata/big.bin /shared/testdata/big-clone.bin`
   - relative-path reflink from within `/shared/testdata`
   - recursive directory reflink with `-r`
9. Verified that reflinks were real host-side shared extents using host-side `btrfs filesystem du` and `filefrag`.
10. Tested negative cases:
   - missing token;
   - wrong token;
   - destination already exists;
   - path outside mount root;
   - symlink source;
   - recursive copy failure leaving partial destination state.
11. Uninstalled from guest and host, then tore down the VM, virtiofs, and loopback filesystem.

## What Worked Well

- The README explains the host/guest split better than many low-level projects do. The topology and control-plane/data-plane distinction were understandable.
- `vreflinkd` failed closed with good startup errors:
  - default config on a nonexistent share root: clear failure;
  - existing `ext4` share root: clear reflink capability failure.
- `vreflink config init` is a strong first-run command:
  - it writes a sensible guest template;
  - it uses expected XDG paths;
  - it refuses to overwrite existing config unless forced.
- The main workflows worked once the environment existed:
  - single-file reflink;
  - relative-path reflink;
  - recursive directory reflink.
- Core error messages were mostly understandable:
  - `destination already exists`
  - `path must stay within the guest mount root`
  - `symlinks are not supported`
  - `invalid authentication token`
- The product delivered real reflinks, not silent full copies. Host-side verification showed shared extents before mutation and a small exclusive extent after writing to the clone.

## Problems Found

### Bugs

#### 1. Debian package omits a file the README says it installs

- Severity: Medium
- Type: Bug / packaging mismatch
- Reproduction:
  1. Inspect `dist/vreflink_0.1.5_amd64.deb` with `dpkg-deb -c`.
  2. Install it with `sudo dpkg -i dist/vreflink_0.1.5_amd64.deb`.
  3. Check `/usr/share/vreflink`.
- Expected:
  - Package contents should match the README, including `/usr/share/vreflink/config.toml`.
- Actual:
  - The `.deb` installed `/usr/bin/vreflink`, `/usr/bin/vreflinkd`, the unit file, and `/etc/vreflinkd/config.toml`, but not `/usr/share/vreflink/config.toml`.
- Evidence:
  - `artifacts/deb-contents.txt`
  - `artifacts/postinstall-files.txt`
  - `artifacts/guest-postinstall-files-verified.txt`

#### 2. `dpkg -r` deletes `/etc/vreflinkd/config.toml` immediately

- Severity: High
- Type: Bug / packaging / data-loss risk
- Reproduction:
  1. Install the package.
  2. Edit `/etc/vreflinkd/config.toml`.
  3. Run `sudo dpkg -r vreflink`.
  4. Check whether `/etc/vreflinkd/config.toml` remains.
- Expected:
  - A normal package removal should typically leave admin-edited config behind and require purge for full deletion.
- Actual:
  - `dpkg -r` removed the config file and directory immediately on both host and guest.
  - A later `dpkg -P vreflink` had nothing left to purge.
- Evidence:
  - `artifacts/guest-uninstall.txt`
  - `artifacts/host-remove-postcheck.txt`

#### 3. Removing the enabled host package leaves the daemon running from a deleted binary

- Severity: High
- Type: Bug / uninstall / service lifecycle
- Reproduction:
  1. Install the package on the host.
  2. Enable and start `vreflinkd.service`.
  3. Run `sudo dpkg -r vreflink`.
  4. Check `systemctl status vreflinkd`.
- Expected:
  - Package removal should stop/disable the service cleanly, or at minimum not leave a stale enabled unit and running process.
- Actual:
  - The package files were deleted.
  - `vreflinkd` kept running from the deleted binary.
  - `systemctl disable vreflinkd` later failed because the unit file no longer existed on disk.
  - I had to manually stop the service and run `systemctl daemon-reload`.
- Evidence:
  - `artifacts/host-remove-postcheck.txt`
  - `artifacts/host-systemd-cleanup.txt`

### Documentation Problems

- The README clearly explains what the project is, but it does not provide an end-to-end prerequisite checklist for someone evaluating it from scratch.
- Practical hidden prerequisites included:
  - a working virtiofs environment;
  - a reflink-capable backing filesystem;
  - vsock-enabled VM setup;
  - enough QEMU/virtiofs knowledge to bring those pieces together.
- The README’s package-install file list is inaccurate because the `.deb` does not include `/usr/share/vreflink/config.toml`.
- There are no uninstall or cleanup instructions.
- The README does not warn that the package installs both guest and host binaries/config on both sides. That is technically described in spirit, but still surprising in practice when a guest-only test install drops `/etc/vreflinkd/config.toml` and `vreflinkd.service`.

### Usability Problems

- The main product workflow becomes pleasant only after significant infrastructure already exists. A researcher evaluating the project from scratch still has to solve a large amount of virtiofs/vsock plumbing outside the README.
- The missing-token error message, `token-authenticated requests require protocol version 2`, is technically defensible but not very user-centered. For a person who simply forgot to set the token, a more direct message would help.
- The daemon logs were helpful for successful requests and some server-side failures, but uninstall behavior undermined confidence more than the runtime behavior did.

## UX / DX Feedback From the Target Persona

From this persona, the product idea is compelling and the successful path is promising. When it works, it feels elegant: a small CLI, a small daemon, no fake copy fallback, and clean failures when the host filesystem cannot do what was asked.

The rough part is adoption confidence. I had to work much harder on infrastructure than on the product itself, and once I finally trusted the reflink path, the package removal behavior immediately damaged that trust again. For a research workflow tool, reversibility matters. If installation and removal are not boring and safe, adoption gets harder even when the core feature works.

## Suggested Improvements

Prioritized by impact:

1. Fix Debian packaging so `/etc/vreflinkd/config.toml` is treated as a real conffile and survives `dpkg -r`.
2. Add proper package maintainer scripts or equivalent lifecycle handling so removing the package cleanly stops/disables the service.
3. Make the README’s package contents section accurate, or fix the package to include `/usr/share/vreflink/config.toml` as documented.
4. Add a practical “Prerequisites” section near the top:
   - host needs a reflink-capable filesystem;
   - guest needs virtiofs mount and vsock path to host;
   - project assumes existing VM/virtiofs setup rather than creating it.
5. Add an “Uninstall / cleanup” section with exact commands and expected leftover state.
6. Consider splitting packages or artifacts into host and guest variants, or at least explain why both sides currently receive the full bundle.
7. Improve the missing-token error wording so it tells the user what to set.
8. Add a minimal smoke-test recipe in the README or docs for validating a setup safely after install.

## Final Verdict

I would hesitate before adopting this project in a real research workflow today.

Reason:

- the core feature is real and useful;
- the product behavior during normal operation is promising;
- but the packaging and uninstall behavior are not yet trustworthy enough for a tool that wants root-owned config, systemd integration, and host-side filesystem operations.

If the packaging/removal issues were fixed and the README added a sharper prerequisite and cleanup story, I would be much more comfortable recommending it.

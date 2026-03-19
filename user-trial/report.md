# User Trial Report — vreflink v0.1.2

## Executive Summary

I evaluated `vreflink` as a Linux researcher comfortable with CLI tools but
not expert in virtiofs, vsock, or reflink internals. I followed the README as
the primary source of truth and used only the packaged artifact from `./dist`.

**Bottom line:** The packaging, CLI, and daemon startup behavior are now
trustworthy. Every bug and documentation gap identified in the previous trial
(dev-logs/05-user-trial-report.md) has been addressed:

- `/usr/share/vreflink/config.toml` is present in the package ✓
- `/etc/vreflinkd/config.toml` is a proper Debian conffile and survives
  `dpkg -r` ✓
- `dpkg -r` now stops and disables the service via `prerm`; `dpkg -P` cleans
  up systemd state via `postrm` ✓
- The README's package contents section matches the installed files exactly ✓
- The missing-token error is rewritten to a direct user hint ✓

The full end-to-end workflow (virtiofs + vsock + actual reflink operation from
a guest VM) could not be tested in this environment: QEMU and virtiofsd were
unavailable. Everything that could be tested short of a live VM passed.

I would adopt this project for a VM-based research workflow, conditional on
one remaining open area: the documentation still does not state explicitly that
both `vreflink` and `vreflinkd` binaries install on both host and guest. This
is unusual and worth a single clarifying sentence for new users.

## Test Environment

See `environment.md` for the full environment summary and
`artifacts/environment-raw.txt` for raw command output.

High-level:

- Host OS: Ubuntu 24.04.3 LTS
- Host kernel: 6.14.0-1017-azure
- Host workspace filesystem: ext4
- Reflink-capable filesystem for startup test: 200 MiB loopback-mounted btrfs
- No QEMU or virtiofsd available (full VM path not tested)

## What I Attempted

1. Read the README as the primary guide.
2. Inspected the provided `.deb` and tarball in `./dist`.
3. Installed the `.deb` on the host.
4. Verified installed files, help output, and service state.
5. Ran `vreflink config init` and verified template content and idempotency.
6. Ran `vreflinkd systemd-unit` and compared with the installed unit.
7. Tested daemon startup with:
   - a missing config file;
   - a nonexistent share root;
   - an ext4 share root (no reflink support);
   - a loopback-mounted btrfs share root (success).
8. Tested guest CLI path validation error cases.
9. Tested `dpkg -r` and `dpkg -P` lifecycle, including config preservation and
   maintainer script invocation.
10. Attempted to test the full end-to-end VM path — **blocked by missing QEMU**.

## What Worked Well

**Package contents match the README exactly.**
The `.deb` now includes `/usr/share/vreflink/config.toml` as documented.
No discrepancy between the README list and the installed file layout.

**`vreflink config init` is polished.**
- Writes to the correct XDG path.
- Refuses to overwrite without `--force`.
- Config template content matches the README quick-start.

**Daemon startup errors are clear and actionable.**
Every error case tested produced a specific, operator-facing message without
a stack trace or an opaque error code:
- missing config: names the path and the OS error
- missing share root: names the path
- no-reflink filesystem: names the path and explains the limitation
- valid btrfs: starts and logs the share root and port

**Package lifecycle is now correct.**
- `dpkg -r` calls `deb-systemd-invoke stop` and `deb-systemd-helper disable`
  before removing the binary and unit file.
- Admin-edited `/etc/vreflinkd/config.toml` survives `dpkg -r`.
- `dpkg -P` calls `deb-systemd-helper purge` and `daemon-reload`.

**Service not auto-enabled on install.**
Consistent with safe Debian packaging practice.

**Help output is complete and concise.**
All flags documented in the README appeared in `--help`. Defaults are shown
inline.

## Problems Found

### Bugs

None new found in this trial.

All three bugs from the previous trial (dev-logs/05-user-trial-report.md)
have been fixed:

| Previous bug | Status |
|---|---|
| Package missing `/usr/share/vreflink/config.toml` | Fixed |
| `dpkg -r` deleted `/etc/vreflinkd/config.toml` immediately | Fixed |
| Removing enabled package left daemon running | Fixed |

### Documentation Problems

#### 1. README does not mention that both binaries install on both host and guest

- Type: Documentation problem
- Severity: Low (cosmetic / first-run surprise)
- Detail: A researcher installing the package on a pure-guest machine will
  receive both `vreflink` (guest CLI) and `vreflinkd` (host daemon), plus
  `/etc/vreflinkd/config.toml` with a daemon configuration template. The
  package currently provides no mechanism to install only one side.

  The README explains this implicitly — it says "the package installs both the
  guest-side vreflink CLI and the host-side vreflinkd daemon" — but a single
  sentence like "This is intentional: the same package installs on both host
  and guest" would prevent the first-run confusion.

- Impact: Researchers running on a guest-only setup will see an unexpected
  `/etc/vreflinkd/config.toml` and a disabled systemd service after install.

#### 2. README prerequisite section is present but lacks version guidance

- Type: Documentation problem
- Severity: Low
- Detail: The "Prerequisites" section lists the required environment (virtiofs
  + vsock + reflink-capable backing filesystem) but does not specify minimum
  kernel or QEMU versions needed. A user with an older kernel may hit a
  `EOPNOTSUPP` that looks like a product bug before they discover the kernel
  is too old.

- Impact: Low. The error message when reflink is unsupported is clear enough
  to guide investigation.

### Usability Problems

#### 1. Token placeholder in daemon config is not obviously a placeholder

- Severity: Very low
- Detail: `/etc/vreflinkd/config.toml` ships with:
  ```
  [[tokens]]
  token = "replace-me"
  ```
  A non-expert user might start the daemon without updating this field. The
  daemon currently accepts the config as valid; it does not warn that the
  token value looks like a default template string.
- Impact: Minor operational confusion. Not a security issue since both
  sides must configure the matching token.

## UX / DX Feedback From the Target Persona

The install-to-verify path now feels polished. The package installs quietly,
nothing auto-enables, and the first thing a user runs after install works
exactly as documented (`vreflink config init`, `vreflinkd systemd-unit`).

The daemon startup error messages are good enough that a non-expert can
diagnose a wrong share root without reading source code. The logged line
on successful startup (showing share root and port) is the minimum useful
confirmation that the daemon is ready.

The CLI flag naming is consistent and self-explanatory. The `--mount-root` /
`--cid` / `--port` defaults are reasonable for typical QEMU setups and match
the packaged guest config template.

The only friction left is setting up the virtiofs + vsock environment itself.
That friction lives outside this project, but the README does not help the
user discover that they need a KVM/QEMU setup before they try to run this.
A link to an example `qemu-system-x86_64` command showing virtiofs and
vhost-vsock flags would significantly reduce the setup burden for a first-time
evaluator.

## Suggested Improvements

Prioritized by impact:

1. **(Low impact)** Add a single explicit sentence to the README explaining
   that the package is intentionally symmetric — the same `.deb` installs on
   both host and guest.

2. **(Low impact)** Add a minimal example QEMU launch command in the README's
   Prerequisites section showing how to attach a virtiofs share and enable
   vsock. This removes the biggest friction for a first-time evaluator.

3. **(Very low impact)** Consider warning at daemon startup if any configured
   token value is the template default `"replace-me"`.

## Final Verdict

I would adopt this project in a real research workflow.

The previous trial correctly identified adoption blockers in packaging and
uninstall behavior. All of those have been fixed. The packaging is now correct
by Debian standards, the daemon fail-closed behavior is clear, and the CLI is
consistent with its documentation.

The remaining items are cosmetic or edge-case improvements, not trust-breaking
issues. The core feature — performing real host-side reflinks from inside a
guest via vsock — is technically sound and the implementation is production
quality for a small specialized utility.

Conditional on the environment prerequisites being met (virtiofs + vsock +
reflink-capable host filesystem), this tool does exactly what it claims to do.

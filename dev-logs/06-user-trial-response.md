# 2026-03-19 User Trial Response

This log records our design choices in response to
`dev-logs/05-user-trial-report.md`.

## Goal

Address the trust-breaking issues from the user trial without expanding the
project scope beyond its intended host/guest workflow.

## Decisions

### Accepted

1. **Fix the `.deb` / README mismatch**

   The Debian package must install `/usr/share/vreflink/config.toml` exactly as
   documented.

2. **Preserve admin-edited daemon config on remove**

   `/etc/vreflinkd/config.toml` is treated as a real Debian conffile.
   `dpkg -r` must preserve it, and `dpkg -P` must remove it.

3. **Stop and disable the daemon during package removal**

   The package must not leave `vreflinkd` running from a deleted binary or
   leave systemd in a stale state after removal.

4. **Add a short prerequisites section**

   The README now states the intended workflow assumptions explicitly:

   - reflink-capable host backing filesystem;
   - virtiofs-mounted guest share;
   - vsock connectivity between guest and host;
   - typical KVM/QEMU/`virtiofsd` environment.

   These are not extra project requirements. They describe the environment that
   `vreflink` is designed for.

5. **Keep one package for both host and guest**

   We explicitly decided **not** to split the package. The single package keeps
   installation symmetrical and keeps the documented file layout and packaged
   templates consistent on both host and guest.

6. **Improve the missing-token UX**

   We kept the existing protocol/version behavior, but the guest CLI now
   rewrites the specific missing-token daemon error into a direct user-facing
   hint:

   `authentication token is required; set token in config or pass --token`

7. **Add package lifecycle tests**

   The release suite now verifies:

   - `.deb` contents;
   - Debian control metadata (`conffiles`, `prerm`, `postrm`);
   - install layout;
   - `dpkg --remove` config preservation;
   - daemon stop/disable behavior during removal;
   - `dpkg --purge` config deletion.

### Rejected

1. **Do not add an uninstall / cleanup section to the README**

   For the Debian-package path, removal behavior should be correct by design.
   For the manual-install path, users are already opting into manual lifecycle
   management. Adding cleanup instructions to the README would add noise rather
   than solve the real problem.

2. **Do not split host and guest packages**

   The surprise should be solved by documentation and package behavior, not by
   introducing multiple package variants.

## Implementation Notes

- Debian maintainer scripts prefer `deb-systemd-helper` and
  `deb-systemd-invoke`, with `systemctl` fallback.
- The unified test entry remains the supported interface:

  - `go run ./cmd/vreflink-dev test all`
  - `go run ./cmd/vreflink-dev test release`

## Outcome

The user trial correctly identified adoption blockers. We responded by fixing
packaging trust, clarifying the intended deployment assumptions, and improving a
high-friction CLI error, while deliberately keeping the project small and the
README focused.

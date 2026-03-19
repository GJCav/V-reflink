# Releasing

Local release artifacts are built from:

```bash
go run ./cmd/vreflink-dev release build --version 0.1.0 --out-dir ./dist
```

That script produces:

- `vreflink_<version>_linux_amd64.tar.gz`
- `vreflink_<version>_amd64.deb`
- `vreflink_<version>_sha256sums.txt`

The `.deb` is local-installable:

```bash
sudo dpkg -i ./dist/vreflink_<version>_amd64.deb
```

It installs:

- `/usr/bin/vreflink`
- `/usr/bin/vreflinkd`
- `/lib/systemd/system/vreflinkd.service`
- `/etc/vreflinkd/config.toml`
- `/usr/share/vreflink/config.toml`

The package intentionally carries both the guest CLI and the host daemon
together. That keeps the packaged templates and documented layout consistent on
both the host and the guest.

The package does not enable or start `vreflinkd` automatically. Operators edit
`/etc/vreflinkd/config.toml` to set the real `share_root`, token mappings, and
any optional `allow_v1_fallback = true` override before enabling the service.
Because that file contains bearer tokens, it should stay root-owned and mode
`0600`.

The tarball contains the same binaries plus the packaged templates so users on
other Linux distributions can copy the files into place manually.

## Validation

Run the packaging/release verification stage with:

```bash
go run ./cmd/vreflink-dev test release
```

This stage builds the artifacts, inspects the `.deb` contents and control
metadata, installs it into a temporary Debian package root, checks the
installed file layout, runs `vreflink --help` and `vreflinkd --help`, and
verifies remove/purge lifecycle behavior without enabling the service by
default.

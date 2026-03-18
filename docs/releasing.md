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
- `/etc/default/vreflinkd`

The package does not enable or start `vreflinkd` automatically. It also does
not ship `/etc/vreflinkd/tokens.yaml`; operators create that YAML token map
separately when enabling protocol v2 authentication. Legacy v1 fallback is
disabled by default unless `VREFLINK_ALLOW_V1_FALLBACK=true` is set.

The tarball contains the same binaries plus the packaged templates so users on
other Linux distributions can copy the files into place manually.

## Validation

Run the packaging/release verification stage with:

```bash
go run ./cmd/vreflink-dev test release
```

This stage builds the artifacts, inspects the `.deb` contents, installs it into
a temporary Debian package root, checks the installed file layout, runs
`vreflink --help` and `vreflinkd --help`, and verifies the service is not
enabled by default.

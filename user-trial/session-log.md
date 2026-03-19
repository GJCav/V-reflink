# Session Log

Chronological record of every action taken during the user trial.

---

## 2026-03-19 — Installation and Basic Exploration

### Step 1: Read the README

Read `README.md` in full before touching any binary. Noted:

- clear host/guest split (virtiofs data plane, vsock control plane);
- Debian package is the recommended install path;
- both `vreflink` and `vreflinkd` land on both host and guest in one package;
- service is installed but disabled by default;
- `vreflink config init` is the first-run guest command;
- `vreflinkd systemd-unit` prints the unit template, does not install.

Questions I had before installing:

1. What files does the package actually install? The README lists five.
2. Does the service auto-enable on install?
3. What does the daemon log look like on startup?

### Step 2: Inspect the artifact

```
$ dpkg-deb -c dist/vreflink_0.1.2_amd64.deb
```

Verified package contains exactly the five files the README lists:
- `./usr/bin/vreflink`
- `./usr/bin/vreflinkd`
- `./lib/systemd/system/vreflinkd.service`
- `./etc/vreflinkd/config.toml`
- `./usr/share/vreflink/config.toml`

Also verified the control metadata: `conffiles` lists `/etc/vreflinkd/config.toml`,
`prerm` and `postrm` are present and executable.

### Step 3: Install the package

```
$ sudo dpkg -i dist/vreflink_0.1.2_amd64.deb
```

Installed cleanly. Verified all five documented files are present.
Confirmed the service is installed but **not enabled** by default.

See `artifacts/deb-install.txt`, `artifacts/deb-contents.txt`,
`artifacts/postinstall-files.txt`.

### Step 4: Explore the CLI help

```
$ vreflink --help
$ vreflinkd --help
$ vreflink config --help
```

Both help outputs were clear. Flag names matched README documentation exactly.

### Step 5: Guest config initialization

```
$ vreflink config init
$ vreflink config init     # second run
$ vreflink config init --force
```

`config init` wrote the template to `$XDG_CONFIG_HOME/vreflink/config.toml`.
Second run correctly refused to overwrite. `--force` succeeded.
Template content matched the README quick-start example.

See `artifacts/config-init.txt`.

### Step 6: systemd unit template

```
$ vreflinkd systemd-unit
```

Printed the canonical unit template. Did not install or write any file.
Matched the documented behavior.

### Step 7: Daemon startup error cases

Tested error paths documented in the README's Failure Modes section.

**Missing config file:**
```
$ vreflinkd -c /nonexistent/path/config.toml
vreflinkd: read daemon config /nonexistent/path/config.toml: open ...
```
Clear, actionable.

**Nonexistent share root:**
```
$ vreflinkd -c /tmp/bad-share.toml   # share_root="/nonexistent"
vreflinkd: share root "/nonexistent/share/root" does not exist
```
Clear.

**ext4 share root (no reflink):**
```
$ vreflinkd -c /tmp/ext4-config.toml   # share_root="/tmp"
vreflinkd: share root "/tmp" does not support reflink: ...
```
Clear. No false starts.

See `artifacts/startup-errors.txt`.

### Step 8: Daemon startup with btrfs (happy path, no VM)

Created a 200 MiB loopback-mounted btrfs filesystem and pointed the daemon
at it:

```
$ dd if=/dev/zero bs=1M count=200 of=/tmp/btrfs.img
$ mkfs.btrfs /tmp/btrfs.img
$ sudo mount -o loop /tmp/btrfs.img /tmp/btrfs-mnt
$ vreflinkd -c /tmp/btrfs-config.toml
time=2026-03-19T15:18:41.994Z level=INFO msg=listening share_root=/tmp/btrfs-mnt port=29090
```

Daemon started cleanly and logged its share root and port.

### Step 9: Guest CLI path validation

Exercised the path-containment and argument validation without a live daemon.

```
$ vreflink                        # no args
vreflink: accepts 2 arg(s), received 0

$ vreflink /etc/passwd /tmp/out   # path outside mount root
vreflink: path must stay within the guest mount root

$ vreflink --mount-root /shared some/relative/src some/relative/dst
vreflink: path must stay within the guest mount root
```

Relative paths are resolved from the working directory but still must stay
within the mount root. The error is clear.

**Bad config file:**
```
$ vreflink /shared/a /shared/b   # with malformed config.toml
vreflink: load CLI config: parse .../config.toml: toml: expected character =
```
Clear and actionable.

See `artifacts/client-errors.txt`.

### Step 10: Package removal and config preservation

Simulated an admin-edited config, then ran `dpkg --remove` and `dpkg --purge`.

**Remove:** binary and unit file removed; admin-edited
`/etc/vreflinkd/config.toml` preserved. Maintainer scripts invoked
`deb-systemd-invoke stop` and `deb-systemd-helper disable` correctly.

**Purge:** config removed. `deb-systemd-helper purge` and daemon-reload
invoked correctly.

See `artifacts/remove-purge.txt`.

### Step 11: Full end-to-end (untested — no QEMU)

The following were documented as untested due to missing QEMU/virtiofsd:

- mounting virtiofs share in a guest VM;
- running `vreflink` from inside the guest over vsock;
- verifying host-side shared extents with `btrfs filesystem du` / `filefrag`;
- testing wrong token, duplicate destination, and symlink source over the
  live protocol path;
- recursive directory reflink.

---

## Summary of Steps

| Step | Outcome |
|---|---|
| Read README | Completed |
| Inspect artifact | Completed — contents match README |
| Install package | Completed |
| Verify help output | Completed |
| Guest config init | Completed |
| systemd-unit template | Completed |
| Daemon startup errors | Completed |
| Daemon startup (btrfs happy path) | Completed |
| Guest CLI path validation | Completed |
| Package remove/purge lifecycle | Completed |
| Full end-to-end via VM | **Not attempted — QEMU unavailable** |

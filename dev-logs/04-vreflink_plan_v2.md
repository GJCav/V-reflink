# vreflink / vreflinkd --- Implementation Plan (Pinned v2)

## 0. Goal

Solve the ownership bug from `dev-logs/03-feedbacks.md` in a way that matches
the actual project background:

- one VM per project is the common deployment model;
- guest users commonly have unrestricted `sudo`;
- guest-side per-user identity is therefore not a trustworthy security signal;
- the host should still create destination files and directories under a
  non-root identity selected by host policy.

Pinned v2 decision:

**Use a bearer token sent by the guest CLI. The host daemon maps that token to a
host identity `(uid, gid, supplementary groups)` from a YAML configuration file
and runs the reflink work in a subprocess under that mapped identity.**

This is intentionally a **trusted-guest** design. It does not attempt to prove
which guest user invoked the request.

------------------------------------------------------------------------

# 1. Frozen v2 Scope

### New Capability

- support authenticated token-based identity selection;
- support host-side identity mapping:
  - `uid`
  - `gid`
  - supplementary groups;
- create destination files and directories under the mapped host identity rather
  than as `root`;
- enforce filesystem permission checks through a worker subprocess running under
  that mapped identity.

### Explicitly In Scope

- request protocol bump to **version 2**;
- new guest token config and CLI flag/env handling;
- new daemon token-map config in YAML;
- hidden/internal daemon worker subprocess;
- unit tests and VM integration tests for ownership and permission behavior.

### Explicitly Out of Scope

- proving the actual guest login user identity;
- resistance against guest-root compromise;
- cryptographic request signing;
- rotating or minting tokens dynamically;
- multi-tenant zero-trust guest security;
- ACL-specific policy management beyond normal kernel permission checks.

------------------------------------------------------------------------

# 2. Pinned Trust Model

## Security Statement

`vreflinkd` trusts possession of a configured bearer token.

That token authorizes the guest to request reflink operations as one mapped host
identity.

### Consequences

- a guest with the token can act as the mapped host identity;
- guest root can read or reuse the token;
- spoofing by guest users is **not** prevented once the token is available in
  the guest;
- this is acceptable for the intended "one VM per project" workflow where the
  VM itself is the trust boundary.

### Recommended Operational Model

- one token per guest VM or per project;
- one mapped host service identity per token;
- do **not** issue separate tokens per human user inside the same guest unless
  the guest trust model changes.

------------------------------------------------------------------------

# 3. User-Facing Contract

## CLI

Pinned v2 CLI shape:

```bash
vreflink [--token TOKEN] SRC DST
vreflink -r [--token TOKEN] SRC DST
```

Guest config/env adds:

- `VREFLINK_AUTH_TOKEN`

Precedence becomes:

```text
flags > environment > $XDG_CONFIG_HOME/vreflink/env > defaults
```

Where the token has no default and must be supplied explicitly through flags,
environment, or guest config.

## Daemon

Daemon config/env adds:

- `VREFLINK_TOKEN_MAP_PATH`

Default:

- `/etc/vreflinkd/tokens.yaml`

Pinned daemon behavior:

- if the token-map file exists and is configured, `vreflinkd` requires protocol
  v2 requests with a non-empty token;
- requests with missing, unknown, or malformed tokens fail;
- the daemon never trusts guest-supplied `uid`, `gid`, or group lists because
  those are **not** sent at all in v2.

------------------------------------------------------------------------

# 4. Protocol Changes

## Request

Pinned v2 request shape:

```json
{
  "version": 2,
  "op": "reflink",
  "recursive": false,
  "src": "relative/path",
  "dst": "relative/path",
  "token": "bearer-secret"
}
```

### Rules

- `token` is required for protocol v2;
- `uid`, `gid`, and `groups` are **not** request fields;
- the token must not be included in logs.

## Response

Response shape remains unchanged from v1:

```json
{
  "ok": true
}
```

or

```json
{
  "ok": false,
  "error": {
    "code": "EPERM",
    "message": "invalid authentication token"
  }
}
```

------------------------------------------------------------------------

# 5. Token Map YAML

Pinned config format:

```yaml
version: 1
tokens:
  - name: project-a
    token: "vreflink-9e4c2d7b7f3b4d97..."
    uid: 1001
    gid: 1001
    groups: [44, 1001]

  - name: project-b
    token: "vreflink-1d8bcb0c1cf9486e..."
    uid: 1002
    gid: 1002
    groups: [1002, 1003]
```

## Semantics

- `token`: bearer secret, unique across entries;
- `uid`: mapped host user ID;
- `gid`: mapped host primary group ID;
- `groups`: supplementary groups used when starting the worker subprocess.

### Notes on `groups`

- `groups` are **not** mainly about the final file's group ownership;
- they are needed so kernel permission checks match the mapped account when the
  request depends on supplementary group membership;
- example: destination parent directory is writable only through a shared group;
- Linux group inheritance rules such as SGID directories still apply normally,
  so the final file group may follow filesystem semantics rather than always
  equaling the configured `gid`.

## Validation Rules

- YAML must parse successfully at startup;
- `version` must be recognized;
- token strings must be non-empty and unique;
- `uid` and `gid` must be valid unsigned integers;
- `groups` entries must be valid unsigned integers;
- duplicate group IDs are removed during normalization;
- startup fails if the token-map file is malformed.

## File Permissions

Operational recommendation:

- owner: `root`
- mode: `0600`

The daemon should not log token values on success or error.

------------------------------------------------------------------------

# 6. Host Execution Model

Pinned implementation model:

1. `vreflink` builds a v2 request and includes the token.
2. `vreflinkd` validates the request and looks up the token in the YAML map.
3. `vreflinkd` resolves the mapped identity `(uid, gid, supplementary groups)`.
4. `vreflinkd` starts a short-lived worker subprocess under those credentials.
5. The worker performs path validation, destination creation, and reflink work.
6. The worker returns status to the daemon.
7. The daemon converts the worker result into the normal response format.

## Worker Form

Pinned choice:

- use the same installed daemon binary with a hidden/internal subcommand, for
  example `vreflinkd worker`;
- the public deployment artifact set remains unchanged;
- packaging does not need a second helper binary.

## Credential Handling

The worker subprocess is started with:

- mapped `uid`
- mapped `gid`
- mapped supplementary groups

using OS credential dropping for the child process only.

Pinned rule:

- do **not** switch credentials inside the long-lived daemon process;
- all filesystem operations that determine ownership and permissions must happen
  inside the worker process under the mapped credentials.

------------------------------------------------------------------------

# 7. Permission and Ownership Semantics

Pinned v2 semantics:

- the host token map selects the identity of the operation;
- source traversal, destination parent access, file creation, directory
  creation, and reflink execution all happen as the mapped identity;
- destination ownership is whatever the kernel produces for that identity and
  target directory semantics;
- root-owned destination files from daemon-created paths are no longer expected.

### Important Behavior

- if the mapped identity lacks permission to create the destination, the
  request must fail;
- if recursive mode creates directories, those directories must also be created
  under the mapped identity;
- supplementary groups affect authorization but are not exposed in the request.

------------------------------------------------------------------------

# 8. Error Model

Pinned new errors:

- missing token in v2 request → `EINVAL`
- token-map required but request uses v1 → `EINVAL`
- unknown token → `EPERM`
- malformed token map at daemon startup → startup error
- mapped identity lacks destination write/execute permission → `EPERM`

Existing reflink/path errors remain unchanged where possible.

Suggested user-facing message for an unknown token:

```text
invalid authentication token
```

------------------------------------------------------------------------

# 9. Compatibility and Migration

## Protocol

Pinned compatibility rule:

- daemon may continue to support v1 only when no token map is configured;
- once token mapping is configured, authenticated requests use protocol v2.

## Guest Migration

Guest side adds:

- `--token`
- `VREFLINK_AUTH_TOKEN`
- optional token entry in `~/.config/vreflink/env`

## Host Migration

Host side adds:

- YAML token-map file
- `VREFLINK_TOKEN_MAP_PATH`

## Packaging

Packaging updates for v2 should include:

- daemon defaults template with `VREFLINK_TOKEN_MAP_PATH`
- documentation for the YAML file path and permissions
- no token values shipped in package defaults

------------------------------------------------------------------------

# 10. Testing Plan

## Unit Tests

- protocol v2 request validation requires token;
- token-map YAML parsing and normalization;
- duplicate token rejection;
- duplicate group normalization;
- daemon rejects unknown token;
- daemon converts token to mapped credentials;
- worker runs under configured `uid`, `gid`, and supplementary groups;
- destination ownership is not root after successful worker execution.

## Service / Integration Tests

- mapped identity creates single-file destination with expected owner;
- recursive mode creates directories and files with expected owner;
- request fails when mapped identity cannot write destination parent;
- request succeeds when access depends on a supplementary group;
- request fails when token is missing or unknown.

## VM Tests

Pinned VM coverage additions:

- guest request with configured token succeeds and resulting host file owner is
  the mapped `uid/gid`;
- guest request fails when token is absent or invalid;
- guest request succeeds when the mapped identity relies on a supplementary
  group for destination-parent access;
- daemon startup still validates share-root reflink capability independently of
  token-map loading.

------------------------------------------------------------------------

# 11. Security Summary

Pinned v2 security posture:

- stronger than always running as host root;
- much simpler than guest-side user attestation;
- appropriate for trusted-guest project VMs;
- not suitable for hostile or multi-tenant guests.

This version intentionally answers:

> "Which host identity should this guest VM use for reflink operations?"

It does **not** answer:

> "Which human guest user really invoked the command?"

------------------------------------------------------------------------

# 12. Final Decision

Pinned v2 design:

- protocol v2 adds a bearer `token`;
- host YAML maps `token -> uid, gid, supplementary groups`;
- guest sends only the token, never raw identity claims;
- daemon chooses credentials from host policy;
- daemon executes reflink work in a worker subprocess under those credentials;
- trust boundary is the guest VM, not the individual guest user.

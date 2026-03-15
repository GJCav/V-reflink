# vreflink / vreflinkd --- Implementation Plan (Pinned v1)

## 0. Goal

Build a **guest CLI + host daemon** pair so a guest using **virtiofs**
can request a **true host-side btrfs reflink** on files inside the
shared tree.

The host/guest control channel is **AF_VSOCK with stream sockets**, and
the implementation is in **Go** using existing libraries where that
materially reduces low-level work.

Architecture summary:

-   Data plane: virtiofs shared directory
-   Control plane: vsock stream RPC
-   Execution: host daemon performs reflink on host filesystem

------------------------------------------------------------------------

# 1. Frozen v1 Scope

### Functionality

-   One operation only: **reflink**
-   Two forms:
    -   single file
    -   recursive tree (`-r`)
-   Destination must **not exist**
-   One client: **CLI**
-   One daemon transport: **vsock stream**
-   Language: **Go**

### Recursive Behavior

-   Directories → create
-   Regular files → reflink
-   **Fail fast**
-   **Non-transactional**
-   Partial destination tree may remain

### Explicit Rejections

-   Symlinks
-   Hard links
-   Device nodes
-   FIFOs
-   Sockets

### Explicit Non-features

-   No copy API
-   No reflink-or-copy
-   No overwrite modes
-   No library API
-   No metadata preservation
-   No rollback
-   No batching
-   No qemu-ga integration

------------------------------------------------------------------------

# 2. User-Facing Contract

## CLI

``` bash
vreflink SRC DST
vreflink -r SRC DST
```

### Semantics

-   Both paths must be under virtiofs mount
-   Destination must not exist
-   Success = real reflink executed on host
-   No fallback copy
-   Recursive mode stops on first error
-   Partial destination tree may remain

------------------------------------------------------------------------

# 3. Names

  Component   Name
----------- -------------
  CLI         `vreflink`
  Daemon      `vreflinkd`

------------------------------------------------------------------------

# 4. Architecture

## Data Plane

    host filesystem (btrfs)
          ↓
    virtiofs export
          ↓
    guest mount (/shared)

## Control Plane

    guest CLI
         ↓ vsock
    host daemon

Operation flow:

1.  Guest CLI builds request
2.  Request sent via vsock
3.  Host daemon validates paths
4.  Host daemon executes reflink
5.  Response returned

------------------------------------------------------------------------

# 5. Dependencies

Pinned dependencies:

  Package                                   Purpose
----------------------------------------- ----------------------
  `github.com/mdlayher/vsock`               vsock transport
  `github.com/cyphar/filepath-securejoin`   safe path resolution
  `golang.org/x/sys/unix`                   linux syscalls
  `github.com/KarpelesLab/reflink`          reflink execution
  `github.com/spf13/cobra`                  CLI framework

Logging uses Go standard library.

------------------------------------------------------------------------

# 6. Repository Layout

    cmd/
      vreflink/
      vreflinkd/
    
    internal/
      protocol/
      framing/
      client/
      server/
      service/
      tree/
      validate/
      config/
      logutil/

Responsibilities:

  Package    Role
---------- --------------------------
  protocol   request/response structs
  framing    length‑prefixed JSON
  client     guest RPC
  server     daemon listener
  service    reflink orchestration
  tree       recursive logic
  validate   path validation
  config     configuration
  logutil    logging helpers

------------------------------------------------------------------------

# 7. Transport Design

Transport: **AF_VSOCK SOCK_STREAM**

Rules:

-   One request per connection
-   One response per connection
-   Close connection after response

Concurrency:

    accept()
       ↓
    goroutine per connection

------------------------------------------------------------------------

# 8. Framing & Protocol

## Framing

    [4‑byte length][JSON payload]

## Request

``` json
{
  "version": 1,
  "op": "reflink",
  "recursive": false,
  "src": "relative/path",
  "dst": "relative/path"
}
```

## Success Response

``` json
{
  "ok": true
}
```

## Failure Response

``` json
{
  "ok": false,
  "error": {
    "code": "EEXIST",
    "message": "destination already exists"
  }
}
```

------------------------------------------------------------------------

# 9. Path Model

Guest input:

    /shared/data/file.bin

Protocol path:

    data/file.bin

Daemon root:

    /srv/labshare

Rule:

**Daemon only handles paths relative to root.**

------------------------------------------------------------------------

# 10. Path Safety

Use `filepath-securejoin`.

Security rules:

-   root‑anchored resolution
-   reject symlinks
-   reject path escape (`..`)
-   reject absolute paths

Goal:

Prevent accidental or malicious escape outside shared root.

------------------------------------------------------------------------

# 11. File-Type Policy

## Single File

Allowed:

-   regular file (`nlink == 1`)

Rejected:

-   symlink
-   hardlink
-   directory
-   special files

## Recursive

Allowed:

-   directories
-   regular files

Rejected:

-   symlink
-   hardlink
-   device/FIFO/socket

------------------------------------------------------------------------

# 12. Single File Operation

Algorithm:

1.  Validate request
2.  Resolve source under root
3.  Check source regular file
4.  Reject hardlink (`nlink > 1`)
5.  Resolve destination parent
6.  Verify destination not exist
7.  Execute reflink
8.  Return result

------------------------------------------------------------------------

# 13. Recursive Operation

Algorithm:

1.  Resolve source root
2.  Ensure directory
3.  Ensure destination not exist
4.  Create destination root
5.  Walk source tree
6.  For each entry:

```{=html}
<!-- -->
```
    dir   → mkdir
    file  → reflink
    other → error

7.  Stop immediately on failure

------------------------------------------------------------------------

# 14. Overwrite Policy

Pinned rule:

    destination exists → FAIL

No replace, rename, or delete behavior.

------------------------------------------------------------------------

# 15. Error Model

Typical codes:

  Code         Meaning
------------ -----------------------
  ENOENT       source missing
  EEXIST       destination exists
  EOPNOTSUPP   reflink unsupported
  EINVAL       unsupported file type
  EPERM        forbidden path

Example CLI output:

    vreflink: destination already exists

------------------------------------------------------------------------

# 16. CLI Responsibilities

CLI performs:

1.  parse arguments
2.  validate mount root
3.  convert to relative paths
4.  open vsock connection
5.  send request
6.  display result

No filesystem logic happens locally.

------------------------------------------------------------------------

# 17. Daemon Responsibilities

Daemon performs:

1.  load config
2.  listen on vsock
3.  accept connection
4.  read request
5.  validate
6.  run operation
7.  send response
8.  close connection

Daemon is **not** a shell or file server.

------------------------------------------------------------------------

# 18. Logging

Log fields:

-   timestamp
-   peer CID
-   operation
-   source
-   destination
-   result code

Example:

    CID=4 reflink data/A.bin → runs/B.bin OK

------------------------------------------------------------------------

# 19. Configuration

## Daemon

  Field        Example
------------ ---------------
  share_root   /srv/labshare
  vsock_port   19090

## CLI

  Field              Example
------------------ ---------
  guest_mount_root   /shared
  host_cid           2
  vsock_port         19090

------------------------------------------------------------------------

# 20. Implementation Order



This is the pinned build order.

Phase 1: repo scaffolding

1. Create repo
2. Add Cobra-based `vreflink`
3. Add plain `vreflinkd`
4. Add dependency pins

Phase 2: transport

1. Implement vsock client with `mdlayher/vsock`
2. Implement vsock server with `mdlayher/vsock`
3. Prove echo round-trip between guest and host

Phase 3: protocol

1. Implement request/response structs
2. Implement length-prefixed framing
3. Add basic validation

Phase 4: path and validation

1. Implement guest-path-to-relative-path conversion in CLI
2. Implement safe-root resolution in daemon with `filepath-securejoin`
3. Implement source/destination validation helpers
4. Implement symlink rejection
5. Implement hard-link rejection

Phase 5: core operation

1. Implement single-file reflink using `KarpelesLab/reflink`
2. Return precise errors

Phase 6: recursive mode

1. Implement source-tree walk
2. Create destination directories
3. Reflink regular files
4. Fail fast on first bad entry

Phase 7: logging and polish

1. Add structured stdlib logging
2. Improve CLI error messages
3. Add basic usage/help text

Phase 8: testing

1. Local unit tests
2. Local btrfs integration tests
3. VM integration tests over virtiofs + vsock
4. Concurrency/race tests

Phase 9: deployment

1. Install daemon on host
2. Add daemon startup method
3. Install CLI in guests
4. Document usage and failure modes



------------------------------------------------------------------------

# 21. Test Plan

## Unit Tests

-   framing encode/decode
-   request validation
-   path conversion
-   symlink detection
-   hardlink detection

## Local Integration

-   reflink success
-   destination exists
-   symlink rejection
-   hardlink rejection
-   recursive success

## VM Integration

-   guest CLI → host daemon
-   verify reflink works through virtiofs

## Concurrency

Two guests:

    guest1 → reflink A→X
    guest2 → reflink B→X

Expected:

    one success
    one EEXIST

------------------------------------------------------------------------

# 22. Deployment Assumptions

Environment:

-   host filesystem: **btrfs**
-   shared via **virtiofs**
-   guest mount: `/shared`
-   vsock device enabled
-   host CID = 2

------------------------------------------------------------------------

# 23. Deferred Future Work

Potential future features:

-   overwrite modes
-   symlink support
-   hardlink preservation
-   dry‑run
-   metadata preservation
-   batching
-   direct ioctl implementation
-   authentication policy

Not part of v1.

------------------------------------------------------------------------

# 24. Success Criteria

The project succeeds when:

1.  Guest runs

    vreflink /shared/A /shared/B

and gets real reflink.

2.  Recursive mode works:

    vreflink -r /shared/dirA /shared/dirB

3.  Destination exists → failure.

4.  Symlink/hardlink → rejection.

5.  Multiple guests work concurrently.

6.  Code is understandable by general Go developers.

------------------------------------------------------------------------

# 25. Final Dependency List

Pinned dependencies:

    github.com/mdlayher/vsock
    github.com/cyphar/filepath-securejoin
    golang.org/x/sys/unix
    github.com/KarpelesLab/reflink
    github.com/spf13/cobra

No additional dependencies unless required.

------------------------------------------------------------------------

# 26. Final Summary

-   Go implementation
-   vsock control plane
-   virtiofs shared storage
-   one operation: reflink
-   recursive support
-   strict fail-if-exists policy
-   fail-fast tree behavior
-   symlink and hardlink rejected
-   minimal CLI surface
-   small daemon

This specification defines **vreflink v1**.

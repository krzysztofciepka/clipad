# Self-upgrade (`clipad --upgrade`) — design

## Goal

Let a user update their installed `clipad` binary in place by running `clipad --upgrade`. The command fetches the most recent release from GitHub, verifies it, and atomically replaces the running binary. A companion `clipad --version` prints the embedded version and exits.

## Scope

**In scope**
- New CLI flags `--upgrade` and `--version`, parsed before the TUI starts.
- Compile-time version embedding via `-ldflags "-X main.version=<tag>"`.
- Download of the GitHub release asset matching the running platform.
- SHA-256 verification against the asset's `digest` field returned by the GitHub API.
- Atomic in-place replacement with a `.old` rollback safety net.

**Out of scope**
- Multi-platform releases. Today the project ships only `clipad-vX.Y.Z-linux-amd64`. On any other `GOOS`/`GOARCH`, `--upgrade` exits with an unsupported-platform error. Adding more platforms is a separate change to the release pipeline.
- Auto-restart of the running TUI. `--upgrade` is a one-shot subcommand; the user re-launches `clipad` afterwards.
- Channel selection (stable/beta/etc.). Only the latest release is considered.
- Retry loops. A single attempt per HTTP call; the user re-runs the command on transient failure.
- `go install`-based upgrade paths or package-manager integration.

## User-facing behavior

### `clipad --version`

```
clipad v0.0.21
```

Exits 0. The version string is whatever was baked in at build time. For `go build .` without `-ldflags`, it stays at the default `dev`.

### `clipad --upgrade`

Successful run on stderr:

```
Current version: v0.0.21
Latest version:  v0.0.22
Downloading clipad-v0.0.22-linux-amd64 (16.4 MB)...
Verifying checksum... ok
Installing to /home/kc/.local/bin/clipad... ok
Upgraded v0.0.21 → v0.0.22. Restart clipad to use the new version.
```

Special cases:

| Case | Output | Exit |
|------|--------|------|
| Already on latest tag | `clipad is up to date (v0.0.21).` | 0 |
| `runtime.GOOS`/`GOARCH` != `linux/amd64` | `self-upgrade is not supported on <GOOS>/<GOARCH> — please reinstall manually` | 1 |
| Permission denied on install dir | `cannot write to <dir>: <err> — re-run with sudo or move clipad to a user-owned path` | 1 |
| Network / API / checksum failure | Specific message (see Error handling) | 1 |

The current version is detected from the compiled-in `version` constant, defaulting to `dev` for unversioned local builds. When `version == "dev"`, the latest-comparison shortcut is skipped (a `dev` build always proceeds to download).

## Architecture

A new file `upgrade.go` holds the entire feature. `main.go` gets a small flag-parsing prologue that dispatches `--version` / `--upgrade` before the TUI is constructed.

```
main.go
  ├── var version = "dev"               ← overridden via -ldflags at release time
  └── main()
        ├── parse flags (stdlib `flag`)
        ├── if --version → print, exit 0
        ├── if --upgrade → runUpgrade(); exit (0 or 1)
        └── …existing TUI bootstrap…

upgrade.go
  ├── const repoOwner = "krzysztofciepka"
  ├── const repoName  = "clipad"
  ├── runUpgrade(out io.Writer) error            // top-level orchestrator
  ├── fetchLatestRelease(ctx) (release, error)
  ├── pickAsset(release, goos, goarch) (asset, error)
  ├── downloadAsset(ctx, asset, dst, expectedDigest) error
  └── installBinary(srcPath, targetPath) error

upgrade_test.go
  └── unit tests using httptest.Server stubs
```

**Why one file:** the whole feature is roughly 250 lines and one cohesive flow. Splitting it adds navigation cost without buying clarity. A future change that adds platforms or auto-restart can split it then.

**Dependencies:** stdlib only — `net/http`, `encoding/json`, `crypto/sha256`, `encoding/hex`, `io`, `os`, `path/filepath`, `runtime`, `flag`, `context`, `time`. No new modules in `go.mod`.

**Interaction with the TUI:** none. `runUpgrade` returns from `main()` before any Bubble Tea program is constructed.

## Data flow

```
runUpgrade(out)
  │
  ├─ 1. Platform gate
  │     if runtime.GOOS != "linux" || runtime.GOARCH != "amd64": fail fast
  │
  ├─ 2. Resolve install path
  │     exe, _ := os.Executable()
  │     target, _ := filepath.EvalSymlinks(exe)   // follow symlinks; replace the real file
  │     dir   := filepath.Dir(target)
  │
  ├─ 3. fetchLatestRelease(ctx)
  │     GET https://api.github.com/repos/krzysztofciepka/clipad/releases/latest
  │       Headers: Accept: application/vnd.github+json
  │                User-Agent: clipad-upgrader/<version>
  │       Timeout: 30s (context.WithTimeout)
  │     Decode minimal struct:
  │       { tag_name, assets: [{name, browser_download_url, size, digest}] }
  │
  ├─ 4. Compare versions
  │     latest := release.TagName
  │     if version != "dev" && latest == version:
  │         print "up to date"; return nil
  │     // String equality is sufficient: tags are produced by us, always vX.Y.Z form;
  │     // we never need ordering, only equality.
  │
  ├─ 5. pickAsset(release, "linux", "amd64")
  │     want := "clipad-" + latest + "-linux-amd64"
  │     find asset with matching Name; error if missing
  │
  ├─ 6. downloadAsset(ctx, asset, tmp, asset.Digest)
  │     tmp := filepath.Join(dir, ".clipad-upgrade-<pid>")  // same dir → atomic rename later
  │     defer cleanup(tmp) on every failure path
  │     http.Get(asset.BrowserDownloadURL)
  │     stream body → io.MultiWriter(tmpFile, sha256Hasher)
  │     got := hex.EncodeToString(hasher.Sum(nil))
  │     want := strings.TrimPrefix(asset.Digest, "sha256:")
  │     if got != want: fail
  │     chmod tmp 0o755
  │
  └─ 7. installBinary(tmp, target)
        backup := target + ".old"
        os.Rename(target, backup)        // move running binary aside
        if err := os.Rename(tmp, target); err != nil {
            os.Rename(backup, target)    // best-effort rollback
            return err
        }
        os.Remove(backup)                // ignore error; .old is cosmetic at this point
```

### Same-filesystem invariant

The temp file is created in `dir` (not `os.TempDir()`), so both renames in step 7 are guaranteed atomic on Linux. Using `/tmp` would risk an `EXDEV` cross-device error on systems where `/tmp` is `tmpfs`.

### Symlinks

`os.Executable()` may return a symlink (e.g. when packaged into `/usr/local/bin/clipad` linking to a versioned path, or installed via `go install` with shims). `filepath.EvalSymlinks` resolves it to the real file. The temp file is placed beside that real file, and the swap replaces the real file. Symlinks pointing at the binary continue to work because they were never touched.

### Why atomic + rollback

- `os.Rename` on the same filesystem is a single inode swap — there is no in-between state where the binary is missing.
- The `.old` backup gives us a recoverable previous state if the second rename fails for any reason. In practice it only fails under exotic conditions (e.g., the target's parent dir was concurrently changed), but the cost of the safety net is one extra `rename(2)`.

### Single attempt, no retries

`--upgrade` is invoked manually. If the network flakes, the user can re-run the command. A retry loop adds code without meaningful UX gain.

## Error handling

Every error from `runUpgrade()` is printed to stderr and exits 1. Messages are written to be specific enough that the user knows the next step.

| Failure point | Message | Recovery |
|---|---|---|
| Wrong platform | `self-upgrade is not supported on <GOOS>/<GOARCH> — please reinstall manually` | Manual reinstall |
| `os.Executable()` fails | `cannot determine clipad binary path: <err>` | Unusual environment |
| GitHub API non-200 | `failed to fetch latest release: <status>: <body-snippet>` | Network or rate-limit |
| Release JSON decode fails | `failed to parse release metadata: <err>` | Upstream API change |
| Already on latest | `clipad is up to date (<version>).` (exit 0) | — |
| No matching asset | `no asset matching clipad-<tag>-linux-amd64 in release <tag>` | Publish issue |
| Download HTTP non-200 | `failed to download <url>: <status>` | Retry |
| I/O during download | `download interrupted: <err>` | Retry |
| Checksum mismatch | `checksum mismatch: expected <hex>, got <hex>` | Refuse to install |
| Cannot create temp file in `dir` | `cannot write to <dir>: <err> — re-run with sudo or move clipad to a user-owned path` | Permissions hint |
| `chmod` fails on temp | `cannot make new binary executable: <err>` | Tmp removed |
| Backup rename fails | `cannot move existing binary aside: <err>` | Original untouched |
| Final rename fails after backup | Restore `.old`, then `failed to install new binary: <err>` (rollback note included) | Original restored |
| Restore-after-failure also fails | `failed to install new binary: <err>; original saved at <target>.old — restore manually` | Manual recovery |

**Cleanup discipline**

- `defer cleanup(tmp)` runs the moment the temp file is created. After a successful rename, `cleanup` is a no-op (the file no longer exists at that path).
- The `.old` backup is removed only after the final rename succeeds. If the final rename and the rollback both fail, `.old` is left in place and the user is told where it is.

**No partial-state windows**

At every point in the install step, either the original binary is at `target` (possibly via rollback) or the new binary is at `target`. There is no window where `target` is missing.

## Build / release process changes

This adds one small change to how releases are built: the `version` variable must be set via `-ldflags` so the binary knows its own tag.

```bash
go build -ldflags "-X main.version=v0.0.22" -o clipad-v0.0.22-linux-amd64 .
```

This replaces (or wraps) whatever current build invocation produces the release asset. The implementation plan will codify the exact command — likely a small `Makefile` target or a documented one-liner — but no broader release-pipeline overhaul is required.

For local development (`go build .` without ldflags) the version stays at `dev`, and `--upgrade` skips the equality short-circuit so a developer build always proceeds to download.

## Testing

Hermetic unit tests using `httptest.Server` for the GitHub API and asset host.

**`TestPickAsset`** (table-driven)

- Exact match for `clipad-vX.Y.Z-linux-amd64`.
- No matching asset → descriptive error.
- Multiple assets present → picks the right one.

**`TestDownloadAsset`** — `httptest.NewServer` serving a known byte payload

- Success: written file matches payload bytes; computed sha256 matches expected digest.
- 404 from server → error wraps status.
- Digest mismatch → error contains both hashes; temp file is cleaned up.
- Short read (server hangs up mid-stream) → error returned; temp file cleaned up.

**`TestInstallBinary`** — operates inside `t.TempDir()`

- Happy path: `target` content equals `src` content; no `.old` left behind; mode is `0755`.
- Target is a symlink → the resolved file is replaced, not the symlink itself.
- Target directory is read-only → returns permission error; no orphan files in the directory.
- Final rename fails (simulated by making `target` into a directory after backup, since `rename(file, dir)` fails) → backup is restored; reading `target` returns the original bytes.

**`TestFetchLatestRelease`** — `httptest.Server` returning canned JSON

- Success: returns parsed release with tag and asset list.
- Non-200 status: error includes status code and body snippet.
- Malformed JSON: error mentions parse failure.

**`TestRunUpgrade_AlreadyLatest`**

- API stub returns a release whose tag equals the compiled-in `version`.
- Asserts no download requests were made and `runUpgrade` returns nil.

**Out of scope for tests:** invoking the actual upgraded binary. Byte-for-byte equality with the downloaded payload is sufficient; running it would test the Go runtime, not our code.

**Manual verification before release**

1. Build with `-ldflags "-X main.version=v0.0.20"`. Run `./clipad --upgrade` against the live repo. Confirm it pulls v0.0.21, swaps the binary, and `./clipad --version` then prints `v0.0.21`.
2. Run `./clipad --upgrade` again. Confirm the "up to date" path.

## Open questions

None at design time. Implementation may surface details (e.g. the exact form of progress output during download) which the plan will pin down.

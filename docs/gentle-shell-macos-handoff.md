# Gentle Shell — macOS handoff

## Branch and scope

- Branch: `feat/shell-linux-installation-experience-01a148a7`
- Head at handoff: `c48dd3c84b55` (includes latest Pi/Gentle Shell resolution, GGA removal and update-channel preference)
- Worktree used for verification: `/home/devel/.local/state/gentle-ai-wsl-pr5596-01a148a7-v1/worktree-main-defaults-r1`
- Goal: make the in-TUI Gentle Shell installer behave on macOS like it does on Linux/Windows.

This is the same product flow: an already-installed `gentle-ai` TUI opens **Install → Separate**, installs/updates Gentle Shell and publishes `gentle-shell` for the user.

macOS is intentionally **Apple Silicon only** for this feature. `userDarwinKernelCheck` refuses root, Intel, Rosetta and macOS < 14. Do not silently broaden that guard without a separate product decision.

## Current implementation map

### Shared TUI and flow

These are platform-independent and already contain the Linux/Windows behavior:

- `internal/tui/shell_*.go`, `internal/tui/screens/shell_*.go` — Welcome, Configure, Plan, 5-step install, Manage, update and uninstall UI.
- `internal/shellinstaller/user_install_flow_unix.go` — install orchestration shared by Linux and Darwin.
- `internal/shellinstaller/user_install_unix.go`, `user_install_stage_unix.go`, `user_experience_stage_unix.go` — stage, preferences, native/runtime publication.
- `internal/shellinstaller/user_shell_*.go` — launcher, registry, discovery, Manage apply, channel preference, update check, uninstall.
- `internal/shellinstaller/user_launch_unix.go` — process-group launch primitives.
- `internal/shellinstaller/user_experience.go` — experience validation and the persisted update channel.
- GGA has been removed from the shell installer; general Gentle AI GGA/preset code is intentionally untouched.

### Darwin-specific files

- `user_install_darwin.go` — kernel gate, entry points, supervised commands.
- `user_guards_darwin.go` — native arm64/macOS 14 gate, APFS/path guards, xattrs/quarantine rules.
- `user_supervise_darwin.go` — process supervision and group reaping.
- `user_launch_darwin.go` — Darwin process-group/kill semantics.
- `user_os_darwin.go` — syscall seams (`renamex_np`, termios, trusted `/private/tmp`).
- `user_stage_publish_darwin.go` — no-replace publication and directory sync.
- `user_pins_darwin.go` — Node/fd/rg and Gentle AI arm64 pins; native bundle currently **4.2.0** from `gentle-pi@4.3.0`.
- `user_shared_darwin_test.go`, `user_install_darwin_test.go`, etc. — Darwin-only coverage.

The shared Unix flow already uses these files; there is no second macOS installer.

## Verified state

- `GOOS=darwin GOARCH=arm64 go build ./...` passes on Linux.
- `GOOS=darwin GOARCH=amd64 go build ./...` is not a supported product target; the kernel guard rejects it at runtime.
- Linux TUI/screens/shellinstaller tests pass at this head (some environment-sensitive tests need `umask 022`).
- Native macOS 26 arm64 acceptance of the main TUI passed (non-root, isolated HOME): Separate on Stable with Gentleman, Background/Strict TDD/Global RDD/Context7 ON and CodeGraph OFF; canonical preference files, `~/.local/bin/gentle-shell`, `gentle-shell --version`; Manage → Apply preferences (Neutral, Strict TDD OFF, Context7 OFF) with schema'd bytes and the Context7 `url`/`exposure` kept; uninstall with backup and launcher removal.
- `go test ./internal/shellinstaller/... ./internal/tui/...` passes natively on macOS; the "Gentle Shell macOS qualification" lane runs both packages.
- `Darwin Runtime` CI exists (`macos-latest`) but runs only `scripts/darwin-release-blockers.manifest`; **it currently contains no shellinstaller test entries**.
- Still refused on macOS: CodeGraph ON (no darwin archives/staging) and Main preview (no darwin native runtime or channel overlay).

## Priority tasks

1. **Done: fix the Darwin test-build gap.** `GOOS=darwin go vet ./internal/shellinstaller` currently fails because `writeInstallFixture` is defined in `user_shell_reconfigure_unix_test.go` under `//go:build linux && amd64`, while `user_shell_apply_unix_test.go` is `linux || darwin`. Move the helper to a shared `linux || darwin` test file or add a Darwin equivalent. Then run the shared shell installer tests natively on macOS.

2. **Add shell-installer coverage to Darwin CI.** Add the relevant native tests to `scripts/darwin-release-blockers.manifest` (or a dedicated macOS shell job), including at minimum:
   - install inspection/selection and no-effect validation,
   - Separate install publication and launcher,
   - Manage discovery/apply/channel preference,
   - Stable ↔ Main update-check routing,
   - uninstall + minimal backup,
   - launch from the installed target.
   The manifest runner's drift guard is deliberate; add real test names, not broad regexes.

3. **Partly done: run a real macOS acceptance pass.** Install, Manage apply and uninstall passed (see Verified state); Stable ↔ Main waits for the darwin Main runtime, and reinstall/cache lifecycle is not yet re-run. On macOS 14+ Apple Silicon, as a non-root user:
   - Build the TUI from this branch: `go build -o /tmp/gentle-ai ./cmd/gentle-ai`.
   - Run it, choose **Install → Separate**, confirm all five steps and the completion screen.
   - Verify `~/.local/bin/gentle-shell` and the installed target.
   - Run `gentle-shell`; verify it launches Pi/Gentle Shell.
   - Open Manage: apply a preference, switch Stable → Main, confirm the Update action checks Main; switch back and confirm Stable.
   - Uninstall and verify the minimal backup and launcher removal.
   - Reinstall and verify the download/cache lifecycle.

4. **Decide macOS path conventions.** The shared code currently uses:
   - install target: `~/.local/share/gentle-shell/install`
   - launcher: `~/.local/bin/gentle-shell`
   - registry/home state under `~/.local/share` / `~/.gentle-ai`
   This mirrors Linux, not `~/Library/Application Support` / `~/Library/Caches`. Decide whether product consistency or native macOS convention wins; if changed, update `shellDefaultDestination`, launcher helpers, registry/discovery, uninstall backup naming and user-facing docs together.

5. **Port the remaining performance work only where it applies.**
   - Windows added a verified per-user download cache and system-Go reuse. macOS native install uses signed/cached assets and does **not** source-build with Go, so system-Go reuse is not applicable. A Darwin cache equivalent would be `~/Library/Caches/gentle-shell` and must be owned/removed on uninstall.
   - Windows added a fast launch boundary. macOS still uses the shared full launch/readback path; measure `gentle-shell` startup on a real Mac before deciding whether the same launch-only trust boundary is worth porting.

6. **Gatekeeper / quarantine.** The installer deliberately rejects `com.apple.quarantine` on installer-owned entries while tolerating it on the operator's downloaded `gentle-ai`. Test a browser-downloaded/quarantined build and a locally built source binary. Do not weaken these xattr rules without a dedicated security review. There is no Authenticode-style signature solution here; macOS distribution still needs Gatekeeper/notarization policy.

7. **Keep native pins current.** `user_pins_darwin.go` currently pins Gentle AI native `4.2.0` (the bundle published by `gentle-pi 4.3.0`), plus Node 24.18.0, fd 10.5.0 and ripgrep 15.2.0. When `gentle-pi` moves, verify the arm64 asset SHA and native bundle directory dynamically; do not assume the native version equals the `gentle-pi` version.

## Validation commands

On macOS 14+ arm64:

```bash
go build ./...
go test ./internal/tui/... ./internal/cli ./internal/shellinstaller
./scripts/darwin-release-blockers.sh verify
./scripts/darwin-release-blockers.sh run
```

On any host, compile-only:

```bash
GOOS=darwin GOARCH=arm64 go build ./...
GOOS=darwin GOARCH=arm64 go vet ./internal/shellinstaller ./internal/tui ./internal/cli
```

Use a disposable macOS user/VM for install/uninstall tests; the installer moves and prunes real user-level trees.

## Non-goals / do not port

- Windows Job Objects, WindowsApps `.cmd` shim, `LOCALAPPDATA` cache, system-Go reuse.
- Windows Authenticode/Defender handling.
- The Windows `provision-gentle-shell-windows.mjs` path.
- Linux-only PNPM/native source-build modules.
- General Gentle AI GGA/preset/upgrade code removed or retained outside the shell flow.

## References

- `docs/gentle-shell-common-selection.md`
- `docs/gentle-shell-linux-install.md`
- `.github/workflows/ci.yml` (`darwin-runtime`)
- `scripts/darwin-release-blockers.sh`
- `scripts/darwin-release-blockers.manifest`

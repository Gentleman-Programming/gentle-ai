# Gentle Shell macOS parity
Objective: make `gentle-ai shell install` work on macOS with the Linux contract instead of the current unsupported-platform refusal.
Branch: feat/shell-macos-parity (based on #5279 head add89dce5 = main + Windows). Delivery: single PR with size:exception (user decision). Test runner: `go test` natively on the operator Mac (darwin/arm64) plus Linux/Windows CI.

## Specs
- S1. Parity scope (L1, L3): "faltaria hacerlo para mac"; "presentamos un odd para hacer la paridad en mac". macOS gets the installer launched from the Gentle AI TUI/CLI like Linux and Windows.
- S2. Containment (L4 answer 1): "Process group + rlimits". Owned processes run in their own process group with setrlimit limits and are reaped; the limitation that a descendant calling setsid can escape the group is documented, never hidden.
- S3. Modes (L4 answer 2): "Separate + Shared + recover". Same modes, confirmations and recovery contract as Linux.
- S4. Platform (L4 answer 3): "Solo arm64, macOS 14+". Other architectures and older macOS refuse before any change.
- S5. Qualification (L4 answer 4): "Sí, calificación nativa". A native macOS qualification workflow on macos-latest; support is declared only with that evidence. Native tests also run on the operator Mac in an isolated scratch directory (assumption: concrete scratch path is authorized when the first native install test runs).
- S6. No regression: Linux and Windows behavior, contracts, hashes and CI stay identical; every existing integrity, ownership and custody guard keeps its strength on all platforms.
- S7. Delivery (L5 answer 2): "size exception" — one PR instead of a chain.

## Tasks
- T1 | S1,S6 | delegated (context backstop; worker muzuti2k-e-3blu) | in_progress | Move POSIX-shareable code out of *_linux.go into unix (linux||darwin) files behind small per-OS syscall helpers (stat times, termios, no-replace rename); Linux unchanged; darwin compiles; darwin still refuses at the entry points.
- T2 | S1,S4,S6 | pending | Darwin kernel/path/ownership checks: non-root, arm64, macOS>=14, /private canonicalization, case-insensitive APFS disjointness, refuse extended ACL/flags/quarantine xattrs, F_FULLFSYNC, RenamexNp RENAME_EXCL. High risk.
- T3 | S1,S4 | pending | Pinned darwin-arm64 artifacts (Node, fd, rg, gentle-ai release binary) and portable bootstrap without GNU stat/sha256sum.
- T4 | S2 | pending | Process supervision and cancellation for darwin: process group, rlimits, foreground tty, reap; documented setsid limitation. High risk.
- T5 | S3 | pending | Shared + recover on darwin. High risk.
- T6 | S1,S4 | pending | CLI help, TUI model and docs for macOS; enable darwin entry points.
- T7 | S5 | pending | shell-macos-qualification.yml native workflow with evidence receipts.

## Log
- L1. User (2026-10-08): "necesito algo de ti, fijate los pr del instalador de gentle shell desde la tui de gentle ai hechos por deco, los hizo para windows y linux, faltaria hacerlo para mac"
- L2. User (2026-10-08): "ok primero quiero que hagas review profundo a ambos prs y los mergiemos si los ves bien, una vez terminemos eso, presentamos un odd para hacer la paridad en mac te parece?"
- L3. Read-only map (installer feature L50): macOS reaches user_install_unsupported.go through the non-Windows CLI; main risk is containment (no cgroup2/systemd/Job Object equivalent); Linux code uses Stat_t.Mtim, TCGETS, Renameat2 and /proc; /private symlinks and case-insensitive APFS affect canonical checks; release already ships darwin_arm64 gentle-ai assets; Darwin Runtime CI job runs no shellinstaller tests.
- L4. User answers (2026-10-08): 1) "Process group + rlimits" 2) "Separate + Shared + recover" 3) "Solo arm64, macOS 14+" 4) "Sí, calificación nativa". User: "No entiendo el drama, tu estás corriendo en Mac, no deberia de tener problemas como para ofrecer no hacerlo".
- L5. User (2026-10-08): "podes ir viendo el de mac?" then answers: 1) "Sí, arrancar ya" 2) "size exception". Implementation authorized; feature document created before the first source write. Base add89dce5 chosen so the branch matches main once #5279 merges.
- L6. Worktree .worktrees/macos-parity-01a11bdf created on new branch feat/shell-macos-parity at add89dce5. T1 delegated to one bounded gentle-ai-worker (route: delegated, trigger: long-session context backstop) with exact allowed surfaces (user_*_{linux,unix,darwin}[_test].go, user_install.go), RED/GREEN discipline and verification commands; darwin entry points must keep refusing in T1.

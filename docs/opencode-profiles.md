# Native OpenCode background subagents

> [!NOTE]
> These docs track `main`, which may include unreleased changes. For the latest release, see the [v4.0.0 docs](https://github.com/Gentleman-Programming/gentle-ai/tree/v4.0.0/docs).

← [Back to usage](usage.md)

Gentle AI uses OpenCode's native subagents through its `task` permission. It does not install the legacy `background-agents.ts` plugin by default. Background execution is optional and independent of ODD model configuration.

## Choose an execution preference

Install and sync accept the same preference:

```bash
gentle-ai install --agent opencode --opencode-background-subagents=on
gentle-ai sync --agent opencode --opencode-background-subagents=off
```

Use `auto`, `on`, or `off`. You can also set `GENTLE_AI_OPENCODE_BACKGROUND_SUBAGENTS=auto|on|off`. The CLI flag takes precedence, then a non-empty environment variable, then the previous managed choice in Gentle AI state; the default is `auto`. An interactive installer may ask for a choice when no prior or explicit setting exists. Cancelling installation does not save a new preference.

When activation is enabled, Gentle AI manages launchers under `~/.gentle-ai/bin/` (`opencode` on POSIX; `opencode.cmd` and `opencode.ps1` on Windows). The launcher sets `OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS=true` only if the variable is unset: an explicit `false` keeps execution in the foreground. On POSIX systems Gentle AI also persists `~/.gentle-ai/bin/` in your login profile; see [Platforms](platforms.md) for which profile is used and when you must add the line yourself. Restart OpenCode after activation, and start a new login shell so PATH includes the launcher directory.

Managed launchers preserve the executable's stable package-manager symlink (for example, `/opt/homebrew/bin/opencode`) instead of pinning a versioned Cellar target, so upgrades do not invalidate the launcher. If an older, unedited Gentle AI launcher fails, runtime detection can use an OpenCode executable elsewhere on `PATH`, outside the managed directory, so `gentle-ai sync` can regenerate the launcher. Edited or user-owned launchers are not bypassed; the alternate runtime must still report a supported version.

Sessions launched through `opencode serve`, `opencode attach`, or OpenCode Desktop may not inherit the managed launcher environment; they fall back to foreground execution. Gentle AI does not rewrite their configuration.

Background jobs are process-local and non-durable: restarting OpenCode loses them. They provide no filesystem isolation, so use them only for independent read-only work, not dependent tasks or parallel writers in one worktree.

For model selection and review roles, use the TUI **Configure Models** screen; see [Usage](usage.md#model-assignment).

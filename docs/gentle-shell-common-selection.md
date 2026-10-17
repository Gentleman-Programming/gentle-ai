# Common Gentle Shell selection boundary

The CLI selector can be embedded in an existing Bubble Tea program without starting an installer worker. The owner remains responsible for restoring its terminal before invoking the existing installation entry.

```go
model := cli.NewShellInstallModel(cancel)
// Run/update this model inside the owning tea.Program.
selection, err := cli.ShellInstallOutcome(finalModel)
args := cli.ShellInstallArguments(selection)
```

- Initial and cancelled selectors return no executable arguments. Escape and Ctrl+C invoke cancellation and quit the selector.
- Confirmation only records a reviewed selection and quits. It does not grant installation authority: the backend still validates the physical destination/confirmation token.
- The argument vector preserves the existing platform entry protocol.
- Windows selection-only behavior is explicit opt-in through this constructor. Ordinary Windows installation still owns its worker and waits for stop/reap after cancellation.
- An unrelated final model returns an actionable error.

The existing Linux/macOS `gentle-ai shell install` route already consumes these helpers: it creates the selector, reads its outcome and forwards its argument vector only after its owning program returns. Cancellation still produces no worker invocation.

The ordinary application opts into `WithShellInstaller()` and exposes **Install Gentle Shell** in Welcome. Selection stays inside its existing Bubble Tea program; only a confirmed result after terminal restoration reaches the guarded host handoff. See [Linux installation and limits](gentle-shell-linux-install.md).

## Verification and rollback

On Linux, `TestShellInstallParentPhysicalPTY` drives the real common TUI through an owned PTY: confirm, Escape/Ctrl+C while editing and Escape/Ctrl+C on review. It observes exactly one parent program, a fresh physical-selection token and full termios restoration before the inert worker boundary. Successful cleanup joins the parent and PTY reader before restoring process-global state. Each scenario runs in an isolated invocation of the same test binary, with a bounded parent wait/kill/reap; a failed join fails the test without leaking hooks or streams into its parent test process. Worker count is exactly zero for cancellation and one for confirmation. A controlled stalled-child case checks the deadline, completed wait and absent PID; its expected timeout is a negative fixture, not a suppressed product failure. The spy never acquires or installs a runtime; startup network commands are isolated, so this is not full ordinary-startup, installed or human visual acceptance.

`TestShellInstallEmbeddedDefaultsAndCancel`, `TestShellInstallEmbeddedConfirmationOnlySelects` and `TestShellInstallEmbeddedOutcomeRejectsOtherModels` cover the model boundary and platform argument roundtrip. Confirmation in these tests is a model fixture, not physical consent or native Windows/Darwin acceptance.

Reverting this unit removes the exported selector/selection helpers, their platform constructors, tests and this guide. It does not remove any installer backend or change personal runtime preferences. No benchmark claim or review-lifecycle change is made.

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

The ordinary application opts into `WithShellInstaller()` and initially focuses the original **Install Gentle-Shell, our own agent** campaign. Enter opens the grouped configuration menu with the agreed fresh defaults: Gentleman, Stable SDK, background subagents/Strict TDD/global RDD/CodeGraph/Context7 ON and GGA OFF. Those are actual `UserExperience` selections, not recommendation-only checkmarks. Re-editing an existing request preserves its explicit OFF or inherited choices; omitted legacy transport remains unchanged. The configuration screen also selects the install target: Separate install (own prefix, agent, runtime and launchers) or On my existing Pi (Shared). Shared is offered only when exactly one npm-style Pi install is detected (agent `~/.pi/agent` plus a prefix holding the stock bundle); the operator never types the detected paths, and Destination shows them read-only. Main preview requires Separate. The editor switches each optional between ON and OFF, shows an incoming inherited value once as Inherited, and materializes an explicit choice on the first toggle. Explicit optional effects require a qualified Linux amd64 fresh Separate profile; unsupported Main/GGA-ON/platform combinations remain refused. Main preview now reuses the same canonical private writers and CodeGraph runtime for Strict TDD, global RDD, CodeGraph and Context7 on the legacy authenticated Main worker; GGA ON has no qualified consumer in this installer either, so Continue names that reason on both channels instead of failing later at Destination. Retired backend controls are not reintroduced. Selection stays inside its existing Bubble Tea program; only a confirmed result after terminal restoration reaches the guarded host handoff. See [Linux installation and limits](gentle-shell-linux-install.md).

Wide configuration retains colored headings above each group. Its contextual panel separates About, changes, current value, recommendation and rationale; recommendations never replace selected or inherited values. Continue and Cancel have short action-specific panels. There is no Advanced section: provider input stays beside GGA only when GGA is ON or a retained provider needs correction; inherited/OFF choices with no provider show no empty field.

The embedded destination view uses the historical two-column field/About layout. Ready offers the original Plan action cursor and About panel, with a zero-based line-offset indicator and responsive compact fallbacks. Selecting Install forwards only the existing physical confirmation/quit path; Back clears old consent without discarding the request; Cancel returns to Welcome. Complete JSON, SDK qualifications and changes/scopes remain pageable, including long paths and confirmation values. No additional preference authority or nested program is introduced.

## Verification and rollback

On Linux, `TestShellInstallParentPhysicalPTY` drives the real common TUI through an owned PTY: confirm, Escape/Ctrl+C while editing and Escape/Ctrl+C on review. It observes exactly one parent program, a fresh physical-selection token and full termios restoration before the inert worker boundary. Successful cleanup joins the parent and PTY reader before restoring process-global state. Each scenario runs in an isolated invocation of the same test binary, with a bounded parent wait/kill/reap; a failed join fails the test without leaking hooks or streams into its parent test process. Worker count is exactly zero for cancellation and one for confirmation. A controlled stalled-child case checks the deadline, completed wait and absent PID; its expected timeout is a negative fixture, not a suppressed product failure. The spy never acquires or installs a runtime; startup network commands are isolated, so this is not full ordinary-startup, installed or human visual acceptance.

`TestShellInstallEmbeddedDefaultsAndCancel`, `TestShellInstallEmbeddedConfirmationOnlySelects` and `TestShellInstallEmbeddedOutcomeRejectsOtherModels` cover the model boundary and platform argument roundtrip. Confirmation in these tests is a model fixture, not physical consent or native Windows/Darwin acceptance.

Reverting this unit removes the exported selector/selection helpers, their platform constructors, tests and this guide. It does not remove any installer backend or change personal runtime preferences. No benchmark claim or review-lifecycle change is made.

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

This is an internal application API, not a new user command. It does not add a Welcome entry, experience controls, terminal handoff integration, acquisition or installed Ready. Those are separately qualified units. Default CLI behavior is unchanged.

## Verification and rollback

`TestShellInstallEmbeddedDefaultsAndCancel`, `TestShellInstallEmbeddedConfirmationOnlySelects` and `TestShellInstallEmbeddedOutcomeRejectsOtherModels` cover the model boundary and platform argument roundtrip. Confirmation in these tests is a model fixture, not physical consent or native Windows/Darwin acceptance.

Reverting this unit removes the exported selector/selection helpers, their platform constructors, tests and this guide. It does not remove any installer backend or change personal runtime preferences. No benchmark claim or review-lifecycle change is made.

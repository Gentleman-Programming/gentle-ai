# Preserve native cancellation after the child has started

The native client must retain `context.Canceled` or `context.DeadlineExceeded`
when the existing owned process runner returns after cancellation. An uncertain
process-group settlement must retain priority over either context classification.
No installer, source supplier, signal hook, dependency pin, or TUI is added.

## Pertinent proof

`TestUserNativeRuntimeAfterStartCancellation` runs only in the bounded offline
Linux amd64 Guest. It invokes the shared transport's `collect` protocol with the complete reviewed local source
and authenticated Node, identifies the exact private argv/cwd/process group,
then holds that invocation through a kernel pidfd **after exec**. It confirms
stopped state, no exit and an active context before cancellation or deadline.

PASS requires the native error category and context cause, kernel-confirmed
Node exit, no remaining member of its process group, post-run source comparison,
a private project marker unchanged and owned cleanup. Observer failure or
unsettled cleanup is not behavioral RED/PASS; failed workspaces remain evidence.
The default priority test checks that uncertainty cannot become ordinary cancel.

## Scope

This proves transport cancellation after Node exec, not a partially installed
product, cooperative interruption during an install action, a detached descendant,
production user-manager qualification, or installed acceptance. The original
factory/source is not rewritten. Ordinary TUI routing, PNPM publication/readbacks
and both launchers remain separate work. Rollback removes the context propagation
change and these tests without reverting source binding or original bootstrap.

# Decisions and current work

<!-- Written by `knos export`. Commit this file. -->

A second clone reads this on its first question - it is one of the decision
records knos looks for. Nothing here is private: secrets and private paths
never reach it.


## Decisions

- **skills are the extension point** - Behaviour is added through skills rather than by editing the core.  _(AGENTS.md, Skills)_
- **scheduled tasks are guarded by a lock** - `.claude/scheduled_tasks.lock` prevents a scheduled task running twice.  _(.claude/scheduled_tasks.lock)_
- **install state is guarded by a lock** - `internal/cli/install_state_lock.go` serialises install-state writes.  _(internal/cli/install_state_lock.go)_

## Being worked on right now

_Nothing claimed._

---
<sub>knos export. Claims lapse after 30 minutes.</sub>

# Read-only native cgroup control view

The native-only service builder mounts `/sys/fs/cgroup` read-only alongside the existing source/archive protection. Hiding manager sockets is insufficient when the executing UID owns delegated controller files.

`userNativeControlCheck` verifies genuine cgroup2 and the actual read-only mount flag through an opened descriptor, checks visible descendants with the shared native mount parser, and refuses mount-view changes during inspection. It reads genuine procfs rather than accepting a claimed property or supplied limits.

The parent integration must call this helper from `userNativeExecutionCheck` before source execution. This unit does not change generic supervision, mutate limits, contact a personal manager, or wire the ordinary route. Private EROFS confirmation for aggregate/leaf controls remains combined integration proof.

The sealed execution boundary invokes this check before source execution. The
private fixture regression requires the full boundary to reject a writable
control view, not merely the standalone control-check API.

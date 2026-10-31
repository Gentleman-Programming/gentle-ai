# CodeGraph CLI can reuse the authenticated Shell Node

## Original source037 characterization (historical)

The pinned Linux CodeGraph 1.4.1 CLI library runs unchanged with the already
frozen Shell Node 24.18.0 and upstream's `--liftoff-only` flag. This characterizes
a potential private recipe without materializing the alternate 24.16.0 Node
(123,438,592 bytes) or widening production's 32 MiB per-file guard.

## Reproducible check

In the existing offline Guest, expose the two frozen npm archives and authenticated
Node at `/frozen-codegraph-{main,linux}.tgz` and `/opt/node/bin/node`, then run:

`go test -tags=shell_codegraph_shared_node -run '^TestUserCodeGraphFrozenSharedNode$' ./internal/shellinstaller`

The probe checks caller Node SHA/version and both complete archive SHA512 SRIs
before execution. It explicitly excludes only the known bundled Node member;
every materialized member stays within 32 MiB. Actual CLI version is 1.4.1 and
JSON status reports the owned project uninitialized, with no last index. The
owned HOME/project/artifact inventory is unchanged after both calls; no index,
download command, SDK tool, permission, model or production installer is invoked.

This is characterization, not fabricated RED, installed acceptance, platform-wide
qualification, a publisher signature/SLSA proof or production admission of this
recipe. The original publisher wrapper still selects its bundled Node; this probe
calls its unchanged JS entrypoint directly. No SDK/PATH binding or production
custody/acquisition policy changes. Rollback removes only these tests/docs.

Recovery scope: the original fixture's Node24.18.0 identity and all archive,
CLI/status, inventory and bound assertions are retained, not silently repinned.
This is not qualification of the required Node24.21.0 delivery runtime. No
Python/Go/CLI/SDK probe or installation was executed in this reconciliation;
current runtime integration and its own CI remain separate.

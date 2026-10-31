# Overlapped publication verification

After every copied file is closed, staging overlaps its independent byte
inventory with the existing anchored synchronization traversal. The inventory
has its own reader and buffer; synchronization keeps its eight-worker bound.
No file, hash, Sync, Close, source-preservation or final-seal check is omitted.

Either failure cancels the peer. Synchronization drains every accepted file,
and the inventory is joined before comparison, error cleanup or publication.
The older-kernel immediate-copy synchronization fallback remains unchanged.

A retained real payload experiment reduced the combined traversal interval
from 27.555 to 23.560 seconds. Whole publication varied from 48.606 to 40.223
seconds, including unrelated phase variation. This is not a guaranteed total
installation duration; native acceptance must still meet the original limits.

## Qualified regression fixture

`TestIndependentPNPMOverlapPublish` requires `shell_pnpm_publication`, a fresh
writable `/report`, the read-only retained producer at
`/fixture/install-parent/.gentle-native-4277763020/producer`, and repository
sources at `/source`. Its 84,345-entry fixture includes the original receipt;
it does not rerun the original installer. The test checks final manifest,
canonical neutral/off consumers, complete source preservation and stage removal.
Ordinary cancellation and both-error regressions run without this fixture.

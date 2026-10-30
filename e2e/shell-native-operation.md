# Native operation supervision

Native installation reuses the ordinary service lifecycle: authenticated
supervisor and helper, existing manager, unique unit, physical termination
receipt, streams and uncertain-outcome preservation. Generic services retain
exactly their previous argument and cancellation paths.

A native plan requires a remaining deadline of at most 140 seconds and a physical
bounded aggregate. The coordinator retains its full identity map
and manager access; a producer uses protected source custody and bus masking.
Both retain the existing resource limits and 300-second service ceiling.

Only the creator-held manager identity may cancel the entire populated slice,
including coordinator and producer siblings. A read-only membership handle
cannot do that. Normal settlement still refuses populated slices. Failed stop
or physical readback preserves uncertainty; it never authorizes cleanup.

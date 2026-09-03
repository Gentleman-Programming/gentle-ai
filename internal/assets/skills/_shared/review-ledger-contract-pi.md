# Native Compact Review Orchestration

Pi uses the compact gentle-pi facade for this lifecycle. Provider-issued authority and every opaque binding are authoritative; prompt prose never creates authority or decides delivery.

## Entry rule

After authorized source-mutating implementation is complete and normalized, and before reporting it complete, call `gentle_review` with {"operation":"inspect"}. Do this once per candidate whenever the user-owned review switch is enabled. The facade returns the only offered START route; do not infer, reconstruct, or replace it.

## Atomic lifecycle

1. **Inspect before START.** Call `gentle_review` with {"operation":"inspect"} before START. Retain only the provider-issued authority and opaque bindings returned by the facade.
2. **Freeze once.** Invoke only the returned START operation with its exact opaque binding. Retain the returned `lineageId`, revision, target, and `workspaceRoot` as opaque values. Do not start another lineage, reuse burned authority, or perform ambient recovery.
3. **Stay bound.** For later routing, call `gentle_review` with operation `status`, the exact retained `lineageId`, and `workspaceRoot` only when needed. Never run raw shell STATUS. Route only from the returned transition; for `execute`, invoke its offered facade operation with its exact opaque binding; for `stop`, run no lifecycle operation.
4. **Collect exactly.** Use `gentle_review_capture` for one current returned slot or `gentle_review_capture_group` for the complete current reviewer group. Submit only the returned opaque binding and result. After collection, use bound facade STATUS again only when the returned transition requires it.
5. **Acknowledge exactly.** Only the exact provider-issued acknowledgement continuation burns approved authority. Report the burn from its returned envelope, never from a later STATUS.

Pi never reconstructs lineage, target, revision, repository context, lens, order, or commands. It never appends, removes, parses, or rebuilds provider-issued opaque bindings. Go owns repository binding, frozen evidence, provider context, validation, admission, correction scope, and closure.

A malformed, incomplete, or unavailable capture never reaches acknowledgement. Use bound facade STATUS once, and relaunch only when it reoffers the same bound slot. An approved capture awaits acknowledgement; it is not burned. On `approved`, use bound facade STATUS to obtain or replay the exact provider-issued `acknowledge-approved` continuation, then execute it unchanged. Only its successful returned envelope burns authority; do not issue STATUS after that burn. On `correction_required`, continue only through exact bound facade STATUS and the provider-issued correction route.

## Delivery follows ordinary repository policy

Commit, push, PR, and release remain separate human decisions under ordinary repository policy. A review outcome is informational and never authorizes delivery.

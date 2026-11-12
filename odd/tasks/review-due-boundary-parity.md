# Review-due boundary parity
Reconcile generated RDD guidance with the provider-owned due-only entry rule.
Branch: fix/review-due-boundary-parity. Delivery: <=400-line slices; focused Go render tests and independent public guidance verification.

## Specs
S1. User selected "Respetar reviewDue (recomendado)": "Cerrar el bloque anterior sin abrir una review nativa si el proveedor dice que no corresponde; conservar la excepción por pedido explícito del usuario."
S2. User: "si te autorizo todo hasta terminarlo". Apply the accepted reconciliation to the existing review-contract parity task; do not change actual RDD mode, authority or candidate consent.

## Tasks
T1 | S1-S2 | parent inline | done | Reconcile generated boundary/assessment guidance and pin it in render regressions | commit: e8b4a631259679b59165d55f7afff275dfe643b9
T2 | S1-S2 | independent verifier | done | Verify public generated contracts and unchanged RDD-off behavior | commit: e8b4a631259679b59165d55f7afff275dfe643b9 (verified candidate)
T3 | S1-S2 | parent | in_progress | Deliver verified <=400-line change and required mirrors under repository policy | commit: pending

## Log
L1. User: "si te autorizo todo hasta terminarlo".
L2. User selected "Respetar reviewDue (recomendado)" in the explicit choice between due-only review and forcing review at every high-risk boundary. The displayed chosen option was "Cerrar el bloque anterior sin abrir una review nativa si el proveedor dice que no corresponde; conservar la excepción por pedido explícito del usuario."
L3. Existing Pi review ledger entry rule requires reviewDue or an explicit review request; generated routing currently forces review of pending slices, infers due after assessment failure, and unconditionally routes medium/high through consent. Reconcile guidance without changing provider code or claiming unavailable assessment proof. Preserve pending boundary accounting rather than invent a reviewed boundary. RDD remains off in the live session.

L4. RED job171 reproduced all three contradictions; focused job175 passed after the fix. Full job177 failed only the intended routing golden drift. Official regeneration changed four RDD digests and Pi text; full assets/guidance, formatting and diff checks passed in job179.
L5. Independent verifier mwdp0wyz-1z-nzg2 passed S1-S2: 3 top-level tests, 36 runtime subtests; 75 differential snapshots, 9 intended changes and 66 identical, including all 14 non-RDD renderings. Actual installation carriers and consent/authority preserved. No real RDD operation. Original scratch vanished after completion; export-only task mwdp7ysy-20-3pf2 recovered exact probe source and original captured output excerpts, not complete logs, into /private/tmp/review-due-verify.hIezbW/evidence. Missing original files are listed in artifact-recovery.json; no tests were rerun to replace them. The recovered public regression file passes in the repository after gofmt (job187).

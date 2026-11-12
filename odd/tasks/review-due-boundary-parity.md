# Review-due boundary parity
Reconcile generated RDD guidance with the provider-owned due-only entry rule.
Branch: fix/review-due-boundary-parity. Delivery: <=400-line slices; focused Go render tests and independent public guidance verification.

## Specs
S1. User selected "Respetar reviewDue (recomendado)": "Cerrar el bloque anterior sin abrir una review nativa si el proveedor dice que no corresponde; conservar la excepción por pedido explícito del usuario."
S2. User: "si te autorizo todo hasta terminarlo". Apply the accepted reconciliation to the existing review-contract parity task; do not change actual RDD mode, authority or candidate consent.

## Tasks
T1 | S1-S2 | parent inline | in_progress | Reconcile generated boundary/assessment guidance and pin it in render regressions | commit: pending
T2 | S1-S2 | independent verifier | pending | Verify public generated contracts and unchanged RDD-off behavior | commit: pending
T3 | S1-S2 | parent | pending | Deliver verified <=400-line change and required mirrors under repository policy | commit: pending

## Log
L1. User: "si te autorizo todo hasta terminarlo".
L2. User selected "Respetar reviewDue (recomendado)" in the explicit choice between due-only review and forcing review at every high-risk boundary. The displayed chosen option was "Cerrar el bloque anterior sin abrir una review nativa si el proveedor dice que no corresponde; conservar la excepción por pedido explícito del usuario."
L3. Existing Pi review ledger entry rule requires reviewDue or an explicit review request; generated routing currently forces review of pending slices, infers due after assessment failure, and unconditionally routes medium/high through consent. Reconcile guidance without changing provider code or claiming unavailable assessment proof. Preserve pending boundary accounting rather than invent a reviewed boundary. RDD remains off in the live session.

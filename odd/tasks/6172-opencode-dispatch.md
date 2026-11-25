# OpenCode 2.4.1 immutable review dispatch (#6172)
Target: Gentleman-Programming/gentle-ai; base c26d38c04. Scope: prevent incompatible dispatch, not implement asynchronous capture.
Risk: item 3 (shared review capability); keep bound recovery and other runtime behavior intact.

## Specs
S1. “ok hagamos ese arreglo, commit push y pr” — refuse before launch for known incompatible OpenCode 2.4.1, with an actionable reason, using existing capability and admission boundaries.
S2. “y merge” / “luego de probar que todo pasa” — run local tests and real-host checks, independent verification and applicable native review; publish and merge only when checks pass. Use Refs #6172, type:bug.

## Tasks
T1 | S1 | writer: implement, pin negative and positive controls, update docs | implemented; RED/GREEN for both frozen roles
T2 | S1-S2 | independent proof, local checks, review, commit | in progress
T3 | S2 | push, PR, checks, merge | pending

## Log
L1: ok hagamos ese arreglo, commit push y pr
L2: y merge
L3: luego de probar que todo pasa
L4: Branch fix/6172-opencode-review-dispatch; real host 2.4.1 on clean main failed generic foreground assertion; previous temporary wait prototype NOT safe. Before implementation #6172 approved and label read back. Use authorized gh/HTTPS. No SSH or policy bypass.

L5: Differential on usable V2 declaration: pending refuter/validator newly fall to generic manual_intervention_required. Preserve bound lens capture; return an actionable version-specific refusal for roles that cannot dispatch, leaving authority unchanged.

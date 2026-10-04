# rdd-risk-gated: narrow the deterministic high-risk classifier

Branch `feat/rdd-risk-gated` at origin/main `7af7eef8`. Evidence: re-derivation of 307 stored-high local review lineages (Engram `rdd/history-mining`).
Delivery: work-unit commits on the feature branch; push/PR are the user's decision.

## Specs

- S1 The `update` path token no longer makes a change high risk ("Pasa a medio: el token `update`").
- S2 Touching the 6 RDD authority files (`compact_gate.go`, `compact_store.go`, `compact.go`, `gate.go`, `store.go`, `transaction.go`) no longer makes a change high by name alone ("Pasa a medio: ... los archivos de autoridad").
- S3 A large change to the authority files still gets the full 4 lenses, decided by size, not by name: high when the changed lines inside authority files are >= 400 ("Cantidad de lentes según tamaño: que un cambio grande en los archivos de autoridad reciba las 4 lentes por tamaño, no por nombre").
- S4 The process-spawn pattern skips comment-only added lines, decided per language: a line counts as comment-only only when it starts with that file's comment syntax and contains no code after a closed block comment; unknown languages are not skipped ("Sigue siendo alto: un spawn real en una línea agregada (sin contar comentarios)").
- S5 A shebang counts only when present in the candidate tree, not base-only.
- S6 Path tokens (`auth`, `security`, `webhook`, `payments`, service-token) that hit only test files no longer make a change high ("los tokens que aparecen solo en tests").
- S7 Unchanged high signals stay high: real spawn on an added non-comment line, dangerous sink, executable-mode flip, `.github/workflows/*`, non-test shell scripts, service token, process scan limit, and `auth`/`security`/`webhook`/`payments` on non-test paths.
- S8 Medium and passive behavior and the lens selection per tier are otherwise unchanged.

## Tasks

- [x] T1 S1-S8 narrow ClassifyRisk with tests (route: delegated writer, trigger: high-risk guard change in risk.go + tests; verification: independent gentle-ai-verify) — commit: pending

## Log

- L1 User (2026-10-04), approving the proposal "¿Implemento la opción A en `~/work/gentle-ai-rdd`, como trabajo trackeado con TDD sobre `risk.go`?": "Dale"
- L2 Proposal text approved (option A): "Sigue siendo alto: un spawn real en una línea agregada (sin contar comentarios), un sink peligroso, un cambio de permiso de ejecución, los workflows de CI, los scripts de shell que no son de test, y `auth`, `security`, `webhook` y `payments` cuando no son tests. Pasa a medio: el token `update`, los archivos de autoridad, los spawns en comentarios y los tokens que aparecen solo en tests. Cantidad de lentes según tamaño: que un cambio grande en los archivos de autoridad reciba las 4 lentes por tamaño, no por nombre."
- L3 Evidence: 127/307 stored-high lineages came from already-removed rules; of 180 still high, 90 only via authority files, 9 only via `update` (0 findings in update paths); 10/60 spawn matches were comment-only; CB rate under 100 lines equal for 1 and 4 lenses (11%). Forecast ~150-300 authored lines (single work unit).
- L4 T1 writer: S3 publishes the new reason code `large_authority_change` (signal `auth`). `validateReviewStartRiskReasons` in `internal/cli/review_start_contract.go` is a closed code enum (it already rejects `dangerous_sink`) and the consent wording in `internal/cli/review_mode.go` / `review_consent_contract.go` has no phrase for it, so negotiated START needs a follow-up outside T1's edit surface.
- L5 Correction: large authority changes reuse the existing hot_path/auth reason per authority file instead of a new code, because negotiated START (internal/cli/review_start_contract.go:335) and consent wording only accept existing codes; pre-existing gap: dangerous_sink is also rejected by START (follow-up).
- L6 User authorized a second correction (W1 from independent verify mutm8tdj-11-1cj6): prefix-only comment skipping let real spawns drop to medium (Go `*cmd = *exec.Command`, `/* x */ exec.Command`, C `#define ... popen`, TS `#private` fields); comment skipping is now language-aware. Follow-ups not in scope: A1 `spec`/`test` path segments treated as tests by isTestRiskPath; dangerous_sink START gap.

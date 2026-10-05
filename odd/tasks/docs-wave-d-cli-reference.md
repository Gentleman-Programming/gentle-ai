# docs-wave-d-cli-reference

Issue: #5221 (tracker #4556). Branch: `docs/5221-cli-telemetry-arch` from upstream/main 5d449e52 (includes v4.0.0).
Goal: align CLI reference, telemetry and architecture docs with the code on main. Docs only; no Go changes.

## Specs

- S1 `docs/usage.md`: "Remove the `--strict-tdd` row and add one line saying it was retired in v4.0.0." Evidence: `internal/cli/sync.go:213-227` rejects the flag.
- S2 `docs/usage.md` + `docs/non-interactive.md`: "Add the missing install and sync flags and env vars" — install (`internal/cli/install.go:67-69`): `--channel`, `--opencode-background-subagents`, `--pi-background-subagents`; sync (`internal/cli/sync.go:189-192`): the same background-subagent flags and `--scope`; env vars `GENTLE_AI_CHANNEL`, `GENTLE_AI_*_BACKGROUND_SUBAGENTS`. Values and defaults must match the code.
- S3 `docs/usage.md`: "add a short "Other commands" table that links to the existing docs (`rollback.md`, `telemetry.md`, `review-integration.md`, `skill-registry.md`)" covering `restore`, `telemetry`, `review`, `codegraph`, `skill-registry list`, `uninstall opencode-plugin` (`internal/app/app.go:103-154`); drop the stale "changed in v1.x slice 5" marker.
- S4 `docs/trigger-rules.md`: "Rewrite the `trigger-rules.md` paragraph: file count never decides the route, and high risk adds an independent verifier without changing task size." Per L5, "without changing task size" means high risk brings no feature document or delegated writer; it does take the task out of the small path (routing.go Task Size criterion 2). Evidence: `internal/components/agentguidance/routing.go:55,91` (commit 66399a57).
- S5 `docs/telemetry.md`: "Drop "unreleased" from `telemetry.md:30`, and reword the SDD contrast at line 264." The aggregate schema was added in 3f5d1e73, contained in v4.0.0. Keep `sdd-*` agent classes at 117,121 but label them legacy (`aggregate.schema.json:54`).
- S6 `docs/architecture.md`: "Refresh the adapter and component lists in `architecture.md`, and point to `docs/agents.md` instead of listing every agent inline." Component tree must match `internal/components/*` on main.
- S7 `docs/review-integration.md`: "Document `--escalate-item` and `--escalate-reason`." Evidence: `internal/cli/review_assess.go:314-315`.
- S8 "Update the 11 banners to v4.0.0": `docs/codebase/{dashboard,integrations,interfaces,maintainer-playbook,memory-core,mental-model,reference-map,repository-map,sync-and-cloud}.md`, `docs/architecture/{organic-rdd,guard-population}.md`.
- Out of scope: README.md and entry docs (wave F), `docs/audits/`, `docs/releases/`. Diff budget: under 400 lines (estimate 130-170).

## Tasks

- [x] T1 S1-S3 CLI reference (usage.md, non-interactive.md) — route: delegated writer (musvsicy-2-mtt1) — commit: 4d21e214
- [x] T2 S4-S5,S7 Behavior docs (trigger-rules.md, telemetry.md, review-integration.md) — route: delegated writer (musvvd6f-3-a1y2) + parent wording fix — commit: efc65e3f
- [x] T3 S6,S8 Architecture and banners (architecture.md, 11 banners) — route: delegated writer (musvxjt5-4-3dum) + parent wording fix — commit: 3cfcafa3
- [ ] T4 Verify all S# (done), RDD review (approved, burned), PR saying "issue 4556" in plain text, #4556 comment — route: verifier + parent — commit: n/a

## Log

- L1 (user, 2026-10-03): "Retomá la wave D de docs (tracker #4556) desde el handoff odd/docs-wave-d/handoff"
- L2 (user): approved the issue draft ("adelante"); #5221 created; user applied status:approved and type:docs ("Listo").
- L4 (evidence, T1): background-subagent flags have no fixed default (flag > env > saved on/off > auto; auto→off non-interactive, opencode_background.go:41-89, pi_background.go:58-111); --channel accepts stable|beta|nightly(alias beta); codegraph row links components.md, uninstall opencode-plugin has no doc (—). Parent confirmed --strict-tdd rejection is in v4.0.0 sync.go:227.
- L5 (evidence, T2): parent reworded S4 — high risk takes a task out of the small path (routing.go Task Size criterion 2) and adds a verifier; feature document and writer still come only from a failed resume test. S7 semantics checked against review_assess.go:209-241. RDD START deferred to T4 by the parent (candidate incomplete; fork main 0dda8895 behind upstream 5d449e52); not a user disposition.
- L6 (evidence, T3): 20 component dirs listed (matches ls internal/components/*/); agent lists point to agents.md (17 agents); no v3.7.0 left in docs/ outside audits/releases. Parent dropped "independent of SDD" from the reviewassets entry. Follow-up (out of S6): other internal/ packages (reviewtransaction, telemetry, storage, doctor, ...) still absent from the tree.
- L7 (evidence, T4): verifier musvzzfg-6-3g29: S1-S8 PASS at 564b540f; 6 focused go test commands ok; links/anchors valid; 0 v3.7.0; diff 18 files +98/-23. S7 fixed by parent in 564b540f (500 UTF-8 bytes).
- L8 (user): granted RDD consent. Lineage review-60d0491544609209, target sha256:1acc25324c75a3fc847754bae0042c57c89e6b0ba032f479e6103015f1b9c131, base-ref 5d449e52 committed-only (fork sync not needed), risk medium, one lens review-reliability. Claude-code collect input rejected by gentle_review_capture (capture-binding-rejected). User chose: continue in a Pi session with `gentle_review status` on this lineage.
- L9 (evidence): this session is Pi; `gentle_review` was reachable through codemode. Facade STATUS returned a Pi materialize binding; review-reliability approved; acknowledge-approved burned authority (consumed revision sha256:c6e78bad7f22dd587f5a65336cae9338411bfd304902569980cc414040496e0e).
- L3 (evidence): explorer musvh21t-1-kvvj report; seed 2 resolved by parent (`git tag --contains 3f5d1e73` → v4.0.0).

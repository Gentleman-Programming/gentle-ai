# Task size canon (gentle-shell#1494, gentle-ai parity)
Branch: `fix/1494-task-size-canon` (on main 665a181a after #5215 merged) · Delivery: single PR with size exception · Runner: `go test ./internal/components/agentguidance/ ./internal/assets/ ./internal/cli/`; full `env -u GENTLE_AI_CHANNEL go test ./... -timeout 30m`
Source spec: gentle-pi `odd/tasks/proportional-task-routing.md` (S2-S6, S8) · Engram mirror: `odd/task-size-canon/tasks` · Route: inline (user: "sin delegar")

## Specs
S1. Parity: "todo lo que hagamos ahora se tiene que replicar para los otros agentes de gentle-ai". The canon routing block and every agent orchestrator carry the gentle-pi Task Size rules.
S2. Small vs large: a task is small when it is understood (within one bounded read batch), its risk is contained, and it can be resumed from the original request and `git diff` alone. Large only when that resume test fails. Counts of files, commands or tests, fixes, or a requested todo list never decide size.
S3. Each mechanism turns on only by its own trigger (ask, mapping, verification, tracking, writer); after it resolves, re-evaluate task size. Small tasks run their focused test and suite inline.
S4. High risk: (1) data or irreversible effects; (2) security; (3) contracts others consume; (4) concurrency; (5) delivery or environment; (6) no test would catch a regression. "Unclear" is high only when a bounded look cannot tell whether (1)-(5) apply.
S5. Agent escalation: "que el agente defina segun nuestros criterios que es algo que amerite una verificacion o un RDD". `gentle-ai review assess --escalate-item <1-6> --escalate-reason <text>` raises passive or medium to high and records an `agent_escalation` reason; it never lowers a tier.

## Tasks
- [x] G2 (S5) inline · `review assess` escalation flags + tests · RED→GREEN · 95ee74d0
- [x] G1 (S1-S4) inline · routing.go Task Size + triggers; 12 orchestrators, shared sections, hermes skill, docs; tests · RED→GREEN · 66399a57
- [ ] G3 (S4) deterministic lowering accuracy · filed as gentle-ai#5216
- [ ] P1 (S1) after merge: gentle-pi `npm run mirror:odd-routing` regenerates the canon fixture

## Log
L1 2026-10-03 user (verbatim): > es que todo lo que hagamos ahora se tiene que replicar para los otros agentes de gentle-ai
L2 2026-10-03 user (verbatim): > Sigue todos termina
L3 2026-10-03 G2 evidence (risk: HIGH, CLI flag contract; independent verifier not run, user forbade delegation): RED `flag provided but not defined: -escalate-item`; GREEN all `TestReviewAssess*` incl. the published-schema check (reason codes are free strings, so `agent_escalation` needs no schema change). Limit: START still selects lenses from the native tier; the escalation makes the review due at this commit (`high_risk`), it does not change the lens count.
L4 2026-10-03 G1 evidence (risk: HIGH, mirrored prompt contract): RED `TestRenderRoutingSizesTasksByUnderstandingRiskAndResumability` failed on missing `### Task Size`; GREEN agentguidance, assets, and components packages. Non-RDD renders map the new verification row through `nonRDDReplacements`. `capabilitymanifest` keeps `writerMinNonTrivialFiles` (JSON contract) but routing no longer renders it.
L5 2026-10-03 rebase: #5215 merged; branch rebased onto origin/main 665a181a (G2 95ee74d0, G1 66399a57); agentguidance, assets, cli packages pass.

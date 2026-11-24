# RDD Live Review Runbook — gentle-ai beta 4.2.1

Live review run for the new gentle-shell conversation. Prepared 2026-10-23.

## 1. Version under test

| Component | Version | How it resolves |
| --- | --- | --- |
| gentle-ai binary | `4.2.1-0.20261023161727-f3cb16ff49e7` | dev-binary registration (`~/.pi/gentle-ai/dev-binary.json`, source=registration, sha256 revalidated per resolution) |
| gentle-shell harness | `4.3.0-1.2.0` | npm package; model routing lives behind `/gentle:profiles` → `~/.gentle-shell/config/gentle-ai/profiles.json` |

Note: the standalone `models.json` in the config home is **retired by design** in 4.3.x (routing consolidated into the profiles store) — not data loss.

## 2. Pre-flight (verified 2026-10-23)

- [x] RDD switch: `gentle-ai review mode status` → `receipt-driven development: on` (global)
- [x] Reviewer routing: sandbox repo pinned to profile `nemotron-3-super-120b` (all 4 `review-*` roles + refuter/validator, NIM refs validated live 2026-10-15)
- [x] Relay timeout: `GENTLE_PI_REVIEW_RELAY_PI_TIMEOUT_MS=7200000` in `~/.env`
- [x] Beta binary registered and resolving (source=registration)
- [ ] In-session re-check: 🌹 amber dev-binary card at boot, `/gentle:dev-binary status`

## 3. Sandbox (already prepared)

Path: `/run/media/bladimir/Datos/Datos/proyectos/work/sandbox/rdd-live-review-20261023`

- Base commit seeded (`chore: seed sandbox for live RDD review beta run`).
- Pin installed: `.git/gentle-ai/profile-pin.json` → `nemotron-3-super-120b`.
- `notes/REVIEW-NOTES.md` left **untracked on purpose** → exercises the intended-untracked selection path.
- Disposable: nothing pushes anywhere; delete with `rm -rf` when done.

## 4. Prompt to paste in the NEW gentle-shell session

Open the new conversation with cwd = the sandbox path, then paste:

```text
Beta test: run ONE real RDD review lifecycle end-to-end in this sandbox repo.

1. Author a small genuine change (one concern, ~80–150 lines, e.g. a log-line
   parser or a lockfile validator) plus one focused test. Commit it as a single
   work-unit commit (Conventional Commits). Do NOT commit notes/REVIEW-NOTES.md —
   leave it untracked on purpose.
2. Then run the review: gentle_review {"operation":"assess"}, then
   {"operation":"inspect"}. Resolve the intended-untracked selection if inspect
   asks for it. Then START ordinary with lenses ["reliability","readability"] and
   a lensesReason naming what you touched and how.
3. Follow the returned transitions exactly. For any materialize capture: relay
   the forecast losslessly first, then resubmit the SAME binding with
   reviewerRunAcknowledged: true. Never hand-type bindings — machine-extract
   them from STATUS output.
4. If a consent envelope (gentle-ai.review-integration.consent/v3) appears,
   relay it to me losslessly and STOP — I decide granted/declined.
5. On approved: execute the exact acknowledge-approved continuation ONCE and
   report the burn from its returned envelope.
6. Report back: gentle-ai --version, candidate commit SHA, full transition
   timeline, forecast contents, consent (if triggered), corrections (if any),
   stop reason codes (if any), wall-clock per reviewer.
Do NOT commit anything after the candidate commit. Do NOT start a second
lineage. Review outcome never authorizes push/PR.
```

## 5. Evidence checklist (gist report format)

Report each with: version + commit + OS + agent + steps + expected + obtained.

- [ ] Transition timeline (inspect → START → collect → acknowledge)
- [ ] Forecast relay + acknowledgement round trip
- [ ] Consent envelope behavior (if triggered for the tier)
- [ ] Intended-untracked selection round trip
- [ ] Reviewer wall-clock per lens (relay models: NIM nemotron-3-super-120b)
- [ ] Final verdict + burn receipt
- [ ] Any stop reason code + continuation attempted

## 6. Stop codes cheat sheet

| Code | Continuation |
| --- | --- |
| `managed_assets_outdated` | Run the `gentle-ai sync` command printed by the stop, then re-query STATUS |
| New commit mid-review (scope change) | Use the exact advertised recover command |
| `corrected_candidate_unavailable` | Change the correction candidate (commit/amend bounded fix), then STATUS |
| `lens_context_budget_exceeded` | Smaller candidate, new transaction |
| `captured_*` / `*_unavailable` / `native_stop_required` | Terminal: maintainer inspects authority, or disable the switch |

Kill switch if the lifecycle wedges: `gentle-ai review mode disable --scope clone`, then ordinary policy decides delivery.

## 7. Rollback

- Sandbox: `rm -rf /run/media/bladimir/Datos/Datos/proyectos/work/sandbox/rdd-live-review-20261023`
- Dev-binary override (user-global, affects all sessions): delete `~/.pi/gentle-ai/dev-binary.json`
- Repo pin (this repo): delete `.git/gentle-ai/profile-pin.json`

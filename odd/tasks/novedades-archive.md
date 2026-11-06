# Feature: novedades archive in docs/novedades

## Objective
Turn the generated "novedades" PDF into a periodic publication with a fixed identity, archived in the repository so the community can consult past editions and future ones stay reproducible.

## Problem / Why
The 2026-09-20 edition was generated ad hoc from a local content JSON. Nothing in the repository records the structure, the naming convention, or the source of an edition, so the next one could drift in style and old ones could not be regenerated.

## Scope
- `docs/novedades/README.md`: what this publication is, fixed structure, naming convention, how an edition is produced, and the index of editions.
- `docs/novedades/plantilla.json`: the fixed template, derived from the approved 2026-09-20 edition, with placeholder content and comments-by-example.
- `docs/novedades/2026-09-20/`: first archived edition — `novedades.json` (source), both PDFs (dark + light).
- Style/design lives in the global `gentle-docs` skill, not in this repo; the README points at it and records the commit range so an edition can be rebuilt.
- Out of scope: publishing a website, automating generation in CI, changing the skill itself (tracked separately below).

## Constraints
- Fixed structure for every edition, in order: cover + stats, "En 30 segundos", "¿Te afecta?", numbered sections, glosario, cierre, anexo de commits.
- Naming: `gentle-ai-novedades-YYYY-MM-DD[-claro].pdf`, folder `docs/novedades/YYYY-MM-DD/`.
- Every edition records its commit range (`<base>..<head>`) so it is reproducible.
- Reader-facing content in neutral Latin American Spanish; repository docs (README) in English per repo convention — verify against existing docs/ files before writing.
- Never show palette hex codes as visible document text.

## TDD
- Mode: strict where code is written. This feature is documentation + data only, so no unit tests apply.
- Verification instead: regenerate both PDFs from the archived `novedades.json` and confirm page counts match the archived files; `gentle-docs` contrast check passes.

## Delivery
- Route: delegated direct (writer trigger: 2+ non-trivial files). One writer.
- Worktree: `~/work/oss/gentle-ai-worktrees/novedades-archive`, branch `docs/novedades-archive`, base `upstream/main` (f0782af2).
- Commits: Conventional Commits on the branch. Push/PR remain the user's decision.

## Tasks
- [x] T1 `docs/novedades/` skeleton: README (structure, convention, index, how to produce), plantilla.json, first edition folder with source JSON and both PDFs; commit.
- [x] T2 Parent verification: rebuild the PDFs from the archived JSON, compare page counts, review a preview image, confirm the README index matches the files on disk.
- [x] T3 (outside this repo) `gentle-docs` skill: ship the novedades template as a preset so future editions start from it.

## Acceptance criteria
- A reader can open `docs/novedades/README.md` and understand what the publication is, how to read it, and which editions exist.
- Someone with the skill installed can rebuild any archived edition from its JSON and get the same document.
- The 2026-09-20 edition is archived with both themes and its source JSON.

## Progress / Evidence
- 2026-09-20: worktree created; feature document written.
- 2026-09-20: T1 done. Route: delegated direct (writer trigger, 2+ non-trivial files), one writer.
  - docs/ language convention verified: English (checked `docs/usage.md`, `docs/community-roadmap.md`, `docs/agents.md` — `## Section` headings, `← [Back to README](../README.md)` back-links, concise prose). README.md written in English; archived edition content stays in Spanish (per its own `lang: "es"`).
  - Built: `docs/novedades/README.md`, `docs/novedades/plantilla.json`, `docs/novedades/2026-09-20/{novedades.json, gentle-ai-novedades-2026-09-20.pdf, gentle-ai-novedades-2026-09-20-claro.pdf}`.
  - Verification (all commands run in the foreground from the worktree):
    - `build.py docs/novedades/2026-09-20/novedades.json --out <scratchpad>/build-edition --theme both`: exit 0, both PDFs produced.
    - `build.py docs/novedades/plantilla.json --out <scratchpad>/build-plantilla --theme both`: exit 0, both PDFs produced (proves the template is valid and buildable).
    - Page counts via pypdfium2: archived dark 12 / light 12; rebuilt dark 12 / light 12 (match); template dark 8 / light 8 (fewer sections by design, not compared to the archive).
    - `scripts/contrast.py`: "all 25 token pairs pass >= 4.5:1 in every theme", exit 0.
    - Every link/path in the README index (`README.md`, both PDFs, `plantilla.json`, `novedades.json`) confirmed to exist on disk.
  - Commit: see git log on `docs/novedades-archive` for the T1 commit hash and subject.
  - Deviation: none. Open question: none for T1 (T2 parent verification and T3 skill-side preset remain open, as scoped).
- 2026-09-20 T2 (parent, inline): rebuilt both PDFs from the archived `novedades.json` -> 12/12 pages, extracted text identical to the archived copies; `contrast.py` exit 0; preview rendered and reviewed; README index links resolve.
  - Two findings fixed in commit `7e2ba04f`: the README claimed a rebuild is byte-for-byte (false — PDF build metadata changes the hash; page count and text do match), and nothing linked to the archive, so the community could not find it. Added a `Novedades` entry to the README nav.
- 2026-09-20 T3: shipped `assets/examples/novedades-plantilla.json` in the global `gentle-docs` skill and referenced it from SKILL.md; `pytest` -> 26 passed (the new example is covered by the parametrized build test).
- Status: all tasks done. Branch `docs/novedades-archive` has 2 commits, not pushed. Push and PR remain the user's decision.
- 2026-09-20 (correction): fixed the commit range in `annex.note` — it read `Rango: 82a6de96..upstream/main`, which is unreproducible (`upstream/main` is a moving ref) and unanchored (nothing named the release the base hash corresponds to). Verified `82a6de96` is exactly the `v3.4.0` tag and `f0782af2` is the current `upstream/main` tip.
  - Route: delegated direct (writer trigger: 2+ non-trivial files touched — `novedades.json`, `plantilla.json`, `README.md`, both PDFs).
  - Edits:
    - `docs/novedades/2026-09-20/novedades.json` (`annex.note`, line 334): `Rango: 82a6de96..upstream/main` -> `Rango: v3.4.0 (82a6de96)..f0782af2`.
    - `docs/novedades/plantilla.json` (line 173): `Rango: <hash-base>..<hash-head>` -> `Rango: <tag-release-anterior> (<hash-base>)..<hash-head>`.
    - `docs/novedades/README.md`: "Reproducibility" section now states the anchoring rule (base = latest release tag + resolved hash, head = fixed commit hash, never a moving ref). "Producing a new edition" step 1 rewritten to resolve the base via `git describe --tags --abbrev=0 main` (or newest `v*` ancestor tag) resolved to its hash, and the head via `git log -1 --format=%H main`; step 2 now also gives `git rev-list --count --no-merges <base>..<head>` for the non-merge commit count (19 total / 17 non-merge for this edition). Index table "Commit range" column now shows `` `v3.4.0..f0782af2` `` (compare URL unchanged, still uses the raw hashes).
  - Rebuilt both PDFs from the corrected JSON and overwrote the archived copies.
  - Verification (all commands run in the foreground from the worktree):
    - `build.py docs/novedades/2026-09-20/novedades.json --out <scratchpad>/rebuild --theme both`: exit 0, both PDFs produced.
    - `build.py docs/novedades/plantilla.json --out <scratchpad>/rebuild-template --theme both`: exit 0, both PDFs produced (template still valid after the placeholder edit).
    - Page counts via pypdfium2: rebuilt dark 12 / light 12 (matches the archived count).
    - `scripts/contrast.py`: "all 25 token pairs pass >= 4.5:1 in every theme", exit 0.
    - Text extraction of the rebuilt dark PDF (pypdfium2): `v3.4.0` present, `f0782af2` present, `upstream/main` absent from the annex range line.
    - `scripts/preview.py` contact sheet rendered and visually reviewed: annex page (page 11 of 12) shows "Rango: v3.4.0 (82a6de96)..f0782af2" cleanly, no layout breakage.
  - Deviation: none.
- 2026-09-20 (rework): the per-edition-folder-plus-PDFs model was measured against the actual cadence and does not survive it. Two facts forced the change: the upstream repo cuts a release every ~1.4 days (40 releases over 57 days) and `main` takes ~19 non-merge commits per day (391 over 21 days); the user decided the publication follows `main` near-daily rather than per release. Consequences: (1) anchoring every edition to the latest release tag makes daily editions repeat the previous day's commits, so ranges must chain instead (base = previous edition's head hash, falling back to the latest release tag only for the first edition); (2) committing two ~76 KB PDFs per edition at daily cadence is ~55 MB/year against a 94 MB repo, so PDFs stop being committed — the per-release consolidated PDF is now a GitHub release asset instead.
  - Route: delegated direct (writer trigger: 2+ non-trivial files touched — new `2026-09-20.md`, new `plantilla.md`, rewritten `README.md`, deletions of `plantilla.json`, `2026-09-20/novedades.json`, both PDFs, and the now-empty `2026-09-20/` folder).
  - Built: `docs/novedades/2026-09-20.md` (converted from the archived `novedades.json`, same content and section order, front matter records the range/count/date), `docs/novedades/plantilla.md` (daily-edition template with the chained-range placeholder). Deleted: `docs/novedades/plantilla.json`, `docs/novedades/2026-09-20/novedades.json`, both PDFs under `2026-09-20/`, and the resulting empty `2026-09-20/` folder. Rewrote `docs/novedades/README.md` for the daily-Markdown / per-release-PDF split, the chained range rule with its first-edition fallback, non-merge counting, how to produce a daily edition, and how to produce the per-release PDF from a consolidation JSON authored against the `gentle-docs` skill's `assets/examples/novedades-plantilla.json` (referenced, not duplicated). Root `README.md`'s `Novedades` nav line was left unchanged — the path (`docs/novedades/README.md`) did not change.
  - Verification (all commands run in the foreground from the worktree):
    - Every internal link/path in `docs/novedades/README.md` confirmed to resolve on disk (`../../README.md`, `2026-09-20.md`) or exist on the filesystem (`~/.claude/skills/gentle-docs/assets/build.py`, `assets/examples/novedades-plantilla.json`, `scripts/setup.sh`, `scripts/contrast.py`, `scripts/preview.py`).
    - `git log --no-merges --format='%h %s' 82a6de96..f0782af2`: 17 lines, extracted and diffed against the 17 annex rows in `2026-09-20.md` — exact match (hash and subject, no drift from the JSON's original data).
    - `git rev-list --count --no-merges 82a6de96..f0782af2` -> 17. `git rev-list --count 82a6de96..f0782af2` -> 19. Both match the front matter and the README's stated numbers.
    - `go run ./internal/gofmtcheck` -> exit 0 (no Go touched; sanity check only).
    - `git diff --numstat f0782af2..HEAD` summed: 532 additions, 0 deletions (binary PDFs and the superseded JSON files net to zero against the f0782af2 base, since none of them existed there either). This exceeds the repo's 400-line PR gate; stated here plainly, no chaining applied per this task's explicit scope (single writer, one or more commits on the existing branch).
  - Commit: `981d9b2f` (`docs(novedades): switch to daily Markdown editions with chained ranges`), this `odd/tasks` update follow-up commit recorded separately.
  - Deviation: none from the assigned scope. Open question: none.
- 2026-09-21 (UTC window): editions were dated by the author's local clock and headed at "the `main` tip at publication time", so the same edition depended on when and where it was generated (a draft landed as `2026-09-21` while the author's local date was still the 20th). The user chose a fixed UTC day window, written the day after.
  - Rule: edition `D` covers `[D 00:00Z, D+1 00:00Z)`; head = `git rev-list -1 --first-parent --before='<D+1>T00:00:00Z' upstream/main`; base still chains from the previous edition's head.
  - Route: direct inline (docs only; the rule was already decided and both files were already read in the parent).
  - Edits: `docs/novedades/README.md` (head rule, new "day is a fixed UTC window" subsection, first-edition figures, index row), `docs/novedades/plantilla.md` (UTC note on `date`), `docs/novedades/2026-09-20.md` redone for its UTC window: `v3.4.0 (82a6de96)..2336d09a`, adding section 5 (the #4818 novedades commits that merged on the 20th UTC).
  - Verification: `git rev-list -1 --first-parent --before=2026-09-21T00:00:00Z upstream/main` -> `2336d09a`; `--no-merges` count 24, total 27; `git diff --shortstat v3.4.0 2336d09a` -> 62 files, +2861/-365; the 24 annex rows diffed against `git log --no-merges v3.4.0..2336d09a` -> exact match; all six in-page anchors resolve to headings.
  - Pending: the untracked `2026-09-21.md` draft is not valid under the new rule; the 21st UTC window closes at 2026-09-22T00:00Z, after which it is regenerated from `2336d09a`. TDD: not applicable (docs only).
- 2026-09-22 (retired from the repository): the 2026-09-21 edition was produced under the UTC window rule (`2336d09a..654200af`, 39 non-merge commits), and producing it showed that a committed daily edition does not fit this repository's contribution flow: every edition would need its own approved issue and pull request, and each one adds CI runs and review noise for maintainers without changing any product behavior. The user decided the publication is delivered only as a PDF in the community Discord, outside git.
  - Route: direct inline (deletions plus one README line; nothing new to understand).
  - Edits: removed `docs/novedades/` (`README.md`, `plantilla.md`, `2026-09-20.md`) and the `Novedades` link from the root `README.md` navigation. This task history stays as the record of how the publication was designed.
  - Verification: `git grep -n -i novedades` outside `odd/tasks/` returns nothing; the root README navigation keeps its remaining four links.
  - Superseded: the committed-Markdown, chained-range and per-release consolidated-PDF rules above no longer apply to this repository.

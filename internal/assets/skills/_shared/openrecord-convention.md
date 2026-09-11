# openrecord Convention (shared across the six participating SDD skills)

<--:openrecord-->

This file is self-contained: a phase agent that has read only this file can run every retrieval move
and emit a contradiction stop, without loading any skill openrecord itself emits.

## Activation — declared, never presence-detected

Consult `gentle-ai sdd-status --json` (or the equivalent status projection) and read
`recordStore.resolved`. It is `openrecord` only when the workspace's `openspec/config.yaml` declares
`sdd.record_store: openrecord` explicitly — the presence of an `.openrecord/` directory on disk does
NOT activate this convention, even when the store is fully populated.

**When `recordStore.resolved` is empty (undeclared), this convention does not apply.** Behave exactly
as if openrecord were not installed: no guard, no warning, no mention of a missing declaration. This
degradation is silent on purpose — every phase runs exactly as it does today.

## Per-phase responsibility map

| Phase | Responsibility |
| --- | --- |
| `sdd-explore` | **Consults.** Runs the full walk below and reports how each record surfaced and which candidates it examined and ruled out. |
| `sdd-propose` | **Contradiction-checks.** Runs the "When your work contradicts an accepted record" check below before its approval gate is offered. |
| `sdd-spec` | **Contradiction-checks.** Same check as propose. |
| `sdd-design` | **Consults and contradiction-checks.** Additionally proposes new records — `## Architecture Decisions` is where a new record appears as a proposal with its rationale. Design never writes a record file. |
| `sdd-apply` | **Writes.** The sole writer — see "Writing records" below. |
| `sdd-verify` | **Validates.** Runs `openrecord validate` for structural shape, and separately checks that each record's prose still describes the system as actually built — a check the binary cannot do. |

`sdd-init`, `sdd-onboard`, `sdd-research`, `sdd-tasks` and `sdd-archive` never touch the record store.
The remediation loop inherits `sdd-apply`'s writer contract; `sdd-archive` writes nothing into it.

## The walk (consult, propose, spec, design)

**1. Which surface are you touching?**

```
openrecord component owners <repo-relative-path>
→ owner: <component>
  map:   decisions/<component>
  specs: specs/<type>/<capability>.md
```

Returns the component that owns the path. If it reports nothing declared, stop and say so — without a
declared surface there is nothing to check against, and guessing which component a file belongs to
defeats the whole mechanism.

It answers both halves. `map` is where the decisions governing this file are filed. `specs` is every
capability that declares this surface — that cannot be resolved from a path, because a capability
crosses surfaces and names them in its frontmatter instead of living under one. Both lists are the
starting point, not the answer: descending is still what enumerates.

**2. Descend, reading descriptions.**

Each level returns the `title` and `description` of everything hanging off it, plus a `kind` —
`group` means descend, `record` means open. Descriptions arrive in the response; never open an index
file directly.

```
openrecord map --for decisions/<component>
→ [group]  <concern>   "…description saying when to descend here."
  [record] <slug>       "…"
```

Descend into every level whose description matches what you are about to do — not only the one that
matches best. Picking one is the most common way to miss the record that governs you. The descent
bottoms out on its own: a subgroup is capped at one level, so there is never a third step down.

Descending is cheap; opening is not — a level costs a few hundred tokens of titles and descriptions,
less than reading one record. Stop when every branch you descended reached records and you decided
open-or-not on each — not when you found something, since finding one record says nothing about
whether a neighbouring concern holds another.

**3. Search when you do not know where to look.**

```
openrecord grep "<wording>" --for decisions/<component>
→ decisions/<component>/<concern>/<slug>.md   [record]  line N, K hits
```

One entry per file, never per line, with `hits` saying how many lines matched — the difference between
*mentioned once in passing* and *this is what the record is about*. `grep` is literal: it hits exactly
when you remember the wording, and misses entirely when the record says the same thing in other words.
A hit is a file, not a level — open a `record`; descend into a `group`'s **parent**.

**4. Search by meaning with `qmd` directly.** openrecord's own `qmd` subcommand has only `status` and
`install` — there is no `qmd query`. Invoke the `qmd` binary itself:

```
qmd query "<question in natural language>" -c <project>-decisions-<component>
```

Query the collection for the component you are working in — decisions are one collection per
component, mirroring the fact that they are closed by component. Pass several `-c` flags only when
deliberately looking across surfaces.

A `qmd://` result is not a coordinate: `qmd://<project>-decisions-<component>/<concern>/<slug>.md:12`
translates to `decisions/<component>/<concern>/<slug>.md` — drop the scheme, read the surface out of
the collection name, drop the line number.

**Check that the search actually ran before believing it found nothing.** A provider that is
unreachable or whose credential expired produces "no results" with a clean exit — the same answer as a
genuine miss. Run `openrecord qmd status` first: it says whether qmd runs at all. If it is not
installed or the collections are not registered, say so and continue with what the other three moves
found — a missing capability, not a failure.

**No single move proves an absence.** Coverage — has everything been accounted for — comes from
reading the indexes on the way down; a record that never surfaced in a search leaves no trace of its
absence. Search locates quickly; it is never the evidence that nothing was missed.

**5. Read the ones that govern you. Fully.** A record is prose written to be understood, not scanned.

## Say what you found

End the walk with this stated, not held in your head:

```
Governs this work:

- decisions/<component>/<concern>/<slug>.md   [accepted]
  Constrains you: <what you may not do>.
  Surfaced by: <owners | descent | grep | qmd>

Looked at, does not apply:
- decisions/<component>/<other-concern> — <why it does not touch this work>.
```

"Constrains you" states the consequence, not a summary a reader would have to work out themselves.
"Surfaced by" says how much weight the finding carries — a record found by descending into the concern
that owns your work is stronger evidence than one a search ranked highly. "Looked at, does not apply"
is the only thing separating *does not apply* from *nobody looked* — omit it and a reader cannot tell
which happened.

A record with `status: accepted` governs you; build within it. A record with `status: pending` settles
nothing — do not treat it as a constraint, and do not treat it as permission either.

## When your work contradicts an accepted record

This is the one thing that stops.

Stop that line of work — not everything, only what depends on the conflict — and surface it with both
sides in view:

```
STOPPED: <the specific piece of work>

What the record settles:
  decisions/<component>/<concern>/<slug>.md   [accepted]
  <the record's own claim, verbatim>
  Its reason: <why the record settled it that way>

What the work requires:
  <the concrete requirement that conflicts>

Why both cannot hold:
  <the actual contradiction>

Ways forward:
  A. <option, and what it costs>
  B. <option, and what it costs>
  C. <option, and what it costs>

Recommendation: <one of the above>
```

Arriving with "this conflicts, what do I do?" puts the whole problem back on the person. Arriving with
both sides and their reasons lets them decide in one step.

**Never resolve it on your own.** A record was ratified by a person; overriding it silently makes the
store a lie, and everyone downstream keeps trusting it. Resolving it alone and reporting afterwards is
not a consultation — it is a decision taken and then announced. If the record turns out to be wrong or
stale, that is a fine outcome; it is just not yours to conclude alone.

## Writing records — apply is the sole writer

Only `sdd-apply` writes into the record store, in the same step that implements the governing code —
never a separate pass, and never design or archive.

- **A record that does not exist yet** is created with `openrecord record write` — this replaces the
  whole file, so it is only ever the right verb for a brand-new record.
- **A record that already exists** is updated with `openrecord record edit --section "<heading>"` — a
  section-addressed patch. Every section the change does not name keeps its previous content
  byte-for-byte. `sdd-apply` establishes which case it is — new or existing — before calling either
  command; picking the wrong verb destroys content `record write` would silently overwrite.

**Never touch a record file with a file-editing tool.** A record is a `.md`, and a phase carrying Edit
and Write tools will reach for them exactly as it would for any other file — but every guard openrecord
performs lives on the `record write` / `record edit` path, and an edit that goes around that path goes
around all of them. This binds `sdd-apply` hardest, since it is the sole writer: create with
`openrecord record write`, update with `openrecord record edit`, always — never a direct Edit or Write
call against a file under `decisions/` or `specs/`. It also binds `sdd-design`, whose `## Architecture
Decisions` proposes a record without ever writing one — the proposal is prose in the design artifact,
never a patch to the record file itself. openrecord does not enforce this: the binary does not police
anyone's editor, so carrying this guard is this convention's job, not the tool's.

`sdd-verify` runs `openrecord validate` for structural shape (frontmatter, template sections, branch
anchors) and now also for `body-hash-mismatch` — a record whose body no longer matches the hash the
tool stamped when it was last written or edited through the binary, meaning something changed it
outside `record write` / `record edit`. That check is detection, not prevention: it finds the edit
after the fact, it does not stop it from happening. Separately, `sdd-verify` checks that a record's
prose still matches the system as built — a judgement the binary cannot make. A record `sdd-verify`
refutes is rewritten in the remediation loop, under this same writer contract; `sdd-archive` writes
nothing into the record store, ever.

<--:/openrecord-->

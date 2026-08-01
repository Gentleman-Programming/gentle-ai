# What the ratchet measured — the second detection-gap audit

> Written 2026-07-27, twenty-six commits after
> [the first detection-gap audit](2026-07-27-detection-gap-audit.md) (`2f5002ff`), on the
> same branch and the same day. That audit named two root causes, ranked five fixes, and
> ended with a section on what none of them would fix. Its first fix has since been built
> and run; two of its missing axes have been added; a dozen further defects have been
> closed. This audit asks the only question worth asking of a diagnosis: **did it hold?**
>
> Partly. The two root causes survive and the ratchet did roughly what was predicted. But
> a third pattern is now visible that the first audit filed under the first cause and
> should not have, because it has a different detector; and the honest headline of this
> round is that **the instrument and the record were wrong about as often as the product
> was.** Every number below was re-measured against the repository. Several of the first
> audit's numbers, and several of this branch's own commit messages, did not survive that.

## 1. The escapes under audit

Everything that surfaced *after* `2f5002ff`. The first audit's own table is not repeated;
seven of its eight entries were closed in this window (`4f6e92c2`, `551002de`, `7b29246c`,
`55995e8c`, `1e7de24e`, `9597f85e`, `c696d354`) and those closures are treated below as
evidence, not as escapes.

| Escape | Fixed in | Found by | Surfaced how |
|---|---|---|---|
| `review start` walled dead by a nested, non-gitignored worktree; git's trailing-slash entry for an opaque nested repo failed logical-path canonicalization, exit 1 | `5eb9dd35` | community, issue #1881 | a real ninety-file production repo — the entire receipt path was untestable behind it |
| Discovery failure before any authority existed classified `operation_outcome_unknown`, sending the operator to inspect an authority that never existed | `5eb9dd35` | same report | the same wall, one layer down |
| STATUS and START used two different predicates for one shape; an approved lineage re-staged with a changed tree looped between a STATUS that printed START and a START that created nothing | `719b0f6c` | community, issue #1826 | the reporter's own fixture |
| A zero-byte review governed an archive: an approved receipt whose base tree equals its candidate tree covers no content, and read as allow over unmanaged history | `56370f34` | maintainer, driving #1877 at HEAD | the disable → deliver → re-enable → archive sequence, never chained before |
| `sdd-status` stale/missing-receipt stops ended in a maintainer demand with no runnable command, one of them blocking the disabled window itself | `56370f34` | same sequence | — |
| Darwin ancestry walk opened components with `O_EVTONLY`, whose implicit read authorization refuses a `0111` search-only ancestor — the exact managed-profile shape #1781 exists to support | `3d5fb836` | the new Darwin CI lane, first run | a platform CI had never executed |
| Windows: a normal `.git\gentle-ai` owned by `BUILTIN\Administrators` refused as an unsafe RAR authority path, naming neither path nor owner | `fc72f66d` | community, elevated Windows shell | an ownership every elevated token produces |
| The required-checks meta-test sliced `ci.yml` with a hardcoded job list; the new `darwin-runtime` job was swallowed into `windows-runtime`'s section, so Darwin's own format gate read as Windows satisfying it | `a4fc2fd7` | internal, immediately after `8b62f106` | adding a job the guard did not know about |
| Twenty concurrent exact-replay publishers of identical RAR plan authority: waiters exhausted the bounded lock wait and failed typed, with the identical immutable content already on disk | `6457a4a8` | maintainer, Darwin investigation, #1872 | Darwin filesystem timing; Linux at ~40ms/iteration never reached the window |
| A concurrent finalize loser reaching post-terminal receipt publication met a milliseconds-held lock and got `receipt_publication_pending`, not retry safe. Nothing was pending in any durable sense | `83586ed7` | CI | timing again |
| Per-candidate consent existed only for a human at a terminal; the negotiated contract agents actually drive did not carry the question at all — the maintainer's own agent ran a four-lens review nobody was ever asked about | `5736f2aa` | maintainer, live session | an agent honestly reporting it had consulted a choice nobody made |
| The SDD archive report contradicted facts in its own prompt, summarizing mid-cycle snapshots as final state — pending over finished, open over fixed | `bc4c6b75` | community, issue #1882 | a terminal record a future reader would trust |
| **The bench exited 0 with failed journeys**: a journey that produced no numbers still reported success to any gate reading the exit code | `1d695854` | community, issue #1883 | the instrument, not the product |
| **The tracked `results.json` was swept from a working-tree measurement**, embedding a scratchpad binary path into an artifact whose whole purpose is to name the build it measured | `a54ae8d8` | internal | a directory-wide `git add` |

Twelve product escapes and two instrument escapes. Section 4 argues the second number is
badly understated.

## 2. Did the first audit's diagnosis hold?

### 2.1 The joints: confirmed, and still dominant

Classifying the twelve product escapes by where the defect lived: **nine are joints**, and
they are the same joint families the first audit named, plus two it did not have words for.

- *Two predicates for one concept*: START's recovery edge vs STATUS's classifier
  (`719b0f6c`), fixed by extracting `compactApprovedScopeChangedRecovery` and making both
  consume it. This is the delivery-strategy enum defect wearing different clothes.
- *Producer↔consumer vocabulary*: git's trailing-slash entry for an opaque nested repo vs
  the canonicalizer's path domain (`5eb9dd35`); `ci.yml`'s job names vs the meta-test's
  hardcoded list (`a4fc2fd7`).
- *Operation↔the refusal that should name it*: every `sdd-status` stop over unreviewed
  content (`56370f34`).
- *Machine envelope↔human surface*: the fifth and sixth instances on this branch, in
  `c696d354` and `7b29246c` — inspection computed the distinctions, the denial line
  discarded them. The first audit called this shape a repeat offender at four occurrences.
  It is now at six.
- **New: interactive surface↔negotiated surface.** `5736f2aa` is the envelope↔human-surface
  joint reflected: a question that existed for a terminal and was structurally absent from
  the contract agents drive. The fix sources both surfaces from the same constants "so the
  two surfaces cannot drift" — which is fix #2 (seam tests) applied by hand.
- **New: instruction-layer joints.** `bc4c6b75` is a joint between a launcher that held
  final-state facts and an archiver with no rank ordering to resolve them against. The
  first audit's model of joints was code-shaped; a third of this product's behavior ships
  as prompt assets, and those have joints too.

So the first cause held, and its remedy is visibly working: `55995e8c`'s vocabulary guard,
`5736f2aa`'s shared constants, and `7b29246c`'s "prediction and gate cannot drift" are
three seam tests written in one day by people who had read the audit. Fix #2 needs no
re-argument; it needs an inventory.

### 2.2 The connective tissue: confirmed, but every number was wrong

The second cause — surface outgrowing its edges — held qualitatively and failed
quantitatively. Re-measured today:

| Claim in the first audit | Measured today | Command |
|---|---|---|
| "roughly 2,479 production error sites in the CLI and transaction packages" | **2,385** | `rg -c -g '*.go' -g '!*_test.go' 'fmt\.Errorf\(\|errors\.New\(' internal/cli internal/reviewtransaction` |
| (re-measured mid-round as 2,573) | does not reproduce under any variant tried | — |
| "21 subcommands" | **24 dispatched**, 25 reachable (`review mode` routes ahead of the facade), 21 advertised in the usage line | `internal/cli/review_facade.go:629` |
| "nine are repair or recovery operations" | **13 of 24**. No canonical classification exists in code; nine is not reproducible under any reading | — |

Three of the four load-bearing numbers in the first audit's second root cause were wrong
in the direction that understated the problem, and the fourth was a help string mistaken
for a dispatch table. The argument survives — thirteen repair verbs maintained by memory is
worse than nine — but this is the second consecutive audit whose numbers rotted between
writing and re-measurement. That is itself a finding, and it is the same finding as §4:
**this branch's prose about itself is measurably less reliable than its code.**

Three subcommands (`capture-result`, `capture-evidence`, `preserve-result`) are dispatched
and absent from the usage line; `capture-evidence` is documented in help nowhere. That is
the first cause's exact shape — an operation whose existence no surface admits — sitting
in the help text of the tool built to stop it.

### 2.3 The third pattern: guard scope is a claim about a population

Six of this round's defects are the same shape, and the maintainer named it in flight
without promoting it: `fc72f66d` calls itself "the same too-tight-for-its-threat-model shape
the Darwin lock walk just showed."

| Defect | The guard | Who it walled |
|---|---|---|
| `fc72f66d` | RAR owner predicate accepted only current user or token owner | every elevated Windows shell — `BUILTIN\Administrators` is the default owner of every elevated token, and *the token-owner branch already accepted that exact SID when elevated*, so refusing it from a non-elevated token protected nothing |
| `3d5fb836` (product half) | `O_EVTONLY` ancestry open | `0111` search-only ancestors — the managed-profile population the walk exists to serve |
| `5eb9dd35` (first half) | logical-path canonicalization | repositories with a nested registered worktree |
| `7b29246c` (second half) | abandon required the whole graph to validate | any repair at all: with two bad edges each refusal cited the other. "A repair operation that demands the graph already be healthy can never heal it" |
| `9597f85e` | quarantine eligibility excluded `correction_required` | an operator holding contaminated reviewer input, whose only remaining door was to modify working code |
| `83586ed7` | store lock's instant refusal | post-terminal finishers, where the double-apply the lock prevents cannot occur |

And two run the other way — `1e7de24e` (`finalize --result` admitted fabricated authority)
and `56370f34`'s zero-byte review — so the class is not "guards are too tight." It is
**guard scope disagrees with its threat model**, in both directions.

The first audit would file all of these under §2.1 as guard↔call-site joints. That filing
is wrong, and the reason matters. **A joint has two sides in the repository.** That is
precisely why fix #2 works: you derive both sides from shipped source and assert the
relation. A guard-scope defect has no second side. The threat model is a claim about a
population that lives *outside* the repository — which owners real Windows machines assign,
which modes managed profiles use, which layouts real repositories have, which states real
stores reach. There is no file to diff it against.

Which produces the sharpest correction this audit has to offer. The first audit said
inward-facing tests are *silent* about joints. For guard scope they are worse than silent:
**they certify the error.** The unit test of the RAR owner predicate encoded the same
population assumption the predicate did, written by the same author in the same afternoon,
and passed forever precisely because the belief was consistent. No amount of inward testing
can refute a claim about a population; only a sample of the population can. That is why
five of these six came from a community machine, a new platform lane, or a real production
repository, **and not one came from the bench** — a black-box harness building fixtures
minutes old is, by construction, a sample of one population: ours.

So: two root causes stand, and a third joins them — *the boundary between a guard and its
threat model is unverifiable from inside the repository*. Its detector is not a test. It is
a population.

## 3. What the ratchet measured that the audit could only assert

The first audit's fix #1 was built as `d06c1db3`: an AST ratchet over
`internal/cli`, `internal/reviewtransaction` and `internal/sddstatus` requiring every
production `errors.New` / `fmt.Errorf` origin site to name a runnable `gentle-ai`
invocation, carry a `// refusal:by-design <shape>: <reason>` marker from the closed
vocabulary the bench classifier already uses, or sit in a frozen baseline that may only
shrink. It runs inside `go test ./...` — the one required check — in 0.08s.

**What it verified.** The audit asserted the norm barely existed. The count:

```
sites: 2602 total = 1814 violations + 17 named + 15 annotated + 734 exempt wraps + 22 unanalyzable
```

At birth, exactly **7** sites named a paste-able continuation, and one of those seven is
arguably a regex artifact (`errors.New("gentle-ai package version is unavailable")` matches
`gentle-ai` without naming a command). Six or seven, out of roughly 1,850 non-wrap origin
sites. The audit's claim that the norm did not exist was not rhetoric; it was, if anything,
generous — a rate of 0.4%.

**What it revealed that the audit could not have.** Three things.

First, **the denominator was never one number.** "~1,800 origin refusals" conflates 1,814
unnamed violations with 2,602 constructor sites and 734 exempt `%w` wraps. The ratchet
forced the distinction into existence. Ten of the fifteen production annotations are
`world-action` markers on internal envelope invariants in `review_consent_contract.go`
reading "this envelope is built and validated by the same file; the exit is a code fix, not
a command" — programmer assertions, not operator dead ends. The guard's own header
anticipates this and calls it the honest shape. It is still a real limit on what the
headline ratio proves: **the corpus is a census of error constructors, not of things a
human ever reads.** A count of operator-facing refusals does not exist, and 1,764 is not it.

Second, **prevention by construction is working, and it is measurable.** Across the
twenty-two commits since the ratchet landed, the baseline never grew. Named continuations
went **7 → 17**; by-design annotations went **0 → 15**; and four of the five annotated
production files were annotated by post-ratchet fixes (`5736f2aa`, `719b0f6c`, `fc72f66d`,
`5eb9dd35`) rather than by the ratchet's own seed commit. `5eb9dd35`'s new refusal for an
embedded foreign clone names its three real exits *and* carries a `world-action` marker.
That is the audit's #1 doing exactly what it was built to do, in code written by people who
were forced to answer the question at the moment they wrote the refusal.

Third, **the ratchet ratchets in one direction only, and it is already slack.** The
baseline is 1,764; the live violation set is 1,761. Three entries have been resolved and
never recorded. A ratchet that fails on growth but tolerates staleness silently gives back
the ground it wins.

**Corrections to the record.** The baseline was **1,765** entries at birth and is **1,764**
today. It has shrunk **once, by one entry**, in `c696d354`. The brief for this audit said
1,762 and "shrunk twice"; so does `d06c1db3`'s own commit message, which states a baseline
of 1,762 against the 1,765-line file it committed in the same commit. `human-authority`
remains unused in production: fourteen `world-action`, one `operator-knowledge`, zero of
the third.

## 4. The instrument was the defect, repeatedly

The first audit's §4 credited what the machinery caught. This round the more useful
accounting runs the other way, because the count is high enough to be structural.

**A. The instrument could not fail.**

- `run` wrote results, printed the report and returned 0 unconditionally. A journey whose
  fixture could not be built, or whose assertion fired, reported success to any gate
  reading the exit. Found by community issue **#1883**; closed in `1d695854`, which exits
  nonzero on failed journeys while still writing results first so evidence survives, and
  deliberately keeps `unsupported` non-fatal.
- `go test -run` with a pattern matching nothing exits 0. `scripts/darwin-release-blockers.sh`
  now runs `go test -list '.*'` per package before anything else, so a renamed test fails
  the job by name — "a pattern matching nothing would exit 0 and turn this lane into
  decoration." **The Windows lane's inline `-run '^(…)$'` alternation has no such guard and
  is still exposed to the identical trap.**
- The required-checks meta-test sliced `ci.yml` by a hardcoded job list, so an unlisted job
  was silently attributed to its neighbour (`a4fc2fd7`). A guard whose coverage is a
  hand-maintained list under-covers exactly when something new arrives.
- *(prior round, for the series record)* the friction classifier counted the kill switch
  working — `allowed: false`, `delivery: disabled/unmanaged`, nothing stopped — as friction
  the product inflicted.

**B. The instrument measured something it could not name.** `a54ae8d8`: a directory-wide
`git add` swept a working-tree `results.json` into the tree, with a scratchpad binary path
inside an artifact defined by the named build it measures. *(prior round: a failed build,
an `&&` that did not stop, and a round measuring the previous binary — the tell was
byte-identical totals.)* The harness records `binary` and `binary_version`; it does not
verify freshness. That mitigation is social, and it failed twice.

**C. The report was wrong, and the process caught it before the fix.** Four times in
twelve escapes, the investigation corrected its own premise:

- `3d5fb836`: four of five failing Darwin tests were a **fixture** bug — they handed the
  lock a raw `t.TempDir()` under `/var/folders`, and `/var` is a symlink into `/private`,
  so the #1781 no-follow walk refused the symlinked ancestor **exactly as designed**. Every
  production caller canonicalizes at its store constructor; these tests were the only
  callers that did not. The proposed product change was rejected in place because it "would
  have re-resolved a boundary the product already resolves upstream and weakened the walk
  only by luck." One real product defect, four false alarms, and the class reproduces on
  Linux under a symlinked `TMPDIR` — "the lane was the detector rather than the scope."
- `6457a4a8`: the reported zero-value authority was a test-harness artifact of an oracle
  pushing both channels unconditionally, and the "regression of the receipt-convergence
  work" premise was false — the RAR path carries its own bounded waiter predating that
  refactor.
- `5eb9dd35`: the reported empty stdout "corrected the brief rather than the product" — the
  plain form puts errors on stderr by design.
- `83586ed7`: the brief's claim that `retry_safe: false` contradicts `exact_replay_safe`
  was refuted from the contract's own definitions.

**D. The record claimed things the machinery does not do.** This is the most serious class,
because nothing catches it.

- `8b62f106`'s subject is *"ci: run the Darwin release-blocker manifest natively, **required
  for merge**."* The job exists, runs `macos-latest`, and executes a 46-entry manifest. It
  is **not a required status check.** Verified against the live ruleset
  (`repos/Gentleman-Programming/gentle-ai/rulesets/13932547`), the complete required set is:
  `Check Issue Has status:approved`, `Check Issue Reference`, `Check PR Has type:* Label`,
  `Unit Tests`, `E2E Tests (ubuntu)`, `E2E Tests (arch)`, `E2E Tests (fedora)`. Neither
  Darwin Runtime nor Windows Runtime nor Organic Runtime E2E can block a merge; `main` has
  no classic protection either. The workflow says so itself — *"REQUIRED FOR MERGE: the
  maintainer must add the status check 'Darwin Runtime' … this cannot be done from the
  workflow file"* — and the step has not been taken. `a4fc2fd7`'s comment then asserts
  "Darwin Runtime is a required check like its siblings," which is doubly false: its
  siblings are not required either.
- `d06c1db3`'s "frozen baseline of 1,762 entries," against a 1,765-line file (§3).
- `bench/README.md`'s "Measured: the current build" section quotes `results-after.json` and
  `results-before.json` "in this directory." Both match the gitignore rule `results-*.json`
  and **neither exists on disk**. The README's own argument is that "a number I publish and
  you cannot reproduce is a claim about my honesty"; that section is one.

Ten instrument-or-record faults against twelve product escapes. The uncomfortable reading:
**for a full round, this branch's measuring apparatus and its self-description were
approximately as defective as the thing they measured.** The reassuring reading, which is
equally true, is that every item in class C was caught *before* a fix was written, by a
process that demands premises be verified — the loop is doing the thing the first audit said
a loop must be allowed to do, which is find the instrument wrong.

## 5. Did the new axes find their backlog immediately?

The first audit predicted that every new axis finds its backlog within minutes. **The
damaged-store axis confirmed it exactly; the real-world axis did not, and the difference is
the most instructive thing about them.**

**`damaged-store`** (`29faf5a1`) — five journeys `ds01`–`ds05`, non-black-box by
declaration, writing store bytes directly because `validateCompactRecoveryEdge` runs at
write time so *no CLI sequence* can build a store holding a non-re-deriving recovery edge.
Its first recording measured **nine out-of-band blocks across all five journeys**. One
commit, `c696d354`, closed four distinct defects behind them and took out-of-band to
**zero**, deliberately keeping `dead_end` at 1 for the maintainer-reserved forged
non-pristine shape. Prediction confirmed, unqualified.

The seam is real in the way that matters: `bench/axis.go` is one registry, one flag, one
report section; deleting the axis files leaves the core reporting identical numbers; an
unknown `--axis` name is a hard error, "because '43 journeys' and '43 journeys plus an axis'
are different measurements and a typo must never silently produce the first." And each
fixture reads its damage back out through `review inspect-authority` and re-derives the
record's own SHA-256 revision from the bytes it just read *before* editing — so when the
persisted format moves, the fixture fails first, by name, instead of quietly measuring a
state it did not build. That is the anti-rot discipline class A of §4 was missing.

**`real-world`** (`1d695854`) — twelve journeys `rw01`–`rw12`, black-box, in two families:
ecosystem clutter (nested worktree, 3,000 untracked files, an untracked `.env` with a
sentinel secret, a mutating pre-commit hook, a dirty submodule bump, a shallow clone, fork
topology, three stale lineages, a 15MB ignored binary) and life between commands (rebase,
amend, pull landing mid-lifecycle). Five candidates are documented as *rejected* with
reasons, the byte-exact `git stash` detour among them.

It found **one** out-of-band block: `rw01`, the nested-worktree wall — **which the community
had already found and `5eb9dd35` had already fixed.** The axis did not discover it; it
absorbed it, pinned against the sha it measured. Everything else it did was confirm good
behavior and *price* costs that were previously invisible: an amend passes the gate,
tree-not-commit identity holding where a human actually trips it; the sentinel secret
appears in no emitted byte, proven by a detector whose firing half is itself tested so a
pass is not a tautology; ignored content costs 122 subprocesses against the untracked
tree's 96,294; and untracked clutter escalates a tier-zero change into a lens review and
then denies pre-commit on scope — the product telling a real repository to review or ignore
its clutter, now priced and visible.

So the prediction needs splitting. **Axes that reach states the product refuses to build
find backlog immediately. Axes that reach populations find backlog only at the rate the
population reports them.** The real-world axis's own standing rule states this honestly:
*"community-reported shapes become journeys, because the reporter's fixture is the finding."*
It is a ratchet against regression and a memory for community reports. It is not a
discovery instrument, and the first audit's blanket prediction would have oversold it.

That distinction is §2.3 restated in benchmark terms, and it answers the question of whether
the axis closes the community gap: **it does not. It narrows it in one direction only** —
shapes already reported can never regress. Shapes not yet reported remain exactly as
invisible as before, because the axis samples our population, and the whole point of the
community is that they are a different one.

## 6. What actually caught things, for the record

Credit where the machinery earned it, which it did more than last time:

- **The Darwin lane found a real defect on its first run** — `O_EVTONLY` refusing a
  search-only ancestor — and simultaneously exposed four fixture bugs. Both are wins; the
  second only because the investigation refused to convert a test bug into a product change.
- **CI caught what CI is uniquely good at, again**: `83586ed7`'s post-terminal publication
  loser, after `4f6e92c2`'s CAS race. Timing remains the one axis where CI samples better
  than we do.
- **The damaged-store axis** took nine dead ends to zero in one commit.
- **The refusal ratchet's first day** produced the baseline's only shrink, and its
  vocabulary is now the default way new refusals get written.
- **The vocabulary guard** (`55995e8c`) derives both sides from the shipped embedded
  filesystem through the injector itself — no hand-maintained list — and fails with twelve
  errors naming all three producer sites if the fix is removed. That is the template fix #2
  asked for, built.
- **`5eb9dd35`'s defect report** is the most durable thing in this round: at the single
  choke point every review invocation exits through, any failure still reading
  `operation_outcome_unknown` after the full typed cascade, on a mutating operation, writes
  an artifact and names the issues URL. The boundary is the classifier, not a site list, so
  it cannot rot. It records the command shape and sorted flag names, never raw argv.

The pattern from the first audit holds without amendment: **where the instrument looks, it
finds; nothing it found was subtle once visible.**

## 7. The fixes, ranked

1. **Make the platform lanes required, today.** Highest leverage, lowest cost, and already
   believed done — which is what makes it dangerous. Four macOS defects shipped because no
   lane ran Darwin; a lane now exists, costs ~10x Linux minutes, verifies its own manifest
   against drift, and **cannot block a merge**. The same is true of Windows Runtime and
   Organic Runtime E2E. `ci.yml`'s own comment on the deadcode ratchet states the principle
   this violates: *"A guard that cannot block a merge is decoration."* This is one settings
   change, and until it is made, every claim in this branch's history about platform
   coverage being enforced is false. Add a mechanical check that the required-check set
   matches a list in the repository, so the next assertion of "required for merge" is
   falsifiable — this is class D of §4, and it has no detector at all today.
2. **Give guard scope the treatment fix #3 gave taxonomy.** §2.3 is a root cause with no
   instrument. The cheap, immediate form: **a refusal that excludes a population must name
   what it just excluded in resolved terms** — the owner it found, the path it rejected, the
   mode it saw — not the category it assigned. `fc72f66d` and `5eb9dd35` both did this
   voluntarily, and in both cases a tester's report became reproducible because of it. Make
   it a requirement of the ratchet's annotation for any security or integrity refusal: the
   marker must state the population it believes it is excluding. A guard that names its
   threat model in its own output is a guard a stranger can falsify, and a stranger is the
   only one who can.
3. **Close the ratchet's slack, and separate the operator-facing census from the
   constructor census.** The baseline is three entries loose; make staleness fail, not just
   growth. Then split the corpus: an error reachable on a human-facing path is a different
   object from a programmer invariant, and today's 1,764 conflates them — which is why ten
   of fifteen annotations are boilerplate. Only the first census can carry a target.
4. **Inventory the seams (unchanged from fix #2, now with a starting list).** This round
   produced four hand-built seam guards and three joints nobody had a guard for. The list to
   derive mechanically: interactive↔negotiated envelope parity (`5736f2aa` did one pair by
   hand), workflow jobs↔meta-test coverage (`a4fc2fd7`), help-string↔dispatch-table
   (three subcommands are dispatched and unadvertised right now), prediction↔gate
   (`7b29246c` did one pair), and skill-asset↔consumer vocabulary (`55995e8c`'s template
   generalizes to every prompt asset).
5. **Record the newest axis, and re-record deliberately.** `bench/results.json` holds 48
   journeys (43 core + 5 damaged-store) against `2.2.0-rc.1-pr1801-82e1d1f9`: 40 in band, 0
   out of band, 3 by design, 1 dead end. The twelve real-world journeys are not in it; their
   numbers exist only as prose in the README. And delete or regenerate the "Measured: the
   current build" section, which cites two files the repository does not contain.
6. **Keep adding axes; keep the list of missing ones (§8).**

## 8. The standing list of missing axes

Updated with what is now known. The first audit's list named four; two are now built, and
the honest status of the rest has changed.

| Axis | Status |
|---|---|
| Non-black-box / damaged store | **Built** (`ds01`–`ds05`). Found nine blocks immediately; now at zero out-of-band. |
| Real-world / non-sterile repositories | **Built** (`rw01`–`rw12`). Absorbs community shapes; see §5 — narrows the gap, does not close it. |
| Composition (chained journeys) | **Still missing.** The correction→publication miss that produced `551002de` is still unmodelled; the pairwise space of 43 journeys is ~1,800 cells and we sample the diagonal. `56370f34` came from a maintainer chaining disable → deliver → re-enable → archive by hand, which is exactly the axis that does not exist. |
| History (stores written by older binaries) | **Still missing.** `rw08`'s three stale lineages are the nearest approximation and are written by the current build. |
| Platform | **Lanes exist, gates do not.** Darwin (46-entry manifest) and Windows both run and neither is required (§4-D, fix #1). Coverage still rests on community for anything the manifest does not name. |
| Timing | **CI covers it, and it is the one axis that samples better than we do** — two escapes this round, `83586ed7` and `4f6e92c2`. Say so; do not claim it as ours. |
| Unicode normalization | **Genuinely absent, everywhere.** `rg 'unicode/norm\|norm\.NFC\|norm\.NFD'` returns zero hits in the entire tree; `golang.org/x/text` is present but indirect and never imported for it. Non-ASCII fixtures exist (`docs/naïve guide.md`, `manifest café 文件.json`) but every one is a byte-identity fixture — nothing constructs the same name in two normalization forms. `pathidentity`'s doc names Unicode equivalence as the third of three aliasing properties, and its policy (device+inode via `os.SameFile`) is believed to defer to the volume correctly; the nearest test, `TestSameDirectoryDefersCaseFoldingToTheVolume`, covers case folding only, and the file's own comment admits the symlink fixture is merely *"structurally the same shape"* as a Unicode-equivalent spelling. The testing guide states the consequence plainly: *"That branch has never executed anywhere."* The manifest cannot paper over this, because there is no test for the manifest to name. |
| Interactive-terminal consent | **Structurally outside the bench** — runs are non-TTY by construction, so `human_prompts` counts the number of times the tool *would have asked*. Guide flow 5 is the only route, and `5736f2aa` just changed what the other surface does. |
| Guide flows 25–33 | **Unexecuted.** The guide defines 34 flows and 128 numbered checkboxes, of which **zero are checked**. Flows 27–33 sit under "environments we cannot reach" — network mount, read-only filesystem, disk-full mid-receipt, Unicode-normalizing volume, antivirus mid-write, long paths, clock moving backwards — and flows 25–26 carry their own warning that they *"have never run on a real machine of the platform they describe, only against synthetic test profiles."* |

## 9. What this does not fix

The first audit's residue stands, and this round added to it rather than subtracting.

A ratchet still cannot catch a **wrong** named continuation — one that parses, runs, and
does not help. It also cannot tell an operator-facing dead end from a programmer invariant,
which means its headline number will always be softer than it looks.

Fix #2 cannot reach §2.3 at all. There is no seam to derive when one side of the
disagreement is a population, and the unit test will keep certifying the wrong scope for as
long as the author's belief is self-consistent — which is forever, by definition. The only
countermeasures are a required platform gate and a fast community channel, and both are
sampling, not proof.

The real-world axis does not close the community gap and should never be described as
having done so. It remembers what was reported. Shapes nobody has reported yet are exactly
as invisible today as they were before it existed, and every one of this round's most
expensive escapes — the nested worktree, the Administrators-owned parent, the archive
contradicting its own prompt — arrived from a machine we do not own.

`bc4c6b75` carries the sharpest limit of all, stated in its own commit: the fix ships words
into twelve orchestrator assets, and *"whether the model obeys them only the community
runtime can verify."* A growing share of this product's behavior is instructions to a model,
and instructions have no ratchet, no dispatch table and no exit code. The audit before this
one worried about code with unmaintained edges. The next one will have to worry about
prompts with unmaintained edges, and there is currently no instrument pointed there at all.

And the record itself now needs a guard. Three separate claims in this round — a baseline
count, a required status check, and a benchmark's reproducibility — were false in the
direction of flattering the work, in a branch whose entire argument is that a number you
cannot reproduce is an opinion with decimals. Nothing in the machinery checks the prose.

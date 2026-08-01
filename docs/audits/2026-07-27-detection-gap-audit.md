# Why we kept missing these — a detection-gap audit

> Written 2026-07-27, at the end of the organic RDD recovery effort (PR #1801), after a
> day in which eight further defects surfaced on a branch that had already absorbed
> hundreds of fixes, dozens of mechanical guards, a 48-journey friction benchmark, and
> multiple community testing rounds. The question this audit answers is the maintainer's,
> verbatim: *after this many iterations, why do these keep appearing — is there a root
> problem that keeps producing them?*
>
> There is. It has two halves, both structural, neither mysterious. This document names
> them, shows how every late escape maps onto them, credits what the existing machinery
> did catch, and ends with the fixes that change the class rather than the instance.

## 1. The escapes under audit

Every defect that survived into the final days despite the machinery, with who found it
and what finally surfaced it.

| Defect | Escaped for | Found by | Surfaced how |
|---|---|---|---|
| Kill switch gated `start` only; every mutating verb wrote unguarded; `finalize` minted approved receipts with reviews off | the guard's whole life | extended bench corpus (j30) | a journey that flipped the switch mid-review |
| `finalize --result` performed no admission at all; four hand-written files reached an approved receipt claiming four lenses that never ran | since before this branch — live in production | maintainer anecdote → directed probe | nothing mechanical; a human said "a lens timed out and it signed anyway" |
| Authority store with forged-prefix recovery edges: diagnosis computed, published as `anomaly_classes: []`, no surface admitted the edges (dead end) | unknown; state is CLI-unreachable | community (@Matere413), real repository | a store with *history*, which no test repo has |
| Lost CAS race classified `operation_outcome_unknown`, not retryable, when the product provably knew nothing had mutated | since the CAS existed | CI, twice, same test | timing our machines never produce |
| `pre-commit` allow envelope reports a `base_tree` that is not the receipt's base — but only for correction receipts | since corrections existed | community (@decode2), black-box fixture | the composition correction→publication, which no journey ever chained |
| Stranded successor: named transition runs, exits 0, changes nothing; the real exit (`abandon`) named nowhere | since recovery existed | SDD bench axis (j39) | a journey following the product's own advice to its end |
| Lens transport contamination: two lenses judged pseudocode as the candidate; recovery existed (`reopen-results`, with a `--prepare` emitter) and no message names it; operator modified working code to escape the gate | since `reopen-results` shipped | maintainer, live session | an agent's plumbing bug visiting a state we never construct |
| Delivery-strategy enums: producer and consumer of the same variable, same file, 100 lines apart, disjoint vocabularies; out-of-domain value already persisted into a real tasks artifact | since the preflight shipped | community (@MarcosArispe) | an orchestration loop in a runtime we don't drive |

Add the repeat offender that was fixed four separate times this branch: **a reason
computed, published into the machine envelope, and discarded on the line a human reads**
(receipt-discovery, pre-push, escalated context, `anomaly_classes`).

## 2. The two root causes

### 2.1 Verification faces inward; the defects live facing outward

Almost everything this codebase verifies is of the form *"does this component do what
its spec says?"* The kill-switch guard was correct — unit-tested, documented, refusing
exactly what it should — and wired to nothing. The consumer vocabulary was correct; the
producer disagreed with it. The denial reason was correct — in the JSON nobody reads.
`reopen-results` is correct — and orphaned. Every one of these passes every inward-facing
test forever, because inward-facing tests pin components, and **these defects live in the
joints**: guard↔call-site, producer↔consumer, envelope↔terminal, operation↔the refusal
that should name it.

The proof is what happened when the verification turned outward. Tests of the form *lift
the command out of the message the product emitted, dispatch it through the real router,
require the block to clear* — plus a benchmark that drives the binary like an operator —
found more real defects in two days than the unit suite found in months. Not because the
unit suite is bad, but because it answers a different question. The question that finds
these defects is: **can an operator in state X reach state Y knowing only what the
product tells them?** Nothing asked it systematically until this branch, and every time
we asked it of a new surface, the backlog fell out immediately.

The one-directional guard is the sharpest illustration.
`TestEveryNamedReviewContinuationIsStructurallyReal` AST-walks refusal strings and proves
every *named* command exists. Nothing walks the opposite direction: **a refusal whose
resolution exists and goes unnamed is invisible to every guard we have.** The deadcode
ratchet cannot catch it either — `reopen-results` is reachable from the dispatcher, so it
is "live" code. Liveness-by-dispatch masks orphanhood-by-guidance. That asymmetry is the
single mechanism behind the abandon token, the reset, the stranded successor, the
reconciliation dead end, and the maintainer's contaminated-lens incident.

### 2.2 Surface grew faster than its connective tissue

This branch's review surface is 21 subcommands, of which **nine are repair or recovery
operations**, over roughly 2,479 production error sites in the CLI and transaction
packages. Every operation landed as a node. Nobody's job was the edges. The
refusal-to-recovery graph was maintained by memory — the person who built
`reopen-results` knew it existed; the person who wrote the `receipt_unrelated` denial did
not, so the denial says `review start`, which resumes or multiplies lineages instead.
Memory does not scale to nine recovery operations, and it especially does not scale
across parallel agents building disjoint nodes at high speed, which is exactly how this
branch was built.

The same accretion produced the vocabulary problem: the failure taxonomy is smaller than
the failure reality. A lost CAS is not "unknown" — the product knew. Three structurally
different store damages share one generic message. Contaminated reviewer *input* has no
name of its own, so the only available verb was `invalidate`, which blames the candidate
— and the system's one-way door then demanded a new candidate identity, which is how a
review tool caused a change to working code. **When reality presents a state outside the
taxonomy, the system reports the nearest named state, which is a lie told with
confidence.**

## 3. Why each detector missed what it missed

An honest accounting — each instrument works exactly where it looks, and each has a
boundary that was never written down:

- **The unit suite** verifies components, not joints (§2.1). It held the whole time.
- **The bench corpus** is black-box, so it can only visit states the product agrees to
  construct, starting from repositories that are minutes old. CLI-unreachable stores,
  histories, platform behavior, and timing are outside it *by construction*. It also
  measured single flows: the correction→publication composition sat between j35 and j06,
  and defects live in compositions — the pairwise space of 48 journeys is ~2,300 cells
  and we sample the diagonal.
- **The AST guards** check that what is said is true, never that what is true gets said
  (§2.1).
- **The deadcode ratchet** catches functions with no callers. It caught a 15-function
  dead parser; it structurally cannot catch a constant (`RDDOperationMutate`), an
  unreachable branch, or a dispatch-reachable orphan.
- **CI** runs Linux. Four macOS defects shipped for exactly that reason; the CAS race was
  found *by* CI because timing is the one axis where CI samples better than we do.
- **Community testers** sample everything we cannot: repositories with history, odd
  platforms, agent runtimes with their own bugs. That is why the pattern *"community
  finds it, we reproduce it"* recurred all branch — they are not better testers; they
  stand in different state space.

The uncomfortable and correct conclusion: **the loop is not failing.** Every axis added
this branch — edge journeys, SDD journeys, the damaged-store plugin — found its backlog
within minutes of existing. The instrument converges one axis at a time. The scandal is
not the instrument; it is that each axis's backlog sat there for months because the axis
didn't exist, and nothing enumerated the missing axes.

## 4. What actually caught things, for the record

The machinery earned real credit this same day: the extended corpus caught the kill
switch's mutation hole; the damaged-store axis caught the `ds05` circular reference
(`reclaim` names `reconcile-authority`, which cannot load the thing it was named for) on
its first run; CI caught the CAS race twice; the vocabulary guard now fails with 12
errors naming all three producer sites if the enums drift again. The pattern holds:
**where the instrument looks, it finds. Nothing it found was subtle once visible.**

## 5. The fixes that change the class

Ordered by leverage. The first converts the dominant defect class from
*found-by-measurement* to *prevented-by-construction*; the rest widen where the
instrument looks.

1. **Close the guard's missing direction: a refusal-to-resolution ratchet.** Enumerate
   every production refusal site (the AST machinery already exists). Each must either
   (a) name a runnable continuation, (b) carry an explicit annotation from the closed
   `by_design` vocabulary — operator-knowledge / world-action / human-authority — stating
   why no command can honestly exist, or (c) fail the check. Freeze today's violations as
   a baseline exactly like the deadcode ratchet, so the number can only shrink. This
   single guard would have caught the abandon token, the reset, `reopen-results`, the
   stranded successor, and the reconciliation dead end *at commit time*.
2. **Seam tests as a first-class category.** The delivery-strategy guard is the template:
   derive both sides of a seam from shipped source and assert the relation
   (subset/equality/parity). Inventory the seams — producer↔consumer vocabularies,
   envelope↔human-surface parity (exists today for exactly one pair; generalize it),
   guard↔call-site wiring, schema↔emitter. Each seam gets one derived test.
3. **Taxonomy ownership.** Treat failure codes as contract surface. Every cause the code
   can distinguish gets a distinct code (the `authority_revision_conflict` fix is the
   template); collapsing two causes into one code requires the same justification as
   moving a pinned digest. Immediate applications: the three store damages sharing one
   message, and a first-class "reviewer input was unusable" state so process-fault stops
   being spelled `invalidate`.
4. **Outward-facing tests as the default for new refusals.** The
   lift-dispatch-require-clear shape is now cheap — helpers exist in
   `e2e/organicruntime`. Make it the expected test for any new refusal, the way TDD is
   already expected for behavior.
5. **Keep adding axes, and keep a written list of the missing ones.** The damaged-store
   plugin is the template for non-black-box axes. Known-missing today: a composition
   axis (chained journeys: correction→publication was the miss), a history axis (stores
   written by older binaries), platform (community covers it; say so), timing (CI covers
   it; say so). An axis list that names what is *not* covered is the difference between
   "zero defects found" and "zero defects found where we looked" — the two claims this
   audit exists to stop conflating.

## 6. What none of this will fix

Honesty about the residue. No ratchet catches a *wrong* named continuation that runs and
does not help — only outward-facing tests do, case by case. No axis makes the bench
sample platforms it does not run on. Transport contamination between an agent and its
reviewers is invisible to admission by construction — any echo derivable from supplied
material is forgeable, so the recovery path is the only defense, which is why fix #1
matters more than any detector. And community testers will keep finding things, because
they will always stand in state space we do not: that is not a gap to close but a channel
to keep fast.

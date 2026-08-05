---
name: systemic-issue-triage
description: "Trigger: new issue, bug report, triage, backlog, issue flood, community report, root cause, dead-end, blocked user. Attack issues by root class, never one-by-one; fixes must shrink the system, not grow it."
license: Apache-2.0
metadata:
  author: "Alan-TheGentleman"
  version: "1.0"
---

# Systemic Issue Triage

## Activation Contract

Use when triaging incoming issues, planning fixes for a backlog, or deciding how to respond to any bug report. The failure mode this skill exists to prevent: fixing symptoms one-by-one until the system becomes over-engineered machinery that grows exponentially and nobody can maintain.

For a full-backlog audit REPORT (dispositions across every open issue/PR), use `backlog-triage`; this skill governs how the resulting fixes are designed, batched, and closed.

## Hard Rules

- Classify by ROOT CLASS before touching code. Every issue lands in exactly one bucket: (A) superseded by an in-flight design change — name the change and the test that proves it; (B) duplicate of a known class — name the canonical tracker; (C) real new bug — assign to a root-cause CLUSTER, never a standalone fix; (D) feature request; (E) unclear — ask the reporter, don't guess.
- Two or more issues sharing a root = ONE fix at the root, closing all of them with named test evidence. N issues never justify N patches.
- Issues close against NAMED TESTS (supersession evidence), never against promises or "should be fixed now".
- The over-engineering test before ANY fix: does it add a state, a verb, a config flag, a gate, or a parallel representation of existing truth? If yes, redesign — the correct fix usually DELETES or RELAXES something (13 states became 5; a dead-end became a one-line exit message).
- A block whose fix is "name the executable exit in the message" is a MESSAGE fix — do not build machinery around it. Every refusal must name a runnable continuation; verify runnability by executing the printed command, not by assuming.
- Blocking budget: humans may be blocked only at genuine consent and terminal decisions. Any fix that introduces a new human block or a state without a self-service exit is rejected at review, not shipped.
- If a fix crosses a systemic-cluster boundary another issue already tracks, STOP and write one unified design for the cluster (owner, error precedence, migration table per call site) before any PR. Duplicating an approved sibling issue's scope is a defect.
- Self-reported "fixed" is a claim: reproduce the original report's exact scenario on a fresh binary before closing.
- Retiring a command or verb: grep its user-facing STRING repo-wide, not just its symbol call graph. In a system where every refusal names an executable exit, deleting a verb orphans every refusal, help text, and sanctioned-exit list that names it — the user is then told to run a command that no longer exists. Recurse: each caller's callers, and any test file whose helpers other retained tests reuse. Expect a design-time inventory to undercount real scope by 2-5x; report the delta rather than trusting the citation.
- A deletion slice's deadcode ratchet may legitimately go net-POSITIVE when the consumer dies before its provider (consumer-first ordering). Report the true number and name the slice that absorbs it; forcing artificial negativity hides the transition.
- An issue's stated MECHANISM is a hypothesis; only its symptom is evidence. Reproduce before implementing. A correct conclusion routinely names the wrong line, one surface away — a test written against the stated mechanism that PASSES on unmodified main means the report is right and the diagnosis is not. Trust your reproduction over the issue text and say so.
- Broken in production but green in tests = suspect the test was TAUGHT to agree. Check whether the commit that introduced the defect also taught the corpus, a test helper, or a fixture to satisfy it; that removes the broken shape from the test surface at the moment it becomes broken. Also check for a test row asserting the defect as intended behavior. Remove it rather than leaving a test that pins a bug.
- Before deriving any plan from a contract document, identify which copy CONSUMERS receive. A docs mirror and a shipped asset drift, and the guard enforcing meaning may point at the mirror while the guard pointing at the shipped copy only counts rows. Verify the row TEXT, not the row's existence.
- A WRONG exit outranks a missing one. A dead end tells the user to stop; advice that does not work sends them in circles blaming themselves. Check each named exit still runs before treating the message as fixed.
- Closing an issue while half of it is still true strands the reporters of the other half. When one thread carries two failure modes, close nothing and comment naming both, what shipped for which, and what the rest are still waiting on.
- Distinguish "the provider lacks this fact" from "lacks it AT THIS LINE". The second is plumbing and costs nothing to thread through; only the first justifies new wire vocabulary, which is a one-way door consumers must implement forever.

## Decision Gates

| Situation | Action |
|---|---|
| 3+ issues, same subsystem | One cluster exploration → one chained fix batch, slices by root not by issue |
| Issue matches an in-flight redesign | Bucket A; close at delivery with the named test; offer the current workaround (e.g. kill switch) meanwhile |
| Fix wants a new state/flag/verb | Redesign: what can be deleted or relaxed instead? |
| Dead-end report (no exit) | Highest severity class; the fix is an exit, not a guard |
| Reporter's value not reproducible from code | Make the message self-diagnosing, then ask the reporter for their exact input |
| Test written against the issue's mechanism passes on unmodified main | The diagnosis is wrong, the report is not. Strengthen the test until it drives the real surface |
| One thread, two failure modes | Comment naming both and what each waits on; close neither |
| A gate ships without the change that satisfies it | Highest severity: the population it blocks cannot even reproduce their other issues |

## Output Contract

Per triage: bucket counts, per-issue table (issue | bucket | root/cluster | evidence ref), urgent flags. Per fix batch: which issues it closes, the named tests proving each closure, and net line delta (deletion-heavy is the goal).

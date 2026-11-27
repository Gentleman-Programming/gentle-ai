## Mandatory Codegraph Instructions

Codegraph comes first: for every show, what, where, plan, refactor, code fix, analysis or investigation that needs symbol, structure or call-path discovery, run a codegraph query or explore BEFORE any read/fd/find/rg/grep search whenever the workspace has a .codegraph index. For every refactor, code fix, analysis or investigation, codegraph also precedes any rg/grep symbol search. rg/grep are for exact-string and filename lookups, or when codegraph fails or the index could not be initialized.

Trust by calling, not by checking: never probe daemon state before the first codegraph call — liveness is unknowable in advance. When the ordering rule needs to know whether a `.codegraph/` index exists, check it with shell commands only (file tools are gitignore-blind there). The codegraph tool answers on its own (daemon, then CLI fallback) and is bounded; only a failed call, or one that returns no results for the question, is the signal to fall back to read/grep/find.

Search-tool gate: codegraph gates the search tools — the session's first fd/find/rg/grep search must come after a codegraph query or explore, and once codegraph has been used for the investigation at hand, fd/find/rg/grep may be used freely for the rest of that investigation (exact-string, filename, or symbol); a new investigation re-arms the gate.

Memory ordering: persistent-memory reads (for example engram mem_context / mem_search) happen strictly after codegraph — codegraph calls of the same step may run in parallel with each other, but every planned codegraph query or explore must have returned before any memory read, and a memory call is never issued in the same parallel block as a codegraph call; then, when memory tools are available, run mem_context and at least one task-scoped mem_search before proceeding further, even when codegraph failed or no index could be initialized.

Post-compaction revalidation: after compaction or workspace changes, recover target, actor access, and preparation status before using graph evidence; history alone authorizes no init/sync.

Index hygiene: keep .codegraph/ out of the change — the runtime maintains the local git exclude (including linked worktrees) and the index ships its own .gitignore; never commit index files or hand-edit the exclude.

Fallback: if codegraph fails, returns no results for the question, or the index could not be initialized, fall back to fd/find/rg/grep directly, and any search that ran without a preceding codegraph call names the skip reason in one line in the final report.

Staleness: current source prevails over conflicting graph output — inspect the relevant stale files directly, or task-relevant source if the whole index is stale; graph output never replaces tests, compilers, linters, or runtime checks.

Anti-redundancy: source returned by the graph counts as inspected; do not repeat reads or searches merely to confirm symbols when the required evidence is already present.

Wrong-project guard: before trusting query/explore -p results for a path whose index you have not verified, run "codegraph status `<path>`" and assert its "Project:" line names that path — codegraph silently answers from the current project (exit 0) when the -p target has no .codegraph index, so the failure fallback never fires and wrong-project hits look identical to correct ones.

Worktrees: codegraph is workspace-scoped; a linked worktree without an index is seeded from the main checkout by the runtime at session start (copy + sync, init as the fallback), so a session never initializes one by hand — it only states in its report when no symbol search was available. Worktree commands run through the CLI with explicit path args: init/index/sync/unlock/status take a positional path and query/explore take -p, so no cd is needed.

Subagents: a subagent that can call codegraph does so itself, and for a worktree dispatch it does so via the CLI path args because session-bound codegraph tools answer from the subagent cwd and not the dispatched worktree; when it cannot, the parent session runs the query BEFORE dispatching the subagent and embeds the findings in the task handoff, and a subagent result that claims codegraph verification must quote the query it ran or cite the parent-provided findings, otherwise the claim is treated as not performed.

Truncation fallback (MCP variant): a middle-cut mcp__codegraph\_\_* result — one carrying "Warning: truncated output" or "…N chars truncated…" — is never worked from: re-run the same query through the native session codegraph tool (daemon path, unaffected by the 20 KB MCP cap) or read the result's [Full output: …] file with offset/limit; use the native tool when the whole report matters, the file when only a slice is missing.

# Gentle AI™ — Agent Skills Index

When working on this project, load the relevant skill(s) BEFORE writing any code.

Naming convention: `gentle-ai-*` skills are repo-specific workflow skills. Unprefixed skills are portable writing or work-unit skills and intentionally keep their canonical names.

## How to Use

1. Check the trigger column to find skills that match your current task
2. Load the skill by reading the SKILL.md file at the listed path
3. Follow ALL patterns and rules from the loaded skill
4. Multiple skills can apply simultaneously

## Skills


| Skill                    | Trigger                                                                                           | Path                                                                                               |
| ------------------------ | ------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| `issue-creation`         | When creating a GitHub issue, reporting a bug, or requesting a feature.                           | [`internal/assets/skills/issue-creation/SKILL.md`](internal/assets/skills/issue-creation/SKILL.md) |
| `gentle-ai-branch-pr`    | When creating a pull request, opening a PR, or preparing changes for review.                      | [`skills/branch-pr/SKILL.md`](skills/branch-pr/SKILL.md)                                           |
| `gentle-ai-chained-pr`   | When a change is too large for one review, or when creating chained/stacked pull requests.        | [`skills/chained-pr/SKILL.md`](skills/chained-pr/SKILL.md)                                         |
| `cognitive-doc-design`   | When writing docs that must reduce cognitive load for readers or reviewers.                       | [`skills/cognitive-doc-design/SKILL.md`](skills/cognitive-doc-design/SKILL.md)                     |
| `comment-writer`         | When drafting human comments, PR feedback, issue replies, or async updates.                       | [`skills/comment-writer/SKILL.md`](skills/comment-writer/SKILL.md)                                 |
| `work-unit-commits`      | When splitting implementation work into deliverable commits or chained PRs.                       | [`skills/work-unit-commits/SKILL.md`](skills/work-unit-commits/SKILL.md)                           |
| `rdd-defect-workflow`    | When RDD defects involve receipts, authority, recovery, delivery gates, or kill switches.         | [`skills/rdd-defect-workflow/SKILL.md`](skills/rdd-defect-workflow/SKILL.md)                       |
| `rdd-advisory-transport` | When changing reviewer transport, adapters, lens prompts/schemas, or transport capability policy. | [`skills/rdd-advisory-transport/SKILL.md`](skills/rdd-advisory-transport/SKILL.md)                 |
| `issue-root-resolution`  | When auditing backlog roots, proposing cluster fixes, or closing resolved/outdated issues.        | [`skills/issue-root-resolution/SKILL.md`](skills/issue-root-resolution/SKILL.md)                   |
| `systemic-issue-triage`  | When triaging issues, bugs, backlogs, root causes, dead ends, or blocked users.                   | [`skills/systemic-issue-triage/SKILL.md`](skills/systemic-issue-triage/SKILL.md)                   |
| `gentle-ai-bench`        | When touching `bench/`, journeys, driven mode, the journey corpus, or bench axes.                 | [`skills/gentle-ai-bench/SKILL.md`](skills/gentle-ai-bench/SKILL.md)                               |

# security-subagent-compat

## Goal
Add `gentle-ai-security` subagent compatibility across supported Gentle AI agent runtimes, including OpenCode background subagents, and document/publication workflow through an issue and reviewable PR slices if the implementation exceeds 400 changed lines.

## Acceptance Criteria
- `gentle-ai-security` is bundled for every supported agent runtime that has subagent/agent asset support.
- OpenCode installs/syncs `gentle-ai-security` alongside `gentle-ai-explore`, `gentle-ai-worker`, and `gentle-ai-verify` where background subagents are enabled.
- The security agent explicitly accepts Gentle AI package context and can be routed by orchestrator instructions for attack-surface/Sec-TDD work.
- Tests verify embedded assets and runtime-specific manifests/config outputs include the security agent.
- A GitHub issue describes the specification and planned work; PR delivery is split if the final authored diff exceeds 400 changed lines.

## Tasks
- [x] Map current subagent asset architecture and identify runtime gaps.
- [x] Create/update issue with specification and implementation plan.
- [x] Add `gentle-ai-security` assets for applicable runtimes.
- [x] Wire catalogs/manifests/config generation so the agent is installed/synced.
- [x] Add or update tests covering asset inclusion and OpenCode compatibility.
- [x] Run focused verification and assess changed-line budget for PR slicing.
- [ ] Deliver a dependent feature-branch chain with scoped checks and issue linkage.

## Evidence
- Branch: `feat/security-subagent-compat`
- Issue: https://github.com/Gentleman-Programming/gentle-ai/issues/5176
- Delivery strategy: feature-branch chain selected by user. Tracker from `main`; OpenCode child targets tracker; native-agent child targets OpenCode child. Each PR stays below 400 changed lines.
- Candidate scope: 404 authored diff lines including this feature document (53 tracked changes, 325 new agent assets, 26 document lines before this update); recount each slice before publishing.
- Commits: pending; record identities and slice boundaries after commit.
- Verification: focused OpenCode install/sync, native agent tests, model picker tests, and `go build ./...` passed; independent verification pending.
- Issue creation: confirmed read-back from target host; `status:approved` absent. User explicitly requested opening PRs regardless; do not claim the issue policy check is passing.

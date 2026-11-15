# Shared ODD references

This directory contains ordinary Markdown references for Organic Driven Development (ODD), Engram memory, skill resolution, optional research, and native review. It is not an invokable skill and has no `SKILL.md`. `provider-defect-report.md` alone keeps the `gentle-ai-provider-defect-report` frontmatter so Gentle Shell and Gentle AI ship the same workflow text; here it is read by path, not invoked.

## Contents

- `odd-orchestrator-sections.md`: shared language and delegated verification sections.
- `engram-convention.md` and `persistence-contract.md`: ODD task recovery and safe memory writes.
- `skill-resolver.md`: exact skill-path handoff for delegated work.
- `research-lifecycle.md`: optional, evidence-backed research.
- `review-ledger-contract.md` and `review-ledger-contract-pi.md`: runtime-specific native RDD review contracts.
- `provider-defect-report.md`: the Gentle AI provider-defect report workflow (consent, privacy, duplicate lookup, uncertainty, resume) that the orchestrator's classification stub loads only when a Gentle AI invocation produced the failure.

Install and sync deploy every embedded reference to each compatibility skill root. They remove only the obsolete `_shared/SKILL.md` marker; they do not delete user-authored files.

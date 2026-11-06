# Optional SDD Diagnostic Report

Use only sections useful for the requested checks. This outline is not a schema or an archive certificate.

- **Scope:** change, implementation inspected, available artifacts, and TDD mode.
- **Observed progress:** completed and unfinished tasks; do not alter their recorded state.
- **Checks:** exact commands, exit codes, relevant output, and skipped/unavailable checks with reasons.
- **Findings:** concrete behavior, requirement or design discrepancies, severity, and supporting evidence. Separate runtime proof from static observations and assumptions.
- **Historical context:** identify prior report claims and their dates/sources; preserve unresolved findings and explain any changed conclusion.
- **Summary:** what was verified, what failed or remains unknown, and recommended next work.

Do not invent output hashes, passing executions, or historical TDD evidence. A missing test or unavailable tool is a limitation, not proof of correctness. Findings are diagnostic; archive may record them without a verification verdict or mandatory remediation cycle. No RDD state is read or required.

When Strict TDD is active, use `strict-tdd-verify.md` to assess available TDD evidence for implemented work without claiming that unobserved RED/GREEN execution occurred.

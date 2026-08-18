# Legacy Skills Tier

This directory holds skills that were demoted from the top-level `skills/`
namespace during the `dev-agents-p0-remediation` change (SPEC-006, Branch B).

## Contract

- **Project-local only.** These skills are not shipped to end users. They are
  not embedded (`internal/assets/skills/**`) and are not installed to
  `~/.claude/skills/`.
- **Indexed as `legacy/*`.** The skill registry namespaces every skill under
  this directory as `legacy/<skill-name>` (e.g. `legacy/branch-pr`,
  `legacy/gentle-ai-collab-perfect`) once the depth-2 registry fix lands.
- **Deliberate, not accidental.** None of the skills under this tier were
  embedded before the move either — moving them here does not remove any
  capability that used to ship to users.
- **Do not re-flag in future audits.** A skill living under `skills/legacy/`
  is expected and intentional. Do not open a remediation item asking to
  "restore" or "re-embed" a skill solely because it lives here; if a legacy
  skill genuinely needs to become a shipped, embedded skill, that is a new
  scope decision, not a bug fix.

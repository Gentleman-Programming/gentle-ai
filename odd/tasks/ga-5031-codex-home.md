# ga-5031 — Codex paths honor CODEX_HOME

Fix Gentleman-Programming/gentle-ai#5031: every derived Codex path hardcodes
`<home>/.codex` except the role-dir resolver, splitting install/sync writes from
what Codex actually reads. One shared resolver (`system.CodexConfigDir`), every
listed site routed through it, behavior for isolated homes preserved.

## Specs

S1. Quote (issue #5031, "What"): "Only `reviewassets.CodexAgentRoleDir` resolves
`CODEX_HOME` today. Every other path this product derives for Codex hardcodes
`<home>/.codex`, so a user who sets `CODEX_HOME` gets a split brain:
`install`/`sync` write the managed files where Codex does not read them."

S2. Quote (issue #5031, "Codex's rule"): "`CODEX_HOME` when it is set and
non-empty, it must exist and be a directory, and it is canonicalized — so a
**relative** value is resolved against the process working directory.
`~/.codex` is used only when the variable is unset."

S3. Quote (issue #5031, call sites, as of report 2026-10-08; line numbers have
shifted on main, doctor.go no longer derives the path):
- `internal/agents/codex/adapter.go`: `Detect`, `GlobalConfigDir`,
  `SystemPromptDir`, `SystemPromptFile`, `SkillsDir`, `MCPConfigPath`
- `internal/system/config_scan.go` (knownAgentConfigDirs "codex" entry)
- `internal/components/engram/inject.go` (writeCodexInstructionFiles)
- `internal/components/uninstall/service.go` (TOML targets + engramOperations,
  4 instruction-file joins; the TOML config path itself flows via
  `adapter.MCPConfigPath` once the adapter is fixed)
- `internal/tui/model.go` (AgentCodex skills case)
- `internal/skillregistry/registry.go` (`GlobalSkillDirs` `.codex/skills`)

S4. Quote (issue #5031, "Suggested shape"): "One resolver, reused... honoured
only when the supplied home is the real user home from `os.UserHomeDir()`,
otherwise `<home>/.codex`... an ambient variable cannot redirect an isolated
test or sandbox `HOME`." Existing implementation to reuse:
`internal/system/codex_paths.go` `CodexConfigDir`/`ValidateCodexHome`
(post-report upstream addition; satisfies S2 including Abs of relative values
and EvalSymlinks canonicalization).

S5. Quote (issue #5031, out of scope): "`internal/skillregistry/registry.go:96`
is `<cwd>/.codex/skills`, project-local by design, so it is not part of this."
Also out: `reviewassets.CodexAgentRoleDir` ("already honours `CODEX_HOME`...
in #5029"), `internal/cli/doctor.go` (site no longer present on main),
`scripts/crosslane/hostcodex.go` and
`internal/components/legacyassets/sdd_runtime_files.go` (root-relative legacy;
not listed by the issue).

## Tasks

| ID | Specs | Route | Commit |
| --- | --- | --- | --- |
| T1 | S1 S2 S4 | Adapter: RED tests for CODEX_HOME override + isolated-home preservation on the 6 path fns and Detect; GREEN by routing through system.CodexConfigDir | fix/5031-codex-home |
| T2 | S1 S2 S4 | config_scan: codex entry resolves through CodexConfigDir; test-first in internal/system | same branch |
| T3 | S1 S3 | engram inject writes + uninstall removes under the resolved Codex root; symmetry tests | same branch |
| T4 | S1 S3 | skillregistry GlobalSkillDirs + tui AgentCodex skills resolve through the helper; tests | same branch |
| T5 | S1-S5 | Full affected suites + gofmt + vet; scope check against S5; final Log update | same branch |

## Log

L1 (2026-10-14, user): "dale al engram#1693" → engram#1693 descartado por
deep-read (tomado de facto por BraisonCrece/MataM15/doblas); usuario eligió
"Sí, claim #5031" del radar verificado. Claim posted:
https://github.com/Gentleman-Programming/gentle-ai/issues/5031#issuecomment-6178945396
(reporter dev-addous acreditado, Refs-only #5429 excluido, stand-down ofrecido).

L2 (2026-10-14, user): "si" — aprobado iniciar implementación ODD: feature doc,
branch `fix/5031-codex-home` desde upstream/main (e37030bbf), test-first por
tarea. Worktree: ~/gentleman/gentle-ai-5031.

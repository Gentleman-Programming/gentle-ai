# SDD Spec: Multi-Repo Status & Registry Parsing

## Scenarios

**Scenario 1: Parsing the Markdown Registry**
- **Given** `docs/repository-registry.md` containing a valid markdown table.
- **When** `repository.ParseRegistry` is invoked.
- **Then** it returns a map of `slug` -> `Repository` with exact values from the table.

**Scenario 2: Populating Status**
- **Given** a `tasks.md` declaring `repository: gp-apps-cross/Pagos`.
- **When** `status.go` computes the status.
- **Then** `Status.TargetRepositories` contains exactly 1 entry with `GitlabPath` = `gp-apps-cross/Pagos` and `Slug` = `gp-apps-cross-pagos`.

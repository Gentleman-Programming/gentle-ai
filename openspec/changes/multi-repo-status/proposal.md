# SDD Proposal: Multi-Repo Status & Registry Parsing

## Approach
1. Build a parser for `docs/repository-registry.md` inside `internal/repository`.
2. Extract the `Repository` struct with `GitlabPath`, `Slug`, `Owner`, `Type`, `Purpose`, `Profile`.
3. In `internal/sddstatus`, augment `Status` with `TargetRepositories []repository.Repository`.
4. During `baseStatus` construction, use the slugs extracted from `tasks.md` to look up and populate the `TargetRepositories`.

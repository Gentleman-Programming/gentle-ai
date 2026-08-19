# SDD Tasks: Multi-Repo Status & Registry Parsing

- [ ] 1. Create `internal/repository/registry.go` with `ParseRegistry(path string) map[string]Repository`.
- [ ] 2. Create `internal/repository/registry_test.go` to assert the markdown table is parsed correctly.
- [ ] 3. Update `internal/sddstatus/status.go` to include `TargetRepositories []repository.Repository` in `Status`.
- [ ] 4. Update `baseStatus()` to populate `TargetRepositories` by mapping `declaredRepoSlugs` against the parsed registry.
- [ ] 5. Write a test in `internal/sddstatus/status_test.go` confirming `TargetRepositories` is correctly resolved.

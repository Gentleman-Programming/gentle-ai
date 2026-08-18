# SDD Explore: Multi-Repo Status & Registry Parsing

## Discovery
Currently, `dev-orchestrator`'s `Status` struct understands multiple repos via `RepoProgress`, but it lacks the critical link to the **GitLab paths**. The subagents shouldn't rely on local filesystem clones (as clarified by the user: "deberás usar el mcp de gitlab"). Therefore, the orchestrator needs to resolve `repo-slug` -> `gitlab_path` using `docs/repository-registry.md`.

## Implications
By equipping `Status` with `TargetRepositories`, the orchestrator will be able to pass `gitlab_path` to subagents, enabling native GitLab MCP integration without local clones.

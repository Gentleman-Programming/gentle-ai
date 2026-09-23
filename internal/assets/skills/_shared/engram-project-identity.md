# Engram Project Identity Contract

The active workspace and the logical Engram project are separate identities.

1. Resolve the **active workspace** agent-side with `git rev-parse --show-toplevel`; if it fails, use the existing workspace fallback. Use that returned path for every filesystem read, write, and native command `--cwd`. Never redirect file operations to another checkout because of an Engram project name.
2. Before the first Engram read or write in a session, when `mem_current_project` is callable, call it once and cache its returned canonical `project`. Use that exact cached value for every explicit Engram `project` argument and every project-scoped topic, including `sdd-init/{project}`. Forward the cached value unchanged to phase agents.
3. Never derive the logical Engram project from the active workspace basename. A phase agent consumes the cached canonical project supplied by its orchestrator; it does not rediscover or rename it.
4. If `mem_current_project` is unavailable, reuse an already-supplied canonical project only. Do not invent an explicit project for Engram persistence. When explicit project-scoped persistence is required and no canonical project is available, fail closed and report the unavailable identity.
5. Native status resolution keeps its established fallback order: `ENGRAM_PROJECT`, then the active workspace's Git remote (including the shared Git configuration for linked worktrees), then the basename only for a non-Git workspace. Agents must not reimplement or weaken that resolver.

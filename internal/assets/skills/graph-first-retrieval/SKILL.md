---
name: graph-first-retrieval
description: "Trigger: sdd-explore, sdd-apply, sdd-design, sdd-verify when graphify-out/graph.json exists. Query the knowledge graph before reading files to minimize token waste."
disable-model-invocation: true
user-invocable: false
license: MIT
metadata:
  author: gentleman-programming
  version: "1.0"
  delegate_only: false
---

## Graph-First Retrieval Protocol

Load this skill when:
- The project has `graphify-out/graph.json`
- You are an SDD sub-agent that needs to understand the codebase
- Phases: `sdd-explore`, `sdd-apply`, `sdd-design`, `sdd-verify`

This protocol runs BEFORE you start reading implementation files.

## Detection

Check if `graphify-out/graph.json` exists relative to the project root (cwd)
AND `graphify` CLI is available on PATH (e.g. `command -v graphify`).
If either check fails, skip this entire protocol and read files normally.

## Instructions

### 1. Formulate a Graph Query

Replace `<question>` with a question about what you need to understand:

| Situation | Command |
|---|---|
| Understand what files touch a concept | `graphify query "<topic>" --budget 600` |
| Find the shortest path between two concepts | `graphify path "<A>" "<B>"` |
| Get a plain-language explanation of a node | `graphify explain "<node-label>"` |

Examples:
- `graphify query "auth middleware, login session handling" --budget 600`
- `graphify path "UserRepository" "NotificationService"`
- `graphify explain "DatabaseModule"`

### 2. Interpret the Subgraph

The result contains:
- `nodes` — entities in the codebase (files, modules, concepts)
- `edges` — relationships (imports, calls, extends, implements)
- `source_location` — exact file path for each node

From this, build a list of files to read.

### 3. Read What the Graph Signals

The graph signals the priority files. Start by reading those.

If implementation evidence requires additional files (configuration, tests,
contracts, dependencies), expand the reading scope as needed. The graph is
a starting point, not a complete allowlist.

If the graph returns nothing useful, or `graphify` CLI is unavailable, fall back
to normal file reading — the graph is a supplement, not a replacement.

### 4. After Editing Code

If you modified files, run:
```bash
graphify update .
```

This is AST-only, zero API cost. If `graphify` CLI is unavailable, skip silently.

## Hard Rules

- Always try the graph FIRST before reading files
- If the graph returns a valid subgraph, prioritize those files but expand when
  implementation evidence requires configuration, tests, or other dependencies
- If the graph doesn't exist or the query fails, work normally (read files)
- `graphify update .` runs only after EDITS, not in read-only phases
- Never use the graph as the sole source of truth — always verify by reading
  the files it signals
- The fallback is silent — no warnings or errors if graph is unavailable

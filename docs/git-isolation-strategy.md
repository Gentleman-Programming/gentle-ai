# Git Isolation Strategy

This document defines how agents MUST interact with Git to prevent collisions.

## Core Rules
1. **No direct commits to `main`**: All work MUST occur on branches.
2. **Change Branches**: A logical feature/fix gets a change branch, e.g., `change/PAY-125`.
3. **Task Branches (Agent Isolation)**: Because multiple agents might work on the same change, they MUST use task-specific worktrees/branches, e.g., `task/PAY-125-01`.
4. **Integration**: The `dev-orchestrator` is responsible for verifying dependencies and merging `task/*` branches into `change/*` before opening a Merge Request.

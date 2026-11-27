#!/usr/bin/env bash
# post_merge_rebase.sh - Rebase all chain worktrees after #6252 merge
# Run this AFTER PR #6252 is merged to upstream/main

set -euo pipefail

WORKTREE_ROOT="/run/media/bladimir/Datos/Datos/proyectos/work/gentleman"
MAIN_REPO="$WORKTREE_ROOT/gentle-ai"
SYNC_FORK="$WORKTREE_ROOT/gentle-ai-sync-fork"

CHAINS=(
    "gentle-ai-chain1-antigravity:chain1-antigravity:Antigravity + Engram + Gemini"
    "gentle-ai-chain2-rdd:chain2-rdd:RDD/Review system"
    "gentle-ai-chain3-shell:chain3-shell:Shell installer Win/macOS/Linux"
    "gentle-ai-chain4-guidance:chain4-guidance:Guidance/Orchestrator/Verifier"
    "gentle-ai-chain5-codex:chain5-codex:Codex/OpenCode integration"
    "gentle-ai-chain6-verif:chain6-verif:Verification/Verifier method"
    "gentle-ai-chain7-misc:chain7-misc:Shell/TUI + CI/Release + Misc"
)

echo "=== Post-Merge Rebase Script ==="
echo "Main repo: $MAIN_REPO"
echo "Sync fork: $SYNC_FORK"
echo ""

# Step 1: Verify #6252 is merged
echo "Step 1: Verify #6252 is merged..."
MERGED=$(gh pr view 6252 --repo Gentleman-Programming/gentle-ai --json state --jq '.state')
if [[ "$MERGED" != "MERGED" ]]; then
    echo "PR #6252 is not merged yet (state: $MERGED). Aborting."
    exit 1
fi
echo "PR #6252 is merged"
echo ""

# Step 2: Fetch upstream and rebase sync-fork-all
echo "Step 2: Rebase sync-fork-all onto upstream/main..."
cd "$SYNC_FORK"
git fetch upstream
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [[ "$CURRENT_BRANCH" != "sync-fork-all" ]]; then
    echo "HEAD in $SYNC_FORK is on branch '$CURRENT_BRANCH', expected 'sync-fork-all'. Aborting rebase."
    exit 1
fi
BEFORE_TIP=$(git rev-parse origin/sync-fork-all)
git rebase upstream/main
git push --force-with-lease origin sync-fork-all
echo "sync-fork-all rebased and pushed"
echo ""

# Step 3: Rebase each chain worktree
echo "Step 3: Rebase chain worktrees..."
for entry in "${CHAINS[@]}"; do
    IFS=':' read -r wt_dir branch_name description <<< "$entry"
    wt_path="$WORKTREE_ROOT/$wt_dir"
    
    if [[ -d "$wt_path" ]]; then
        echo "  Rebasing $wt_dir ($description)..."
        cd "$wt_path"
        git fetch upstream
        WT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
        if [[ "$WT_BRANCH" != "$branch_name" ]]; then
            echo "  $wt_dir: HEAD is on branch '$WT_BRANCH', expected '$branch_name'. Skipping rebase."
            continue
        fi
        git rebase sync-fork-all
        git push --force-with-lease origin "$branch_name"
        echo "  $wt_dir rebased and pushed"
    else
        echo "  $wt_dir not found, skipping"
    fi
done
echo ""

# Step 4: Show status
echo "Step 4: Final status..."
cd "$MAIN_REPO"
git worktree list
echo ""
echo "=== Done ==="
echo "All chains rebased onto updated sync-fork-all (which is now on v5 upstream/main)"
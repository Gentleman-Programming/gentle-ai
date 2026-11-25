#!/usr/bin/env bash
# monitor_prs.sh - Monitor PR status for sync-fork workflow
# Usage: ./monitor_prs.sh [PR_NUMBER]

set -euo pipefail

PRS=(6252 6267 6268 6264 6265 6266 6269 6270)
REPO="Gentleman-Programming/gentle-ai"

echo "=== PR Status Monitor ==="
echo "Repo: $REPO"
echo "Date: $(date)"
echo ""

for pr in "${PRS[@]}"; do
    echo "--- PR #$pr ---"
    gh pr view "$pr" --repo "$REPO" --json number,title,state,mergeable,reviews,headRefName,baseRefName --jq '
        {
            number,
            title,
            state,
            mergeable,
            headRefName,
            baseRefName,
            coderabbit_reviews: [.reviews[] | select(.author.login=="coderabbitai") | {state: .state, submittedAt: .submittedAt, actionable: (.body | test("Actionable comments posted: [1-9]"))}],
            human_reviews: [.reviews[] | select(.author.login!="coderabbitai") | {author: .author.login, state: .state}]
        }'
    echo ""
done

# Check if #6252 is mergeable and has type:chore label
echo "=== Critical Path Check ==="
PR6252=$(gh pr view 6252 --repo "$REPO" --json mergeable,labels,state --jq '{mergeable, labels: [.labels[].name], state}')
echo "PR #6252: $PR6252"
echo ""

if echo "$PR6252" | grep -q '"type:chore"'; then
    echo "PR #6252 has type:chore label - ready for merge"
else
    echo "PR #6252 missing type:chore label - NEEDS MAINTAINER ACTION"
fi
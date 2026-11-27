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

# Check if #6252 is mergeable, open, and has the type:chore label
echo "=== Critical Path Check ==="
PR6252=$(gh pr view 6252 --repo "$REPO" --json mergeable,labels,state)
MERGEABLE=$(echo "$PR6252" | jq -r '.mergeable')
STATE=$(echo "$PR6252" | jq -r '.state')
LABELS=$(echo "$PR6252" | jq -r '.labels[].name')
echo "PR #6252: mergeable=$MERGEABLE state=$STATE labels=$LABELS"
echo ""

if echo "$LABELS" | grep -qx 'type:chore' && [[ "$MERGEABLE" == "MERGEABLE" && "$STATE" == "OPEN" ]]; then
    echo "PR #6252 has type:chore label and is mergeable/open - merge readiness checks satisfied"
else
    echo "PR #6252 missing type:chore label or not merge-ready - NEEDS MAINTAINER ACTION"
fi
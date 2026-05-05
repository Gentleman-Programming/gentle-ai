<!-- ⚠️ READ BEFORE SUBMITTING
  Every PR must be linked to an issue that has the "status:approved" label.
  PRs without a linked approved issue will be automatically rejected by CI.
  See CONTRIBUTING.md for the full contribution workflow.
-->

## 🔗 Linked Issue

Closes #

<!-- Replace the # above with the issue number, e.g.: Closes #42 -->

---

## 🏷️ PR Type

What kind of change does this PR introduce?

- [ ] `type:bug` — Bug fix (non-breaking change that fixes an issue)
- [ ] `type:feature` — New feature (non-breaking change that adds functionality)
- [ ] `type:docs` — Documentation only
- [ ] `type:refactor` — Code refactoring (no functional changes)
- [ ] `type:chore` — Build, CI, or tooling changes
- [ ] `type:breaking-change` — Breaking change (fix or feature that changes existing behavior)

---

## 📝 Summary

<!-- Provide a clear and concise description of what this PR does and why. -->

---

## 📂 Changes

| File / Area | What Changed |
|-------------|-------------|
| `path/to/file` | Brief description |

---

## 🧪 Test Plan

**Unit Tests**
```bash
go test ./...
```

**E2E Tests** (Docker required)
```bash
cd e2e && ./docker-test.sh
```

- [ ] Unit tests pass (`go test ./...`)
- [ ] E2E tests pass (`cd e2e && ./docker-test.sh`)
- [ ] Manually tested locally

<!-- Describe any additional manual testing steps if needed. -->

---

## 🤖 Automated Checks

The following checks run automatically on this PR:

| Check | Status | Description |
|-------|--------|-------------|
| Check PR Cognitive Load | ⏳ | PR should stay within 400 changed lines (`additions + deletions`) or use `size:exception` |
| Check Issue Reference | ⏳ | PR body must contain `Closes/Fixes/Resolves #N` |
| Check Issue Has `status:approved` | ⏳ | Linked issue must have been approved before work began |
| Check PR Has `type:*` Label | ⏳ | Exactly one `type:*` label must be applied |
| Unit Tests | ⏳ | `go test ./...` must pass |
| E2E Tests | ⏳ | `cd e2e && ./docker-test.sh` must pass |

---

## ✅ Contributor Checklist

- [ ] PR is linked to an issue with `status:approved`
- [ ] PR stays within 400 changed lines, or I have requested/obtained maintainer-applied `size:exception` with rationale documented
- [ ] I have added the appropriate `type:*` label to this PR
- [ ] Unit tests pass (`go test ./...`)
- [ ] E2E tests pass (`cd e2e && ./docker-test.sh`)
- [ ] I have updated documentation if necessary
- [ ] My commits follow [Conventional Commits](https://www.conventionalcommits.org/) format
- [ ] My commits do not include `Co-Authored-By` trailers

---

## 💬 Notes for Reviewers

<!-- Optional: anything you want reviewers to pay special attention to. -->

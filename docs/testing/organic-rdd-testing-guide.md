# 🧪 How to test — Organic RDD (prerelease 2.2.0-rc.1)

> [!IMPORTANT]
> **Current closure and delivery semantics.** A zero-lens START, or the final admitted lens, refuter, or validation capture, closes and burns a review. Delivery always follows ordinary repository policy; review closure is informational.

> [!WARNING]
> **Historical and superseded guide.** This document preserves the candidate-specific validation procedure for `v2.2.0-rc.1` and PR [#1801](https://github.com/Gentleman-Programming/gentle-ai/pull/1801). It is not current installation or validation guidance for stable [`v2.3.0`](https://github.com/Gentleman-Programming/gentle-ai/releases/tag/v2.3.0), prerelease [`v2.4.0-rc.1`](https://github.com/Gentleman-Programming/gentle-ai/releases/tag/v2.4.0-rc.1), or unreleased `main`. Use the [Quickstart version policy](../quickstart.md#version-policy) for current installation channels and validation entry points.
>
> Community testing guide for the candidate built from PR [#1801](https://github.com/Gentleman-Programming/gentle-ai/pull/1801). Every **Expected** here was validated against real output before publication. The guide uses a throwaway HOME precisely so it does not touch your real config — do not skip the setup.

> [!IMPORTANT]
> **This guide moves; a published asset does not.** It tracks the PR head and describes behaviour that may have landed *after* the binary you downloaded was built. Running it literally against an older asset produces false regressions — that is the guide's fault, not the product's.
>
> Before you start, write down which candidate you are testing:
>
> ```
> gentle-ai --version          # or "$RC_BIN" --version
> ```
>
> **Put that string in your report.** If a step disagrees with what you see, the first question is always whether the guide is describing a newer commit than your binary. Say which one you ran and we can tell the difference; without it we cannot.

## How to get this binary

The binaries are on the prerelease page: **https://github.com/Gentleman-Programming/gentle-ai/releases/tag/v2.2.0-rc.1**

1. Download the asset for your platform from the Assets section of that page.
2. Verify the checksum against `SHA256SUMS.txt`:
   ```
   sha256sum -c SHA256SUMS.txt --ignore-missing
   ```
3. Make it runnable and confirm which build you have. **You do not need gentle-ai installed already** — this works on a clean machine:
   ```
   RC_BIN="$(pwd)/gentle-ai_2.2.0-rc.1_<os>_<arch>"
   chmod +x "$RC_BIN"
   "$RC_BIN" --version
   ```
4. Either invoke `"$RC_BIN"` explicitly for every step, or put it on your PATH under a throwaway directory:
   ```
   mkdir -p /tmp/rdd-bin && ln -sf "$RC_BIN" /tmp/rdd-bin/gentle-ai
   export PATH="/tmp/rdd-bin:$PATH"
   ```
5. **Only if you already had gentle-ai installed** and want to replace it, back the old one up first:
   ```
   command -v gentle-ai && cp "$(command -v gentle-ai)" ~/gentle-ai.backup
   ```
   Roll back with `mv ~/gentle-ai.backup "$(command -v gentle-ai)"` when you are done.

## Setup (once)

1. Create a test HOME so you do not touch your real config:
   ```
   export TESTHOME=$(mktemp -d) && export HOME=$TESTHOME
   ```
2. Create a test repo (the `.gitignore` keeps the installed config out of the diffs):
   ```
   mkdir -p $HOME/demo && cd $HOME/demo && git init -b main && git config user.email t@t && git config user.name T && echo ".claude/" > .gitignore && echo hello > README.md && git add -A && git commit -m "init"
   ```

## Steps to test

### Flow 1: Routing without SDD (the main fix)

1. [ ] `gentle-ai install --scope workspace --agents claude-code --components permissions` → **Expected**: it installs and ends with "You're ready", without asking anything about SDD.
2. [ ] Open `$HOME/demo/.claude/CLAUDE.md` → **Expected**: a routing section with **direct inline**, **delegated direct** and **optional SDD**.
3. [ ] Search for `WorkRun` or `work-capabilities` → **Expected**: **zero results**. If it shows up, that is a bug.
4. [ ] Search for `review mode` → **Expected**: `gentle-ai review mode enable|disable|status` shows up.
5. [ ] Run the same install again → **Expected**: same output and the files do NOT change.

### Flow 2: Kill switch

1. [ ] `gentle-ai review mode status --cwd $HOME/demo --json` → **Expected**: effective `off`, source `default` — receipt-driven development is opt-in, so a fresh install reviews nothing until you ask it to.
2. [ ] `gentle-ai review start --cwd $HOME/demo` → **Expected**: refused, naming that reviews are off **and naming the command that turns them on**:

```
receipt-driven development is disabled: start is rejected because the default mode source
keeps it off; turn reviews on with gentle-ai review mode enable --scope=global
```

It does NOT hang, it does NOT review. A refusal that exits non-zero and names no command is the defect.
3. [ ] `gentle-ai review mode enable --scope global --cwd $HOME/demo` then `status` → **Expected**: effective `on`, source `global`.
4. [ ] `gentle-ai review mode disable --cwd $HOME/demo` → **Expected**: it confirms reviews are off.
5. [ ] `status` again → **Expected**: effective `off`, source `global` (an explicit off, not the default).
6. [ ] `gentle-ai review start --cwd $HOME/demo` → **Expected**: the same shape of refusal, naming commands that actually reach `on` from where you are. If you turned it off at clone scope, the message must name `--scope=global` **then** `--scope=clone`: clearing the clone override alone drops you on the opt-in default, still off.
7. [ ] `enable --scope global` and `status` → **Expected**: `on` again.
8. [ ] `disable --scope clone`, clone (`git clone $HOME/demo $HOME/demo2`) and `status` in `demo2` → **Expected**: `demo2` gives **on** (the global enable still applies) — turning a clone off is NOT inherited.
7. [ ] **Before moving on**: `enable --scope clone` in `demo` → **Expected**: `on`.

### Flow 3: Documentation-only change (zero ceremony)

1. [ ] Edit `README.md` (plain text) and stage **only that file**: `git add README.md`.
2. [ ] `gentle-ai review start --cwd $HOME/demo` → **Expected**: `risk_level: low`, `selected_lenses: []` — zero reviewers, no question; START closes and burns the review.

### Current review lifecycle (use for every flow below)

1. Start the review. A zero-lens START closes and burns it immediately. For selected lenses, use only the exact STATUS-issued capture route for each admitted result.
2. If STATUS requests a correction, use its bound `capture-correction-plan`. After the bounded correction, use the terminal STATUS-issued `capture-validation`; its admitted capture closes and burns the review.
3. If capture output is malformed, incomplete, unavailable, or ambiguous, query STATUS for the same lineage. STATUS reconciles the outcome; relaunch only when it reoffers the identical bound slot.
4. Commit, push, PR, release, and archive actions follow ordinary unmanaged repository policy. Do not run a separate delivery step after review closure.

### Flow 4: The review is chosen by evidence, not by size

1. [ ] `mkdir -p internal/auth && echo "func CheckToken() {}" > internal/auth/session.go`, `git add internal/auth`.
2. [ ] `review start` → **Expected**: `risk_level: high`, 4 lenses, and `risk_evidence` naming the reason (e.g. `"authentication in internal/auth/session.go"`).
3. [ ] Commit that (`git commit -am "auth"`). Generate 1000+ lines of text across several `.md` files, `git add *.md`, `review start` → **Expected**: `low`, 0 lenses. It does NOT escalate on size.

### Flow 5: The consent question (needs a real terminal)

1. [ ] With a tier 1/2 change ready, `review start` in an interactive terminal → **Expected**: **two** options — `1) Run the review now` / `2) Not now, just this once` — and a final line naming `gentle-ai review mode disable`. **There is no option 3.**
2. [ ] Answer `2` → **Expected**: it does not review this candidate.
3. [ ] ANOTHER change and `review start` → **Expected**: it asks again.
4. [ ] Answer `1` → **Expected**: it reviews, and the next change no longer asks.

**If you are driving this from a script or an agent**: the answer is read as one whole line, so it must end with a newline. Sending the bare character `2` over a pseudo-terminal is echoed but never completes the read, and the command waits until your harness kills it. Send `2\n`. There is no timeout on this prompt, so a missing newline looks exactly like a hang.

### Flow 6: Delivery stays ordinary unmanaged

**Watch out for the fixture**: this flow needs a configured upstream so the push is real. Set one up first:

```
git init --bare $HOME/demo-remote.git
cd $HOME/demo && git remote add origin $HOME/demo-remote.git
git push -u origin HEAD
```

1. [ ] Turn reviews off, make a change, and commit → **Expected**: the commit works normally.
2. [ ] Push the commit → **Expected**: ordinary repository policy decides delivery; no review lifecycle command is part of the push.
3. [ ] Turn reviews back on, make and commit another docs change, then push → **Expected**: delivery remains ordinary unmanaged policy.

### Flow 7: Turning it off mid-work and coming back

1. [ ] With reviews on, a change **staged but not committed**. Turn reviews off → **Expected**: everything flows.
2. [ ] Turn them on and `review start` → **Expected**: it works — it freezes and reviews from scratch. Nothing is lost. (If you already committed, the result carries a `hint` with `--base-ref`.)

### Flow 8: No phantom SDD artifacts

1. [ ] `git rev-parse --git-common-dir` and look inside → **Expected**: inside `gentle-ai/` only review state; nothing like `sdd*`, `trace`, `evaluation`.

---

## Flows 9 to 13: what we fixed with your feedback

These flows are new. Each one reproduces a bug someone in the community found in earlier rounds. They need a binary **later than Refresh 4**. Check which build you have with `gentle-ai doctor`: it names the binary you actually invoked and its version, and warns when that differs from the one on your `PATH`. If yours predates the current refresh, download it again from the release page or build from the PR branch.

### Flow 9: Published commits stay ordinary delivery

Reported by @Wladimirfn, @Denver2828, @MarsSall and @Freedom2828. The old candidate procedure tried to make a previously published commit re-enter review delivery. Current delivery never does that.

You need the remote from Flow 6.

1. [ ] Make a docs change and run `review start` → **Expected**: its zero-lens START closes and burns the review.
2. [ ] Commit and push the change.
3. [ ] Turn reviews off, make and commit another docs change, then push it.
4. [ ] Turn reviews on and make a third docs change → **Expected**: a new START concerns only this new candidate; previously closed reviews and published commits do not govern ordinary delivery.

### Flow 10: First commit in a repo with no history

Reported by @lu149e, with the root cause confirmed by @Denver2828.

1. [ ] `mkdir $HOME/unborn && cd $HOME/unborn && git init -b main`.
2. [ ] Create a code file, `gofmt` if it applies, and `git add -A`. **Do not commit yet.**
3. [ ] `git rev-parse --verify HEAD` → **Expected**: it fails, because there is no first commit yet. That is correct.
4. [ ] `gentle-ai review start --cwd "$PWD"` → **Expected**: the review **starts**. It used to blow up with `Needed a single revision`.

### Flow 11: STATUS transitions run exactly as returned

This one is for people using agents. A controller must not infer a lifecycle action from status prose; it uses the returned transition exactly.

1. [ ] With a review in progress, ask for the next transition:

```
gentle-ai review status --next-transition --contract gentle-ai.review-integration/v2
```

2. [ ] First read `next_transition.kind`. If it is `execute`, run the returned operation with its ordered argument tokens exactly as returned → **Expected**: the transition runs without reordered, synthesized, or added arguments.
3. [ ] If it is `collect`, inspect `inputs[].arguments` → **Expected**: each carries a complete token for the bound capture. A model must first produce that bound reviewer result; do not invent an execute action.
4. [ ] Submit a completed result only through the returned collect input. Do **not** add `--cwd`: inputs that carry `--repository-context` refuse both contexts together.
5. [ ] After any malformed, incomplete, unavailable, or ambiguous capture result, query STATUS again. Relaunch only if it reoffers the same bound slot; otherwise follow the returned state.

### Flow 12: Final capture closes the review

1. [ ] Start a review that selects lenses and follow only the STATUS-issued `capture-result` route for each selected lens.
2. [ ] Submit the final admitted capture → **Expected**: the final reviewer, refuter, or targeted-validator capture closes the review and burns its authority. No additional close or retry command follows it; delivery remains ordinary unmanaged policy.

If a capture is malformed, incomplete, or unavailable, query the same exact-lineage STATUS and relaunch only when it reoffers the same bound slot.

### Flow 13: Flag combinations we do not support

1. [ ] `review start --projection staged --base-ref HEAD~1` → **Expected**: a typed rejection naming **both escapes**: `--projection staged` alone (to review the index) or `--base-ref <ref> --committed-only` (to review base-diff). It does not guess which one you meant.

---

## Flows 14 to 19: organic DX

These flows test the new work: that the tool recovers on its own, that blocks say how to continue, and that old history does not constrain current work. They need a binary **later than Refresh 4**.

### Flow 14: Closed reviews do not block new work

The bug @decode2 and @fisidj found. You need the remote from Flow 6.

1. [ ] Make **three** docs changes. For each one, run `review start`, confirm that the zero-lens START closes, then commit. Push the first two.
2. [ ] Turn reviews off and make a fourth commit **without reviewing it**, then push it.
3. [ ] Turn reviews on and run `review status --contract gentle-ai.review-integration/v2` → **Expected**: STATUS does not make any prior closed review govern the fourth commit or its delivery.

### Flow 15: Corrections use the STATUS-bound plan

1. [ ] Reach a correction state and query STATUS for the review lineage.
2. [ ] Use only the bound `capture-correction-plan` STATUS returns → **Expected**: it identifies the admitted correction scope.
3. [ ] After the bounded correction, query STATUS again and use the terminal `capture-validation` it returns → **Expected**: the admitted validation capture closes and burns the review.
4. [ ] If either capture result is ambiguous, query STATUS again before doing anything else; it reconciles closure or reoffers the same bound slot.

### Flow 16: STATUS says which capture comes next

1. [ ] With a selected-lens review waiting for results, run `review status --next-transition --contract gentle-ai.review-integration/v2`.
2. [ ] Read the returned collect input → **Expected**: it identifies the exact bound capture to submit; do not assemble or substitute a different route.
3. [ ] Submit the result only through that route, then query STATUS → **Expected**: STATUS identifies the next capture or the closed state.

### Flow 17: Visible numbers when a correction escalates

1. [ ] Push a correction past its line budget → **Expected**: the message says **spent, remaining and total** with distinct labels. It used to escalate with a number that was not printed anywhere.

### Flow 18: Ambiguous capture output is reconciled by STATUS

1. [ ] Start a selected-lens review and retain the exact STATUS-issued capture input.
2. [ ] Produce a malformed, incomplete, unavailable, or otherwise ambiguous capture outcome.
3. [ ] Query STATUS for that same lineage → **Expected**: it reconciles any admitted outcome. It reoffers a capture only when the identical bound slot is still open.
4. [ ] Follow the reoffered route only when the lineage, target, revision, subject, lens, and order all match the original input. Otherwise stop and report the mismatch with the STATUS output.

### Flow 19: First-run hygiene

1. [ ] `install --agents opencode` (OpenCode only) → **Expected**: the last line names only OpenCode, not "run claude".
2. [ ] `doctor` running the binary **by absolute path** → **Expected**: it reports the binary you ran with its version, and warns if it differs from the one on the PATH.
3. [ ] `review start --committed-only true` (with a space) → **Expected**: the error explains that a boolean flag is passed as `--flag` or `--flag=true`, never with a separate value.

---

## Flows 20 to 23: macOS only

**Why these exist.** CI runs unit tests on Ubuntu and has a native lane for Windows. It has none for Darwin, and @edwinsaavedran showed in #1853 that four macOS defects reached a release through that hole: `/var` path aliasing (#1773), `EPERM` under managed profiles (#1781), reviewer-result publication on ExFAT (#1804), and first-use store contention (#1850).

Cross-compiling with `GOOS=darwin` proves the code builds. It proves nothing about APFS, temp-directory aliases, Darwin advisory locks, or real `git` path output. Only a Mac can answer these, which is why they are here and not in CI.

**Run these on macOS only.** On Linux or Windows, mark them N/A and move on.

### Flow 20: The `/var` alias (#1773)

macOS puts `$TMPDIR` under `/var/folders/...`, and `/var` is a symlink to `/private/var`. The same repository therefore has two valid absolute paths, and authority bound to one must be found from the other.

1. [ ] `cd "$(mktemp -d)"` and set up a throwaway repo there → note the path `git rev-parse --show-toplevel` prints.
2. [ ] Make a change **and stage it**, then run the full cycle:

```
echo "one more line" >> guide.md
git add guide.md
gentle-ai review start --cwd .
# For this docs-only low-risk case, START closes and burns the review.
```

→ **Expected**: the zero-lens START closes and burns the review. No "no discoverable review lineage" or path-shaped error.

**The `git add` is not optional and it is not tidiness.** START with the default workspace projection freezes your uncommitted change. Staging keeps the snapshot stable while this flow verifies that `/var` and `/private/var` resolve to the same review state. Reported by @edwinsaavedran after the old path handling produced a false signal.
3. [ ] Now `cd` into the **other** spelling of the same directory (add or remove the `/private` prefix) and run `review status --cwd "$PWD"` → **Expected**: the same lineage, same state. If it reports no authority, that is the defect: paste both paths.

### Flow 21: Reviewer results on ExFAT (#1804)

macOS lacks the exclusive-rename primitive on ExFAT, so publication falls back to an exclusive-create copy. That fallback only ever runs on a real ExFAT volume.

1. [ ] Make one and mount it:

```bash
hdiutil create -size 200m -fs ExFAT -volname RDDTEST /tmp/rddtest.dmg
hdiutil attach /tmp/rddtest.dmg
```

If `hdiutil` rejects the filesystem name, `hdiutil create -help` lists the ones your macOS version accepts. A real ExFAT USB stick works just as well, and any external volume you already have formatted that way is fine.

2. [ ] Create a throwaway repo **on that volume** (`/Volumes/RDDTEST`), make a change, start the review, and follow each STATUS-issued capture route.
3. [ ] → **Expected**: the reviewer result publishes, and the final admitted capture closes and burns the review. A raw `ENOTSUP`, `EINVAL` or `operation not supported` reaching you is the defect.
4. [ ] Detach with `hdiutil detach /Volumes/RDDTEST` when done.

### Flow 22: First-use store contention (#1850) — **fixed on the branch, still broken in the published asset**

@edwinsaavedran reported that concurrent first use of a new runtime store leaked a raw `ENOENT` to the losing writers instead of a typed conflict. It was fail-closed and no ledger was corrupted, but a controller cannot classify or retry an untyped errno.

**The expected result depends on which candidate you run, and this is the clearest example in the guide of why that matters.** Two independent native macOS runs:

| candidate | result |
|---|---|
| published Refresh 5 asset (`2551c0a5`) | **FAIL** — 181/200 with the known `ENOENT` |
| branch after `0bcff694` | **PASS** — 200/200 |

This needs a source checkout rather than the released binary. From the branch under test:

```bash
TMPDIR=/private/tmp GIT_CONFIG_NOSYSTEM=1 \
  go test -p=1 ./internal/sddstatus \
  -run '^TestRuntimeLedgerCASAllowsOnlyOneConcurrentOrdinal$' -count=20
```

1. [ ] → **Expected on current branch source**: 20 of 20 pass. A failure here is a regression and worth reporting immediately.
2. [ ] If you are testing the **published asset** instead, expect failures, and report how many of 20 failed and whether the message is still `review store lock could not be acquired: no such file or directory`.
3. [ ] Either way, run it with `-race -count=3` as well. The original failure produced no Go data-race report, which is the signature of a filesystem race rather than a memory one.

### Flow 23: Managed profiles (#1781)

Only reproducible on a Mac under an MDM or corporate configuration profile, which cannot be staged on a personal machine.

1. [ ] If your Mac is company-managed, start a review and follow its STATUS-issued capture route → **Expected**: it closes on the final admitted capture, or fails with a typed permission error naming what to do.
2. [ ] A raw `EPERM` or `operation not permitted` with no continuation is the defect. Say which profile restrictions apply if you can.

---

## Flows 24 to 26: things nobody has tested yet

These cover behaviour that did not exist until this refresh. Flow 24 is verified and reproducible; **flows 25 and 26 have never run on a real machine of the platform they describe**, only against synthetic test profiles. That is exactly why they are here.

### Flow 24: A stale target names its own way out

The review target comes from the workspace snapshot, so anything that writes a file between asking for a transition and running it invalidates that transition. A linter, a build, a watcher, or your own shell redirect all do this. It used to produce an opaque refusal.

Run everything below with output going **outside** the repo.

1. [ ] Ask for the next transition and retain its execute operation and ordered argument tokens:

```
gentle-ai review status --next-transition --contract gentle-ai.review-integration/v2 --cwd . > /tmp/rdd-out/nt.json
```

2. [ ] Now change the workspace, exactly as a linter would: `echo "lint output" > lint-report.txt` **inside the repo**.
3. [ ] Run the returned execute transition unchanged → **Expected**: it refuses with

```
code:        stale_target_identity
next_action: review.status
cause:       review start target does not match the freshly built snapshot
```

The `cause` naming the real reason is the point. A bare `invalid_request` with an empty `required_inputs` is the defect.

4. [ ] **Follow the continuation it named**: ask for the transition again → **Expected**: a **different** target identity.
5. [ ] Run that new returned execute transition → **Expected**: exit 0, review starts. If following the named continuation does not unblock you, that is the report.

### Flow 25: Windows updates itself (**never tested on real Windows**)

Windows never auto-updated: it detected a new version and handed you a command to run yourself. With Go on PATH it now upgrades through a pinned `go install`. All the evidence we have is synthetic — this flow is the first real execution.

1. [ ] On Windows with Go 1.25.10+ on PATH, run a command that triggers the update check with an older gentle-ai installed → **Expected**: it upgrades itself. It does **not** print "requires manual update", and it does **not** send you to a releases page.
2. [ ] `gentle-ai --version` afterwards → **Expected**: the new version.
3. [ ] **Report the full output even when it works.** This path has never run outside a test double.
4. [ ] On Windows **without** Go → **Expected**: it still refuses, and the refusal names the exact `go install github.com/...@vX.Y.Z` command plus the Go version needed. A releases URL as the only guidance is the defect.

### Flow 26: The upgrade tells you if it landed somewhere else

`go install` writes to `GOBIN`, or `GOPATH/bin` when that is unset, which is not necessarily the directory holding the binary you run. Previously an upgrade could report success while you kept executing the old one.

1. [ ] Arrange the mismatch on purpose: `export GOBIN=$HOME/go-elsewhere` (a directory that is **not** on your PATH), then trigger the upgrade.
2. [ ] → **Expected**: the upgrade still reports success, **and** warns naming **both absolute paths** — where it wrote and what your shell runs.
3. [ ] → **Expected**: it never silently reports a clean success. If it does, you would keep running the old binary believing you updated, which is the defect this replaced.
4. [ ] If your `gentle-ai` is a symlink into the go-install directory → **Expected**: treated as a match, no warning. A spurious warning there is also a defect.

---

## Flows 27 to 33: environments we cannot reach

**Why these exist.** Everything the friction harness in `bench/` can build — a
linked worktree, a detached HEAD, a bare repository, a path full of spaces and
kanji, a submodule, a symlink, a mode-only change, a merge or rebase or
cherry-pick in progress, the kill switch flipped mid-review, a recovery of a
recovery, and now the whole SDD remediation successor cycle — now runs on every
loop iteration on Linux. These seven cannot be built in a temp directory on a
normal machine, so they are the ones only you can answer. Flow 34 below is a
different kind of gap: reachable everywhere, but written in a document the
harness cannot execute.

**Each flow names the platform or condition it needs.** If you cannot reach it,
mark it **N/A** and move on. Do not guess and do not simulate it with a mock: a
guessed PASS here is worse than a blank, because it closes a hole that is still
open.

Run everything below with output going **outside** the repository under test.

### Flow 27: A repository on a network mount — **needs NFS or SMB**

The review store lives under the Git common directory and takes an advisory
lock (`LOCK`) before it writes. On a local filesystem a lock is held, busy, or
absent. On NFS and SMB there is a fourth answer: the lock manager can be down,
the lease can expire under you, or `flock` can return `ENOLCK`/`EOPNOTSUPP`
because the mount does not support it at all. None of those is "busy" and none
is "missing", and code written against a local filesystem tends to fold them
into whichever of the two it happens to reach first.

1. [ ] Put a throwaway repo on a real network mount — an NFS export or an SMB
   share, not a loopback image. Note the mount options (`mount | grep <path>`)
   in your report; `nolock` and `local_lock=` change the answer.
2. [ ] Run the ordinary review lifecycle there:

```
gentle-ai review start --cwd .
# Follow each STATUS-issued review capture-result invocation.
# The final admitted capture closes and burns the review.
```

→ **Expected**: the final admitted capture closes and burns the review, exactly as on a local disk.

3. [ ] Now run two reviews at once against the same repo from two machines (or
   two shells on two mounts of the same export), started within a second of each
   other → **Expected**: one wins and the other gets a **typed conflict** naming
   what to do. A raw `ENOLCK`, `EOPNOTSUPP`, `Stale file handle` or
   `no locks available` reaching you is the defect — a controller cannot
   classify or retry an untyped errno.
4. [ ] Mount the same export with `-o nolock` and repeat step 2 →
   **Expected**: either it works, or it refuses with a message that says
   locking is unavailable on this mount. Silently proceeding without the lock
   is the worst outcome and the one worth reporting loudest.
5. [ ] **Report the mount type and options either way, including a PASS.** This
   path has never run on a network filesystem.

### Flow 28: A read-only filesystem — **needs a read-only mount**

Reviews write. A read-only repository is a legitimate state (a checked-out
artifact, a mounted snapshot, a container image layer), and the answer must be
a typed refusal, not a stack of failed writes.

1. [ ] Make a repo, then remount it read-only:

```
sudo mount -o remount,ro <mountpoint>     # or: sudo mount -o ro,bind /src /ro-copy
```

2. [ ] `gentle-ai review status --cwd .` → **Expected**: it works. Status is
   read-only by contract and must not need to write anything, not even a lock
   file.
3. [ ] `gentle-ai review start --cwd .` → **Expected**: a typed refusal naming
   that the store is not writable. A raw `EROFS` or
   `read-only file system` with no continuation is the defect.
4. [ ] → **Expected**: nothing was half-created. After the refusal,
   `git status` shows no new untracked files and the common dir holds no partial
   `gentle-ai/` tree.

### Flow 29: The disk fills during capture publication — **needs a small filesystem you can fill**

`ENOSPC` while publishing a capture must not create an ambiguous closed review.
This flow checks that the next STATUS query remains the authority for the
capture outcome.

1. [ ] Build a tiny filesystem and put the repo on it:

```
truncate -s 8M /tmp/tiny.img && mkfs.ext4 -q /tmp/tiny.img
mkdir -p /tmp/tiny && sudo mount -o loop /tmp/tiny.img /tmp/tiny
sudo chown "$USER" /tmp/tiny
```

2. [ ] Set up a repo there, stage a selected-lens change, start the review, and
   retain the STATUS-issued capture input.
3. [ ] Fill the remaining space (`fallocate -l <n> /tmp/tiny/ballast` until
   `df` reports 100%), then submit that capture → **Expected**: a typed failure
   naming the disk; the review is not reported as closed.
4. [ ] Delete the ballast and query STATUS for the same lineage → **Expected**:
   it reconciles the result or reoffers the identical bound capture. Submit it
   only when reoffered; its final admitted capture closes and burns the review.

### Flow 30: A case-insensitive, Unicode-normalizing volume — **needs APFS, HFS+, exFAT or NTFS**

The identity policy defers to the volume: paths are compared as the filesystem
presents them. On a case-insensitive volume `Docs/Guide.md` and `docs/guide.md`
are one file; on macOS HFS+ a file name is normalized to NFD, so a name you
typed as NFC comes back decomposed. **That branch has never executed anywhere.**
Linux CI is case-sensitive and normalization-neutral, so nothing in the test
suite has ever taken it.

1. [ ] Get such a volume. On macOS the boot volume already is one (`diskutil
   info / | grep -i "Case-Sensitive"`); otherwise create one:

```bash
# macOS
hdiutil create -size 200m -fs APFS -volname RDDCASE /tmp/rddcase.dmg
hdiutil attach /tmp/rddcase.dmg
# Linux, to reach exFAT/NTFS instead
truncate -s 200M /tmp/rddcase.img && mkfs.exfat /tmp/rddcase.img
```

2. [ ] Repo on that volume. Stage `docs/guide.md`, run `review start`, and
   follow the STATUS-issued capture route → **Expected**: the review closes on
   its final admitted capture.
3. [ ] Now `cd` in with a **differently cased** path
   (`cd /volumes/rddcase/...` instead of `/Volumes/RDDCASE/...`) and run
   `review status --cwd "$PWD"` → **Expected**: the same lineage and closed
   state. If it reports no authority, paste **both** spellings — that is the
   identity policy failing to defer to the volume.
4. [ ] Create a file whose name has a composed accent (NFC), for example
   `printf 'x\n' > "docs/café.md"` typed with a single é, stage it, and start a
   new review → **Expected**: it follows the STATUS-issued capture lifecycle.
   On HFS+ the volume stores the name decomposed (NFD), so the name git reports
   back differs byte-for-byte from the one you typed. A `path not found` or a
   scope mismatch on a file that is plainly there is the defect. Report the
   output of `git ls-files | xxd | head` alongside it, because the bytes are the
   finding.
5. [ ] Rename `docs/guide.md` to `docs/Guide.md`, start a new review, and follow
   STATUS → **Expected**: report whether the volume treats it as the same path.
   This is the case where "correct" depends entirely on the volume and we want
   the real answer, not the assumed one.

### Flow 31: Antivirus holding a file mid-write — **needs Windows with real-time scanning on**

On Windows a file can be opened by a scanner between the moment we create it
and the moment we rename or read it back. The failure is `ERROR_SHARING_VIOLATION`
(32) or `ERROR_ACCESS_DENIED` (5) on a file that unquestionably exists — a
**transient** condition, and the single most common way a transient gets
reported as a permanent corruption.

1. [ ] On Windows with Defender (or your corporate AV) real-time protection
   **on** and the test directory **not** excluded, run the full cycle several
   times in a row:

```powershell
1..20 | ForEach-Object {
  gentle-ai review start --cwd .
  # Follow each STATUS-issued review capture-result invocation.
  # The final admitted capture closes and burns the review.
}
```

→ **Expected**: 20 of 20 close on their final admitted capture. Report how many did.

2. [ ] Any failure → **Expected**: a typed, retryable message. If you see
   `authority_corrupted` or a defect report for what is really a scanner
   holding a handle for 40 ms, that is the defect: a transient
   was classified as permanent. Paste the whole message.
3. [ ] Repeat with the directory **excluded** from scanning → **Expected**: if
   the failures disappear, that confirms the cause. Say so in the report; it is
   the difference between "our locking is wrong" and "we do not retry a
   sharing violation".
4. [ ] Use `$LASTEXITCODE`, not `$?`, and do not pipe through `tee` — see
   "How to measure properly" below.

### Flow 32: Long paths — **needs Windows**

The Git common directory sits under `.git/`, the review store adds
`gentle-ai/review-transactions/v2/<lineage>/`, and the lineage identifier is
long. Start from a deep checkout and the full transaction path passes 260
characters, which is the classic Win32 `MAX_PATH` wall.

1. [ ] On Windows, clone or create the repo under a deliberately deep path so
   that the transaction path exceeds 260 characters. Check it:

```powershell
$p = "$(git rev-parse --git-common-dir)\gentle-ai\review-transactions\v2"
$p.Length
```

2. [ ] Run the full cycle → **Expected**: it completes. A raw
   `The system cannot find the path specified`, `path too long`, or a
   truncated path in the error is the defect.
3. [ ] Try it **both** with long-path support on and off, and say which you
   ran:

```powershell
Get-ItemProperty HKLM:\SYSTEM\CurrentControlSet\Control\FileSystem LongPathsEnabled
git config --get core.longpaths
```

→ **Expected**: with support **off**, a refusal that names the length problem
and what to enable — not a generic file error. Failing is acceptable there;
failing without saying why is not.

### Flow 33: The system clock moves backwards — **needs a VM snapshot or a settable clock**

**Read this one carefully, because it is testing a claim rather than a
behaviour.** Review closure is bound to the captured candidate state, not to a
wall-clock cutoff. This flow asks whether a clock change affects an already
closed review's STATUS result.

A clock goes backwards for ordinary reasons: an NTP correction after a bad RTC
read, a restored VM snapshot, a laptop resuming with a dead battery, or a
container starting with a host clock behind the one that wrote the state.

1. [ ] Establish a baseline: start a review, follow every STATUS-issued capture
   until it closes, then record `gentle-ai review status --cwd .`.
2. [ ] Move the clock backwards by an hour **after** closure:

```
sudo date -s "-1 hour"        # or restore a VM snapshot taken an hour ago
```

3. [ ] Run `gentle-ai review status --cwd .` → **Expected**: the same lineage
   and closed state. No time-based refusal or `authority_corrupted`.
4. [ ] The kill switch keeps a timestamp for provenance. Test that directly:
   `gentle-ai review mode disable`, then `gentle-ai review mode enable`, then
   move the clock back past `rdd_mode_recorded_at` in
   `$HOME/.gentle-ai/state.json`, then run `gentle-ai review mode status --json`
   → **Expected**: `effective: on`, with the source that decided it.
5. [ ] Move the clock **forwards** by a day and repeat steps 3 and 4 →
   **Expected**: identical answers. A rule that only holds in one direction is
   still a rule about the clock.
6. [ ] **Report the result even when everything passes**, with how you moved the
   clock and by how much. A confirmed absence is the whole point of this flow.

---

## Flow 34: historical candidate contract the harness could not execute

**Historical candidate rationale, superseded.** The `v2.2.0-rc.1` friction
harness drove the binary but could not drive a document. At that time, its
candidate procedure treated the product as closed when reviews were off and
`gentle-ai sdd-status <change> --json` reported the archive dependency `ready`
with a `reviewGate` carrying `delivery: "disabled/unmanaged"` whose `result`
was never `allow`. It then contrasted that result with the candidate
`sdd-archive` skill, which required `reviewGate.result: allow`. The procedure
below is preserved only as superseded candidate history, not as current release
behavior.

1. [ ] `gentle-ai install` (or `gentle-ai sync`) into a throwaway HOME, then
   read the installed `sdd-archive` skill and the shared review-ledger contract
   → **Historical candidate expectation, superseded:** both require
   `reviewGate.result: allow`.
2. [ ] In a repository with a complete, verified SDD change, run
   `gentle-ai review mode disable` and then
   `gentle-ai sdd-status <change> --json` → **Historical candidate expectation,
   superseded:** `archive` is not blocked, `reviewGate.delivery` is
   `disabled/unmanaged`, and `reviewGate.result` is **not** `allow`.
3. [ ] Ask your agent to archive that change → **Historical candidate observation
   to report:** an agent following the candidate skill literally stopped here,
   on a value the candidate product was correct to withhold, which was a rule
   blocking where the product no longer did.
4. [ ] **Report the result even if the agent archives anyway**, and say which
   sentence it followed. An agent that ignores its own contract is a different
   finding, not a passing one.

---

## How to measure properly (read this before reporting)

Six things that made earlier reports measure the wrong thing. They are not bugs, they are environment traps:

**Never write command output inside the repository under test.** The review target is derived from the workspace snapshot, so `gentle-ai ... > out.txt` run from inside the repo adds an untracked file and changes the very thing being measured. A transition proposed before the redirect no longer matches after it, and you get a refusal that has nothing to do with what you were testing. Keep a separate directory:

```
mkdir -p /tmp/rdd-out
cd $HOME/demo
gentle-ai review start --cwd . > /tmp/rdd-out/o.txt 2> /tmp/rdd-out/e.txt
```

This one cost the maintainer an hour of chasing a defect that was his own redirect. Flow 24 turns it into a deliberate test instead.

**If an agent runs it, set `CI=1`.** The consent question only shows up when there is a real terminal. Many agent harnesses allocate a pseudo-terminal, so the tool asks… and nobody answers: the shell hangs until it is killed, and the flow ends up as PARTIAL for a reason that is not the product's.

```
CI=1 gentle-ai review start
```

With `CI=1` the tool reviews anyway and warns on stderr that it did not ask. It is the same path CI already uses. **Exception: Flow 5 is precisely the test for the question**, so that one needs a real terminal and does not take `CI=1`; if your environment does not have one, mark it N/A.

**Exit codes get lost through a pipe.** In bash, `$?` gives you the status of the **last command in the pipeline**, not the binary's. If you run `gentle-ai ... | tee log.txt`, `$?` is `tee`'s and it is always 0. In PowerShell, `$LASTEXITCODE` does give you the binary's, and that is why the same case "behaved differently" between Windows and Linux. To measure properly:

```
gentle-ai review start --projection staged --base-ref HEAD~1 > out.txt 2> err.txt
echo "exit=$?"
```

**`--next-transition` needs the explicit contract.** This is not a bug: passing `--contract` is the opt-in to the negotiated envelope, and leaving it out has its own meaning.

**Before reporting, reproduce in the clean setup repo, not your own.** A tester nearly filed a bug that does not exist because their own repository — with its history, its hooks, its accumulated state — produced a failure that six commands in a fresh `mktemp -d` repo could not. Your repo is full of variables you stopped seeing months ago; the setup repo at the top of this guide has none. Run the same steps there first. If the failure survives the clean repo, report it with those steps — that is a reproduction we can run. **If it only reproduces in your repo, that is still a report worth filing** — say so explicitly, because then the shape of your repository IS the finding, and what we need from you is which shape: the worktree layout, the hook, the config that a clean repo does not have.

**"2.2.0-rc.1" does not identify a binary.** Release assets get replaced in place, so an asset downloaded two days ago and one downloaded today both call themselves `2.2.0-rc.1` — and a tester reported against the old one without any way to know, because nothing in the binary's own output could tell the two refreshes apart. Refresh 8 binaries solve this at the root: they embed `-pr1801-<shortsha>` in the version string. So paste the **complete `--version` output** in any report, not the release version; they are different things and only one is actionable.

## Historical reporting instructions

This section is retained to explain the original candidate procedure. Do not open a current issue or comment on PR [#1801](https://github.com/Gentleman-Programming/gentle-ai/pull/1801) for results from `v2.2.0-rc.1`; that candidate process is complete. For a current concern, first reproduce it against stable [`v2.3.0`](https://github.com/Gentleman-Programming/gentle-ai/releases/tag/v2.3.0) or the opt-in prerelease [`v2.4.0-rc.1`](https://github.com/Gentleman-Programming/gentle-ai/releases/tag/v2.4.0-rc.1), then use the repository's current contribution and issue workflow.

## What is NOT a bug

- **Delivery remains ordinary when reviews are off.** Repository policy rules; review mode does not veto a commit or push.
- **`requirements.txt`/`CMakeLists.txt` get one review (tier 1), not zero.** An unreviewed dependency bump would be a security downgrade.
- **With no terminal, the question does not appear and it reviews straight away** (it warns on stderr). Turning a safety net off silently is not an option.
- **"Not now" asks again on the next piece of work.** Per work unit, on purpose.
- **A `.md` with executable content escalates.** The content is read, not the extension.
- **The installed `.claude/CLAUDE.md` escalates if you put it in the diff.** That is what the `.gitignore` in the setup is for.

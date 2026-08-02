# 🧪 How to test — Organic RDD (pre-release 2.2.0-rc.1)

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

The binaries are on the pre-release page: **https://github.com/Gentleman-Programming/gentle-ai/releases/tag/v2.2.0-rc.1**

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

1. [ ] `gentle-ai review mode status --cwd $HOME/demo --json` → **Expected**: effective `on`, with the source that decides it.
2. [ ] `gentle-ai review mode disable --cwd $HOME/demo` → **Expected**: it confirms reviews are off.
3. [ ] `status` again → **Expected**: effective `off`, source `global`.
4. [ ] `gentle-ai review start --cwd $HOME/demo` → **Expected**: refused, naming that reviews are turned off **and naming the command that turns them back on**, scoped to the source that actually decided:

```
receipt-driven development is disabled: start is rejected because the global mode source
keeps it off; turn it back on with gentle-ai review mode enable --scope=global
```

It does NOT hang, it does NOT review. A refusal that exits non-zero and names no command is the defect. If you turned it off at clone scope, the scope in the message must say `clone`, not `global`.
5. [ ] `enable` and `status` → **Expected**: `on` again.
6. [ ] `disable --scope clone`, clone (`git clone $HOME/demo $HOME/demo2`) and `status` in `demo2` → **Expected**: `demo2` gives **on** — turning a clone off is NOT inherited.
7. [ ] **Before moving on**: `enable --scope clone` in `demo` → **Expected**: `on`.

### Flow 3: Documentation-only change (zero ceremony)

1. [ ] Edit `README.md` (plain text) and stage **only that file**: `git add README.md`.
2. [ ] `gentle-ai review start --cwd $HOME/demo` → **Expected**: `risk_level: low`, `selected_lenses: []` — zero reviewers, no question.

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

### Flow 6: Delivery with reviews turned off

**Watch out for the fixture**: this flow needs a configured upstream. With no remote, `pre-push` cannot derive what to compare against and fails closed with a typed error — that is correct, but it does not test what this flow wants to test. Set the remote up first:

```
git init --bare $HOME/demo-remote.git
cd $HOME/demo && git remote add origin $HOME/demo-remote.git
git push -u origin HEAD
```

1. [ ] Turn reviews off, make a change and commit → **Expected**: the commit works normally.
2. [ ] `gentle-ai review validate --gate pre-push --cwd $HOME/demo` → **Expected**: `"delivery": "disabled/unmanaged"`, `"allowed": false`, **exit 0**. It reports, it does not block.
3. [ ] Check that it does NOT say `allow` → **Expected**: never a false PASS.

### Flow 7: Turning it off mid-work and coming back

1. [ ] With reviews on, a change **staged but not committed**. Turn reviews off → **Expected**: everything flows.
2. [ ] Turn them on and `review start` → **Expected**: it works — it freezes and reviews from scratch. Nothing is lost. (If you already committed, the result carries a `hint` with `--base-ref`.)

### Flow 8: No phantom SDD artifacts

1. [ ] `git rev-parse --git-common-dir` and look inside → **Expected**: inside `gentle-ai/` only review state; nothing like `sdd*`, `trace`, `evaluation`.

---

## Flows 9 to 13: what we fixed with your feedback

These flows are new. Each one reproduces a bug someone in the community found in earlier rounds. They need a binary **later than Refresh 4**. Check which build you have with `gentle-ai doctor`: it names the binary you actually invoked and its version, and warns when that differs from the one on your `PATH`. If yours predates the current refresh, download it again from the release page or build from the PR branch.

### Flow 9: Pre-push after you already pushed (the bug that cost us the most)

Reported by @Wladimirfn, @Denver2828, @MarsSall and @Freedom2828. It looked like a Windows bug and it was not: it happened when the reviewed commit was **already published**.

You need the remote from Flow 6.

1. [ ] Docs change, `review start` + `review finalize` → **Expected**: approved receipt.
2. [ ] `review validate --gate pre-commit`, commit, and `review validate --gate pre-push` → **Expected**: `allow`, **exit 0**. (This is the regression: before the push it still has to work the same.)
3. [ ] **Push**: `git push origin HEAD`.
4. [ ] Turn reviews off and make ANOTHER docs commit.
5. [ ] `review validate --gate pre-push` → **Expected**: `"delivery": "disabled/unmanaged"`, **exit 0**. **NEVER** `authority_corrupted`.
6. [ ] Turn reviews on and repeat the gate → **Expected**: `result: "scope-changed"` naming a **runnable** recovery, not just a reason:

```
review lifecycle gate denied: scope-changed: recover via gentle-ai review recover
  --base-ref <commit> --committed-only (requires: predecessor_lineage_id, ...)
```

No corruption either.

7. [ ] **Now run exactly what it named**, filling in the required inputs, then `review finalize` on the successor, then repeat the gate → **Expected**: `allow`, exit 0.

Step 7 is the one that matters most in this whole guide. Until this refresh the message named a recovery that, followed literally, landed you right back at the same denial. The tests proved the message was **emitted** and never that following it **worked**. If you follow it and stay blocked, that is the most valuable report you can send us.

8. [ ] One case still ends without a one-step recovery **on purpose**: when the committed content is byte-identical to what was approved and only the commit topology changed. No command resolves it, so the message names none — it states the exact reason instead (`reviewed delivery is not exactly one commit from its reviewed base`). The same is true once the reviewed commit is fully published: the message names the reason (`reviewed delivery base commit is missing or ambiguous in publication range`), not a maintainer. That is intended, not a defect.

### Flow 10: First commit in a repo with no history

Reported by @lu149e, with the root cause confirmed by @Denver2828.

1. [ ] `mkdir $HOME/unborn && cd $HOME/unborn && git init -b main`.
2. [ ] Create a code file, `gofmt` if it applies, and `git add -A`. **Do not commit yet.**
3. [ ] `git rev-parse --verify HEAD` → **Expected**: it fails, because there is no first commit yet. That is correct.
4. [ ] `gentle-ai review start --cwd "$PWD"` → **Expected**: the review **starts**. It used to blow up with `Needed a single revision`.

### Flow 11: Transitions run exactly as they are printed

This one is for people using agents. The product prints the next command; if it is not literally executable, an agent that follows instructions to the letter gets stuck.

1. [ ] With a review in progress, ask for the next transition. The command needs the explicit contract:

```
gentle-ai review status --next-transition --contract gentle-ai.review-integration/v1
```

**First read `next_transition.kind`. Steps 2 to 4 only apply when it is `execute`.**

If it is `collect`, the tool is waiting for reviewer results that do not exist yet, so there is no command it could print: a model has to run the lens first. That is correct behaviour, not a defect. Skip to step 5. If it is `stop`, there is no transition at all and the same applies.

The quickest way to land on `execute` is to ask before any review has started, or right after `review capture-result`.

2. [ ] Look at the `token` of each argument in the response → **Expected**: each one is a complete flag ready to run (`--target=sha256:...`), not a name and a value sitting apart.
3. [ ] Read the `next_transition.execute.command` field → **Expected**: one complete line, starting with `gentle-ai review <verb>`, carrying every argument from step 2 in the same order and in `--flag=value` form. You never assemble it yourself: `operation` is a logical name (`review.start`), `command` is the runnable line.
4. [ ] **Copy and paste that `command` exactly as it came out**, without fixing anything → **Expected**: it runs. It used to print `--captured-results true` (with a space) and the parser rejected it, and before that there was no `command` at all — only `operation`, which an agent had to translate into a verb by guessing.
5. [ ] If you landed on `collect`, look at its `inputs[].arguments` → **Expected**: each one carries a `token` too, because those arguments are literally the flags of `review capture-result`. Do **not** mark the flow FAIL for the missing `command` on a collect: there is genuinely nothing runnable to print until a model has produced the reviewer result that `--input` points at.
6. [ ] Paste those collect tokens straight into `gentle-ai review capture-result` → **Expected**: it runs. **Do not add `--cwd`**: the tokens already carry `--repository-context`, and passing both is refused. If you hit that refusal, report whether it told you which one to drop.

### Flow 12: Finalize without evidence says what to do

**This flow needs a review in `validating`, and getting there is part of the test.** Do not look it up here and do not read the source: start a review that selects lenses (the Flow 4 auth change works) and find your way to `validating` using only what the tool tells you.

That is deliberate. Three testers in a row marked this N/A because they could not work out how to produce the reviewer-result payload `capture-result --input` demands. That was not their failure, it was the finding: the product has a command that emits the schema with a working example and nothing pointed at it. It should now point at it from both the flag help and the refusal.

1. [ ] Work your way to `validating` from the tool's own output → **Expected**: every refusal along the way names what to do next. Write down each one that does not, and what you had to guess. **If you cannot get there without reading source or asking someone, stop and report that** — it is worth more than a PASS on the step below.

**The actual test:**

2. [ ] With that review in `validating` and no captured evidence, run `gentle-ai review finalize --lineage <id> --cwd .` → **Expected**: an error that **names both commands** to get out:

```
finalize for lineage "<id>" had no verification evidence to consume and made no
transition; capture it first with `gentle-ai review capture-evidence`, then run
`gentle-ai review finalize --lineage <id> --captured-evidence`
```

It used to say `continue the current review state` and nothing ever happened.

### Flow 13: Flag combinations we do not support

1. [ ] `review start --projection staged --base-ref HEAD~1` → **Expected**: a typed rejection naming **both escapes**: `--projection staged` alone (to review the index) or `--base-ref <ref> --committed-only` (to review base-diff). It does not guess which one you meant.

---

## Flows 14 to 19: organic DX

These flows test the new work: that the tool recovers on its own, that blocks say how to continue, and that old history does not constrain current work. They need a binary **later than Refresh 4**.

### Flow 14: Old receipts do not block new work

The bug @decode2 and @fisidj found. You need the remote from Flow 6.

1. [ ] Make **three** docs changes, each one with `review start` + `review finalize` + commit. Push the first two.
2. [ ] Turn reviews off and make a fourth commit **without reviewing it**.
3. [ ] `review validate --gate pre-push` → **Expected**: `delivery: "disabled/unmanaged"`, exit 0. **Never** "multiple terminal review receipts require explicit target selection".
4. [ ] Turn reviews on and run `review status --contract gentle-ai.review-integration/v1` → **Expected**: it says nothing governs the candidate and that you should start a new one. The old lineages show up listed as a recovery option, **not** as a list you are forced to pick from.

### Flow 15: Recovery that drives itself

1. [ ] Get to a state where `review recover` is the continuation (for example: an approved review, then change the candidate's scope).
2. [ ] Run `review recover` **without** `--actor`, **without** `--reason` and **without** `--maintainer-authorization` → **Expected**: it works. The tool derives all three on its own.
3. [ ] Now run the same thing passing a deliberately **wrong** `--maintainer-authorization` → **Expected**: it refuses. The tool authorizes itself when you said nothing, but it **never corrects what you said wrong**.

### Flow 16: Blocks say which command comes next

1. [ ] With a review in progress waiting for results, run `review finalize --lineage <id>` → **Expected**: the error names `gentle-ai review capture-result`.
2. [ ] Run a gate in a repo **with no review at all**, asking for the negotiated envelope:

```
gentle-ai review validate --gate pre-commit --contract gentle-ai.review-integration/v1
```

→ **Expected**: `code: receipt_missing` and `next_action: review.start`. It used to say only "stop" and the agent had to guess.

### Flow 17: Visible numbers when it escalates

1. [ ] Push a correction past its line budget → **Expected**: the message says **spent, remaining and total** with distinct labels. It used to escalate with a number that was not printed anywhere.

### Flow 18: Defect report ready to paste

**What a report is for.** A defect report is written for a *stored-state deadlock*: an operation that tried to write, could not, and has no command that resolves it. That is a narrow class on purpose. Most blocks are not in it — a policy denial, a scope change, an exhausted correction budget and an escalated lineage are all the tool working correctly, and none of them generates a report. Reads never generate one either: `review status` is read-only and stays read-only, so a state it merely *reports* as unrecognised writes nothing to your repository.

Earlier rounds asked you to report this flow "if you hit a terminal block caused by us", with no way to produce one. Following the guide never reaches the class above, so the step was always marked failed. Here is a procedure that actually reaches it.

1. [ ] Docs change, `review start` + `review finalize` → **Expected**: approved receipt. Note the `receipt_path` it prints.
2. [ ] Break the stored receipt on purpose so it no longer matches the authority that produced it — edit that file and change `"generation"` to a different number:

```
$EDITOR "$(git rev-parse --git-common-dir)"/gentle-ai/review-transactions/v2/<lineage>/review-receipt.json
```

3. [ ] Run `review finalize --lineage <lineage>` again → **Expected**: it fails with **exit 1**, and the last sentence names the report file and the link:

```
... A defect report was saved at <...>/gentle-ai/defect-reports/receipt-publication-conflict-<hash>.md
-- file it at https://github.com/Gentleman-Programming/gentle-ai/issues/new/choose.
```

4. [ ] Open that file → **Expected**: it carries version, commit, OS, the operation and the error. It does **NOT** carry the contents of your files, or absolute paths with your username, or environment variables. It is meant to be pasted into a public issue.
5. [ ] A block that is **your decision** (abandon vs recover), and any ordinary denial or escalation → **Expected**: NO report is generated, and none is promised. There is no bug to report.

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
gentle-ai review finalize --cwd .
gentle-ai review validate --gate pre-commit --cwd .
```

→ **Expected**: it completes with `allow`. No "no discoverable review lineage", no path-shaped error.

**The `git add` is not optional and it is not tidiness.** START with the default workspace projection freezes your uncommitted change; the pre-commit gate asks about the staged index. Skip the staging and you get `receipt_unrelated` — a correct answer to a different question, which reads exactly like the path bug this flow is looking for. Reported by @edwinsaavedran after it produced precisely that false signal.
3. [ ] Now `cd` into the **other** spelling of the same directory (add or remove the `/private` prefix) and run `review status --cwd "$PWD"` → **Expected**: the same lineage, same state. If it reports no authority, that is the defect: paste both paths.

### Flow 21: Reviewer results on ExFAT (#1804)

macOS lacks the exclusive-rename primitive on ExFAT, so publication falls back to an exclusive-create copy. That fallback only ever runs on a real ExFAT volume.

1. [ ] Make one and mount it:

```bash
hdiutil create -size 200m -fs ExFAT -volname RDDTEST /tmp/rddtest.dmg
hdiutil attach /tmp/rddtest.dmg
```

If `hdiutil` rejects the filesystem name, `hdiutil create -help` lists the ones your macOS version accepts. A real ExFAT USB stick works just as well, and any external volume you already have formatted that way is fine.

2. [ ] Create a throwaway repo **on that volume** (`/Volumes/RDDTEST`), make a change, and run `review start` → `review capture-result` → `review finalize`.
3. [ ] → **Expected**: the reviewer result publishes and finalize reaches its normal terminal state. A raw `ENOTSUP`, `EINVAL` or `operation not supported` reaching you is the defect.
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

1. [ ] If your Mac is company-managed, run the ordinary `review start` → `finalize` → `validate` cycle → **Expected**: it completes, or fails with a typed permission error naming what to do.
2. [ ] A raw `EPERM` or `operation not permitted` with no continuation is the defect. Say which profile restrictions apply if you can.

---

## Flows 24 to 26: things nobody has tested yet

These cover behaviour that did not exist until this refresh. Flow 24 is verified and reproducible; **flows 25 and 26 have never run on a real machine of the platform they describe**, only against synthetic test profiles. That is exactly why they are here.

### Flow 24: A stale target names its own way out

The review target comes from the workspace snapshot, so anything that writes a file between asking for a transition and running it invalidates that transition. A linter, a build, a watcher, or your own shell redirect all do this. It used to produce an opaque refusal.

Run everything below with output going **outside** the repo.

1. [ ] Ask for the next transition and keep the `command` it prints:

```
gentle-ai review status --next-transition --contract gentle-ai.review-integration/v1 --cwd . > /tmp/rdd-out/nt.json
```

2. [ ] Now change the workspace, exactly as a linter would: `echo "lint output" > lint-report.txt` **inside the repo**.
3. [ ] Run the command you kept, unchanged → **Expected**: it refuses with

```
code:        stale_target_identity
next_action: review.status
cause:       review start target does not match the freshly built snapshot
```

The `cause` naming the real reason is the point. A bare `invalid_request` with an empty `required_inputs` is the defect.

4. [ ] **Follow the continuation it named**: ask for the transition again → **Expected**: a **different** target identity.
5. [ ] Run that new command → **Expected**: exit 0, review starts. If following the named continuation does not unblock you, that is the report.

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
2. [ ] Run the ordinary cycle there:

```
gentle-ai review start --cwd .
gentle-ai review finalize --cwd .
gentle-ai review validate --gate pre-commit --cwd .
```

→ **Expected**: it completes with `allow`, exactly as on a local disk.

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

### Flow 29: The disk fills during a receipt write — **needs a small filesystem you can fill**

`ENOSPC` in the middle of publishing a receipt is the one failure that can
leave authority and receipt disagreeing. The store publishes immutably
(no-replace) precisely so a losing writer cannot corrupt a published
generation, and this is the flow that tests whether that holds when the failure
is the disk rather than a competing writer.

1. [ ] Build a tiny filesystem and put the repo on it:

```
truncate -s 8M /tmp/tiny.img && mkfs.ext4 -q /tmp/tiny.img
mkdir -p /tmp/tiny && sudo mount -o loop /tmp/tiny.img /tmp/tiny
sudo chown "$USER" /tmp/tiny
```

2. [ ] Set up a repo there, stage a docs change, run `review start` →
   **Expected**: it works.
3. [ ] Fill the remaining space (`fallocate -l <n> /tmp/tiny/ballast` until
   `df` reports 100%), then run `review finalize` → **Expected**: a typed
   failure naming the disk. Not a partial receipt, and not an approval.
4. [ ] Delete the ballast and run `review finalize` again → **Expected**: it
   either completes cleanly or refuses with a runnable continuation. **A state
   that no command can leave is the defect**, and that is exactly the
   stored-state deadlock flow 18 describes — check whether a defect report was
   written, and paste it.
5. [ ] `review validate --gate pre-commit` after step 3 and before step 4 →
   **Expected**: never `allow`. A half-written receipt must never gate as
   approved.

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

2. [ ] Repo on that volume. Stage `docs/guide.md`, run `review start` +
   `review finalize` → **Expected**: an approved receipt.
3. [ ] Now `cd` in with a **differently cased** path
   (`cd /volumes/rddcase/...` instead of `/Volumes/RDDCASE/...`) and run
   `review validate --gate pre-commit --cwd "$PWD"` → **Expected**: the same
   lineage, `allow`. If it reports no authority, paste **both** spellings — that
   is the identity policy failing to defer to the volume.
4. [ ] Create a file whose name has a composed accent (NFC), for example
   `printf 'x\n' > "docs/café.md"` typed with a single é, stage it and run the
   full cycle → **Expected**: it completes. On HFS+ the volume stores it
   decomposed (NFD), so the name git reports back differs byte-for-byte from
   the one you typed. A `path not found` or a scope mismatch on a file that is
   plainly there is the defect. Report the output of
   `git ls-files | xxd | head` alongside it, because the bytes are the finding.
5. [ ] With the receipt approved, rename `docs/guide.md` to `docs/Guide.md` and
   re-run the gate → **Expected**: on a case-insensitive volume this is not a
   scope change, because it is the same file. Whatever the answer, say which it
   gave: this is the case where "correct" depends entirely on the volume and we
   want the real one, not the assumed one.

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
  gentle-ai review finalize --cwd .
  gentle-ai review validate --gate pre-commit --cwd .
}
```

→ **Expected**: 20 of 20 reach `allow`. Report how many did.

2. [ ] Any failure → **Expected**: a typed, retryable message. If you see
   `authority_corrupted`, `receipt_missing` or a defect report for what is
   really a scanner holding a handle for 40 ms, that is the defect: a transient
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
long. Start from a deep checkout and the full path to a receipt passes 260
characters, which is the classic Win32 `MAX_PATH` wall.

1. [ ] On Windows, clone or create the repo under a deliberately deep path so
   that the receipt path exceeds 260 characters. Check it:

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
behaviour.** The design states explicitly that approval is bound to candidate
**content**, never to a wall-clock moment, and that a recorded timestamp is
never used as an approval cutoff. That is written down in the source and it is
true where it was written down. This flow asks whether it holds *everywhere* —
including the places nobody thought about when they wrote the comment.

A clock goes backwards for ordinary reasons: an NTP correction after a bad RTC
read, a restored VM snapshot, a laptop resuming with a dead battery, a container
starting with a host clock behind the one that wrote the state.

1. [ ] First, establish the baseline the claim rests on. Approve a review, then
   look at the receipt:

```
gentle-ai review start --cwd . && gentle-ai review finalize --cwd .
cat "$(git rev-parse --git-common-dir)"/gentle-ai/review-transactions/v2/<lineage>/review-receipt.json
```

→ **Expected**: the receipt carries **no timestamp of any kind** — only
content bindings (`base_tree`, `initial_review_tree`, `final_candidate_tree`,
`paths_digest`, `fix_delta_hash`, `policy_hash`, `evidence_hash`,
`terminal_state`). If you find a time field in there, stop and report it: the
claim is already false and the rest of this flow is academic.

2. [ ] Now move the clock backwards by an hour, **after** the approval:

```
sudo date -s "-1 hour"        # or restore a VM snapshot taken an hour ago
```

3. [ ] `gentle-ai review validate --gate pre-commit --cwd .` → **Expected**:
   `allow`, unchanged. The approval is bound to bytes that did not move.
4. [ ] `gentle-ai review status --cwd .` → **Expected**: the same lineage and
   state. No "receipt from the future", no `authority_corrupted`.
5. [ ] The kill switch keeps a timestamp that the design says is provenance
   only. Test that directly: `gentle-ai review mode disable`, then
   `gentle-ai review mode enable`, then move the clock back past
   `rdd_mode_recorded_at` in `$HOME/.gentle-ai/state.json`, then
   `gentle-ai review mode status --json` → **Expected**: `effective: on`, and
   the source that decided it. A candidate refused because it is "older than
   the mode record" would be exactly the time-based approximation the design
   says it deliberately does not implement.
6. [ ] Finally, move the clock **forwards** by a day and repeat steps 3 to 5 →
   **Expected**: identical answers. A rule that only holds in one direction is
   still a rule about the clock.
7. [ ] **Report the result even when everything passes**, with how you moved the
   clock and by how much. A confirmed absence is the whole point of this flow.

---

## Flow 34: a shipped contract the harness cannot execute

**Why this exists.** The friction harness drives the binary. It cannot drive a
document — and one of the two documented known-open limitations lives entirely
in one. `j42-kill-switch-versus-sdd-archive` proves the **product** half is
closed: with reviews off, `gentle-ai sdd-status <change> --json` reports the
archive dependency `ready` and a `reviewGate` carrying
`delivery: "disabled/unmanaged"` whose `result` is never `allow`, because
declining to manage is not approval. What no journey can check is what an
**agent** does when it reads the shipped `sdd-archive` skill, whose own contract
requires structured status with `reviewGate.result: allow` — a value a disabled
run is right never to produce.

1. [ ] `gentle-ai install` (or `gentle-ai sync`) into a throwaway HOME, then
   read the installed `sdd-archive` skill and the shared review-ledger contract
   → **Expected today**: both still require `reviewGate.result: allow`.
2. [ ] In a repository with a complete, verified SDD change, run
   `gentle-ai review mode disable` and then
   `gentle-ai sdd-status <change> --json` → **Expected**: `archive` is not
   blocked, `reviewGate.delivery` is `disabled/unmanaged`, `reviewGate.result`
   is **not** `allow`.
3. [ ] Ask your agent to archive that change → **Report what it does.** An agent
   following the skill literally stops here, on a value the product is correct
   to withhold, which is a rule blocking where the product no longer does.
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

## What to report

Anything that does not match an **Expected** — and anything you find confusing even if it works. Open an issue with: what you tried, what you expected, what you saw, `gentle-ai --version`, OS, and terminal output.

👉 https://github.com/Gentleman-Programming/gentle-ai/issues/new/choose — mention that this is the **2.2.0-rc.1 pre-release**.

If everything worked, comment on PR [#1801](https://github.com/Gentleman-Programming/gentle-ai/pull/1801) with which flows passed and on which platform — that feedback decides the merge.

## What is NOT a bug

- **The gate exits 0 when reviews are off.** It reports `disabled/unmanaged` but does not veto — repository policy rules.
- **`requirements.txt`/`CMakeLists.txt` get one review (tier 1), not zero.** An unreviewed dependency bump would be a security downgrade.
- **With no terminal, the question does not appear and it reviews straight away** (it warns on stderr). Turning a safety net off silently is not an option.
- **"Not now" asks again on the next piece of work.** Per work unit, on purpose.
- **A `.md` with executable content escalates.** The content is read, not the extension.
- **The installed `.claude/CLAUDE.md` escalates if you put it in the diff.** That is what the `.gitignore` in the setup is for.

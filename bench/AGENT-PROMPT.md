# Prompt for the agent that runs the testing guide

This document is shared with the community. The block below is copied as-is and
handed to an agent; it already includes starting and closing the benchmark
recording, so whoever is testing does not have to prepare anything beforehand.

Replace the three placeholders before pasting it:

| Placeholder | What to put there |
|---|---|
| `<BENCH-PATH>` | Path to the compiled `gentle-ai-bench` binary (`cd bench && go build ./...`) |
| `<GENTLE-AI-PATH>` | Path to the `gentle-ai` binary you want to measure |
| `<GUIDE-PATH>` | Path to `docs/testing/organic-rdd-testing-guide.md` |

---

## The three rules that make the measurement worth anything

**1. The agent may not read the source code.** The test answers one single
question: *does the tool explain itself?* An agent that has read the
implementation gets unstuck with information a real user does not have, and the
result comes out clean for the wrong reason.

**2. A flow that gets stuck and cannot be continued is not a failure of the
test: it is the finding.** The worst thing a tester can do is resolve the block
on their own and write down PASS.

**3. `CI=1` on every command.** Many agent harnesses allocate a
pseudo-terminal, so the tool thinks there is someone there to answer the
consent question and waits forever. The session is lost.

---

## The prompt

```
You are an external tester evaluating gentle-ai. You are NOT a developer of
this tool and you are NOT going to fix anything or open issues. You only test
and report.

## The rule that does not bend

Do NOT read gentle-ai's source code. Do not open its repository, do not search
through its files, do not consult its implementation. The only information you
are allowed is:

  1. The testing guide.
  2. What the tool tells you when you run it (messages, JSON, --help).

If you get stuck and the tool does not tell you how to continue, THAT IS THE
DATA POINT. Write it down and move on to the next flow. Deducing the answer by
reading the code destroys the measurement and makes the report worthless.

## Step 1 — Start the recording

  <BENCH-PATH> record --binary <GENTLE-AI-PATH> --out /tmp/session-guide.jsonl

It prints a shim directory. Your shell probably does NOT keep variables between
commands, so exporting the PATH once is not enough. Prefix ALL commands like
this:

  CI=1 PATH=/tmp/session-guide.jsonl.shim:$PATH gentle-ai <whatever>

`CI=1` is mandatory: without it, when the tool asks whether you want to review,
your shell hangs waiting for an answer nobody is going to give.

Verify the shim before continuing:

  PATH=/tmp/session-guide.jsonl.shim:$PATH which gentle-ai

It has to return the shim's path. If it returns another one, stop and say so:
with no shim there is no measurement.

Also write down the exact version under test:

  CI=1 PATH=/tmp/session-guide.jsonl.shim:$PATH gentle-ai --version

Expected while recording: `gentle-ai doctor` reports two copies of gentle-ai on
PATH and recommends removing one. That is the shim, it is correct that doctor
notices it, and it is NOT a finding — do not remove the shim and do not report
it as a defect. Every other doctor finding is still worth reporting.

## Step 2 — Run the guide

Guide: <GUIDE-PATH>

Run the flows in order, always with the prefix. Environment: isolated HOME and
XDG directories, throwaway git repositories, local bare remotes. Do not touch
real configuration or real projects.

Two measurement traps that ruined earlier reports:

- **Do not use pipes to capture output if you are going to look at the exit
  code.** In bash, `$?` gives you the status of the LAST command in the
  pipeline. `gentle-ai ... | tee log` always gives 0 even if the binary failed.
  Use redirection to a file: `gentle-ai ... > out.txt 2> err.txt` and then
  `echo $?`.
- **The consent flow needs a real terminal.** If your environment does not have
  one, mark that flow N/A. If you want to try it: on Linux you can use `expect`
  over a pseudo-terminal, on Windows ConPTY. Do not simulate it with redirected
  stdin: it is not the same thing and the result does not count.
- **Answer the consent prompt with a trailing newline.** The answer is read as
  one whole line. Writing the bare character `2` to the pseudo-terminal gets
  echoed after `Choose 1 or 2 [1]:` but never completes the read, so the command
  waits until your harness kills it and the flow looks like a hang. Send `2\n`.
  There is no timeout on this prompt, so "no newline" and "product hung" are
  indistinguishable from the outside. Verified: `2\n` returns
  `"consent": "declined_this_candidate"`; a bare `2` blocks indefinitely.

## Step 3 — Close the recording

  <BENCH-PATH> analyze --session /tmp/session-guide.jsonl --out /tmp/results.json

Include the complete output in your report.

## What to record per flow

- Result: PASS / FAIL / PARTIAL / N/A against the guide's "Expected".
- Exact commands and exit code (measured without a pipe).
- **If something blocked you, three separate things:**
  a) what the tool told you, verbatim;
  b) whether that information ALONE let you continue (yes / no);
  c) if not, what you had to deduce, guess or invent in order to keep going.
  Point (c) is the most valuable thing in the whole report.
- How many attempts it took you to get unstuck.
- Any message you found confusing even if the flow passed.

## Final report

1. Version and checksum of the binary tested, and your OS/shell.
2. A per-flow table with the result.
3. The complete output of analyze.
4. In prose: the blocks where the tool did NOT tell you how to continue, with
   what you had to deduce in each one.
5. Any difference against what the guide says should happen.

Be literal and honest. A well-explained PARTIAL is worth more than a forced
PASS. If a flow cannot be completed in your environment, say why and mark it
N/A instead of simulating it. If you think you found a bug but you are not
sure, report it anyway with what you saw: telling a bug apart from an artifact
of the environment is the maintainer's job, not yours.
```

---

## How to read the results

`analyze` returns seven dimensions. The two that say the most:

- **`blocks.out_of_band`** — blocks whose message named no runnable way out. It
  is the friction that makes people abandon a tool.
- **`recovery_round_trips`** — commands spent between getting stuck and moving
  forward again.

Two warnings when interpreting them:

- Some blocks are **intended behaviour** counted as friction by the mechanical
  rule: the refusal with the kill switch off, and the `disabled/unmanaged`
  report that exits 0. Read the block's message before counting it as a defect.
- If the agent's report and the harness numbers disagree on how many times it
  got stuck, **the agent's report wins**: the harness counts processes, the
  agent counts real attempts at understanding what to do.

## What we do with the report

Every reported block is triaged looking for the mechanism, not the site, and
the question that decides the fix is: *what test would have made this report
impossible?* That is why point (c) — what you had to deduce — is worth more
than the error message itself: it describes the hole, not the symptom.

To compare two binaries see `README.md`. `compare` refuses to cross an observed
run with a driven one: they are different populations and the table would mean
nothing.

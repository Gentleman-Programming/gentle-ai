# Gentle Shell TUI acceptance

The `Gentle Shell TUI acceptance` workflow performs real Gentle Shell installs
through the gentle-ai TUI, typing keys into tmux the way a user would, on two
real hosts:

| Runner | Host | Why it is real |
|---|---|---|
| `ubuntu-latest` | Linux amd64, non-root `runner` user | systemd PID 1 with a delegated `user@UID` manager, `/run/user/UID/bus` |
| `macos-latest` | macOS arm64, non-root user | native install, no emulation |

Windows is out of scope: GitHub only offers Windows Server, which the product
refuses by design. Docker on Apple Silicon cannot replace the Linux runner,
because Docker Desktop's Rosetta layer cannot run systemd units, so the
delegated-unit launch probe can never pass there.

## Run it

```sh
gh workflow run shell-tui-acceptance.yml --ref <branch>
gh workflow run shell-tui-acceptance.yml --ref <branch> -f scenarios=A,B,E
```

The workflow only runs on dispatch, with `contents: read`. It builds
`gentle-ai` from the dispatched ref and runs
`e2e/shell-tui-acceptance.py` on both runners. The job fails when any selected
scenario fails.

To run the driver locally on Linux or macOS, as a regular user, run:

```sh
go build -o /tmp/gentle-ai ./cmd/gentle-ai
work="$(cd -P "$(mktemp -d)" && pwd)"
python3 -I e2e/shell-tui-acceptance.py --gentle-ai /tmp/gentle-ai \
  --evidence "$work/evidence" --work "$work" --scenarios A,B
```

The driver never touches your real HOME. Each scenario gets its own canonical
HOME under `--work`, an empty environment (`env -i`) with
`GENTLE_AI_NO_SELF_UPDATE=1` and `GENTLE_AI_NO_ANIMATION=1`, and a tmux server
on a private socket. On Linux it passes through `XDG_RUNTIME_DIR` and
`DBUS_SESSION_BUS_ADDRESS`, which the workflow sets up for the delegated
manager. It refuses to run as root.

## Scenarios

Each scenario is independent: B to E first install the defaults (A) in their
own HOME. Every scenario checks for correct product behavior, so a known
defect shows up as FAIL.

| ID | Flow | Asserts |
|---|---|---|
| A | Fresh defaults: Gentleman, all recommended ON, Stable, Separate | install completes; files below match; `gentle-shell --version` |
| B | Manage on A: Neutral, Strict TDD OFF, Context7 OFF, then Apply preferences | "Apply changes to installation", then "Preferences updated."; canonical bytes, `schema` kept; the Context7 entry keeps `url` and `exposure` |
| C | Manage on A: CodeGraph OFF (component change) | "Update installation" leads to an install plan, and the install completes without CodeGraph |
| D | Manage on A: SDK Main preview, then Check for updates and Update now, then back to Stable | `main_sdk` and `config/update-channel.json` follow the channel; the registry channel matches; launch works |
| E | Manage on A: Uninstall (keep backup), then reinstall | backup is `<target>.removed-<ts>`; target, launcher and registry entry are gone; reinstall passes A's checks |
| F | Fresh: Custom persona, all toggles OFF, Main preview, Separate | no `persona.json`; OFF policies; `rdd_mode` off; Context7 disabled but complete; `main_sdk` present |
| G | Install Pi as a user (`npm --prefix ~/.npm-global`), run it once, then "On my existing Pi" | install completes on `~/.pi/agent`; after any failed attempt, `~/.pi/agent` is byte-identical to its snapshot |

Files checked for a profile (target `~/.local/share/gentle-shell/install`):

- `installation.json`: schema, `Mode`, `Destination`, `codegraph`, `main_sdk`.
- `config/persona.json`, `config/background-subagents.json` and
  `config/strict-tdd.json`: the exact bytes the installer's canonical writer
  produces, including `schema`.
- `home/.gentle-ai/state.json`: `rdd_mode`.
- `<agent>/mcp.json`: `context7` must equal
  `{"url": "https://mcp.context7.com/mcp", "enabled": <bool>, "exposure": "codemode"}`.
- `config/update-channel.json`, when present: the channel matches.
- `~/.local/bin/gentle-shell` (Separate): links to `<target>/bin/gentle-shell`.
- Shell startup file: with `~/.local/bin` off the PATH the harness gives the
  TUI (`--path`), a Separate install leaves exactly one
  `# gentle-ai: gentle-shell launcher` line in the file chosen for the
  harness's `SHELL` (`~/.profile` when unset); otherwise, after uninstall and
  for "On my existing Pi" (G), no marked line anywhere and no startup file the
  install created. Checked in A-F at each install check, in E and G after
  uninstall.
- `~/.gentle-ai/shell-installs.json`: exactly one entry for the target, with
  its mode and channel.

### SKIP

A scenario reports SKIP only when the TUI shows one of the `GATES` texts in the
driver verbatim. That text marks an intended platform gate. Today the only gate
is `SDK Main + stock provider is qualified on Linux amd64 today`, which affects
D and F on macOS. Any other refusal is a FAIL. When a platform gate ships with
its own TUI text, add that exact text to `GATES`.

## Evidence

The artifact `shell-tui-acceptance-<os>-<sha>` contains:

- `summary.json`: `{scenario: {status: PASS|FAIL|SKIP, reason}}`. The driver
  also prints it at the end.
- `<scenario>/result.json`: status, reason, the scenario HOME, and progress
  notes.
- `<scenario>/screens/*.txt`: every captured TUI screen, in order: the
  configuration, Continue, the install result, Manage, and any timeout or
  unexpected exit.
- `<scenario>/files/<step>/...`: copies of the checked files after each step.
- `<scenario>/<step>-version.txt`: the `gentle-shell --version` output.
- `G/agent-diff.txt`: the `~/.pi/agent` changes after a failed attempt.

## Changing the driver

Pure helpers (screen parsing, key plans, file checks) have unit tests in
`e2e/shell-tui-acceptance.test.py`. Run
`python3 e2e/shell-tui-acceptance.test.py` after editing them. The workflow
runs these tests before it builds the candidate.

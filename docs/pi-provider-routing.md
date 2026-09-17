# Configure native Pi review roles

Configure `review-refuter` and `review-validator` through gentle-pi's model configuration. Go reads those same saved assignments for native role capture; it never writes configuration or creates another routing format.

## Resolution

Go reads `GENTLE_PI_CONFIG_HOME/models.json` when that existing locator is set, otherwise `~/.pi/gentle-ai/models.json`. Only an absent global file falls back to `<validated repository>/.pi/gentle-ai/models.json`; entries are never merged. Unrelated entries are ignored and the file remains untouched.

An assignment is a model string or an object containing optional `model` and `thinking` fields. A missing assignment or `{}` leaves the original Pi arguments unchanged, using **Pi's persisted defaults**, not the active session. There is no special `default` role entry.

Explicit values travel only as `--model` and `--thinking` argv pairs. Thinking accepts `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, and `max`. Pi owns model resolution and model-dependent clamping; Go has no model registry.

## Invalid configuration stops before execution

Repair the assignment through gentle-pi and retry the offered capture. Go intentionally validates more strictly than gentle-pi normalization: malformed files (including legacy project files), invalid selected fields, empty model strings, and unknown selected fields fail rather than silently falling back to another paid model. Unrelated entries do not affect routing.

Go still owns binding validation, prompt materialization, request hashes, and admission. The transport receives only argv overrides and the unchanged opaque stdin prompt; its environment allowlist and discovery lockdown are unchanged.

## Verification boundary

Tests exercise local helper subprocesses and both native capture/admission paths, including malformed-config refusal without spawning. They do **not** prove organic execution by the real Pi runtime. That proof and the linked gentle-pi configuration UI work remain separate; no paid model invocation is required by these tests.

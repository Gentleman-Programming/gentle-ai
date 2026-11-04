# GitHub star prompt

Gentle AI can thank engaged users with a single, non-blocking invitation to star the repository — modeled on t3code's `GitHubStarPrompt` courtesies, and engineered to never interrupt active work (issue #5969).

## Behavior

- **Engagement gate.** The offer may appear only after more than `Threshold` (5) successful completed operations (upgrade, sync, uninstall, agent-builder install). The counter persists in `star-prompt.json` inside the gentle-ai state directory.
- **Idle only.** The offer waits 30 seconds without any keypress and with no operation running before showing anything. Typing at any moment restarts the wait and is never consumed.
- **Once per session.** If the offer goes unanswered, it auto-hides after 5 minutes and does not return until the next session.
- **Dismissal is forever.** `Alt+D` (or any dismissal path) persists a never-ask-again flag. If state cannot be read or written, the TUI never asks at all: a dismissal that cannot be saved must not be replaced by nagging.
- **Already starred.** When the opt-in direct flow is enabled and the authenticated `gh` account already starred the repository, the offer marks itself dismissed without ever showing.

## The offer

When the idle conditions are met, a footer line appears under the current screen:

```
Enjoying Gentle AI? Ctrl+G stars https://github.com/Gentleman-Programming/gentle-ai on GitHub, Alt+D never asks again
```

Only two chords are intercepted while the offer is visible — plain typing always passes through untouched:

- `Ctrl+G` — star now.
- `Alt+D` — never ask again.

## Starring

By default `Ctrl+G` opens `https://github.com/Gentleman-Programming/gentle-ai` in the browser; nothing touches stored credentials.

To enable the one-chord flow (star directly with the authenticated `gh` account, t3code-style), set `direct_star` in `star-prompt.json` inside the gentle-ai state directory (`~/.gentle-ai/`):

```json
{
	"schema": "gentle-ai.star-prompt/v1",
	"completed_ops": 12,
	"dismissed": false,
	"direct_star": true
}
```

With `direct_star: true`, the TUI reads starred state and stars through `gh api` after the explicit `Ctrl+G` keypress; if `gh` is missing or fails, it falls back to opening the browser, and if both fail it shows the URL so the chord never dead-ends.

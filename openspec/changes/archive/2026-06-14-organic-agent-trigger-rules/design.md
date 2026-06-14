# Design: Organic Agent Trigger Rules

## Technical Approach

Add a declarative trigger-rules layer as **data + pure renderer + one injection step**, reusing the exact pattern the `strict-tdd-mode` marker section already uses (`internal/components/sdd/inject.go:274-314`). Rules are authored as Go structs (no parser, no new dependency), shipped as a built-in default set in a new `internal/catalog/triggers.go`, rendered by a pure function into a short, scannable instructional block, and injected idempotently into every supported agent through the existing `filemerge.InjectMarkdownSection` marker engine under a new section ID `trigger-rules`. gentle-ai stays an installer: it renders the `when` condition as plain instruction text; it never evaluates it. ZERO runtime, ZERO execution, ZERO new package, ZERO new third-party dependency.

The injection reuses `filemerge` unchanged. Markers are derived from the section ID string (`openMarker`/`closeMarker` in `section.go:198-205`), so a new section ID needs **no new marker constant** and **no edit to `section.go`** — the proposal's note that `internal/filemerge/section.go` may need a marker is incorrect on two counts: the real path is `internal/components/filemerge/section.go`, and the marker mechanism is already generic.

## Architecture Decisions

### Decision: Author rules as Go data, render to text; no authoring-format parser this slice

**Choice**: The built-in default rule set is authored as Go structs in `internal/catalog/triggers.go` (alongside `skills.go`). No YAML/TOML/JSON authoring file is read at runtime. User override authoring is **deferred** — the schema is defined `encoding/json`-friendly (struct tags) so a future change can load a JSON override file with the standard library, but this slice ships defaults only.

**Alternatives considered**: (a) Ship a YAML authoring file — rejected, no parser in `go.mod` and adding one violates the no-new-dependency constraint. (b) Ship a JSON override file now — rejected as scope creep; the proposal lists user overrides as "(later)". (c) Render directly from hardcoded strings with no struct model — rejected because it loses validation, testability, and the future override path.

**Rationale**: Go structs give compile-time safety, table-driven testability, and a clean seam for a future JSON loader without committing to file I/O or a parser today. The struct→text renderer is the only thing the installer needs.

### Implementation Locations

| Component | Location |
|-----------|----------|
| Type model | `internal/model/types.go` |
| Supported events catalog | `internal/catalog/triggers.go` (new) |
| Binding schema validator | `internal/catalog/triggers.go` |
| Default rule set | `internal/catalog/triggers.go` |
| Renderer | `internal/components/sdd/triggerrules.go` (new) |
| Injection integration | `internal/components/sdd/inject.go` (modified, step 1c) |
| Kimi template update | `internal/assets/kimi/KIMI.md` (modified, add include) |
| Tests | `internal/model/types_test.go`, `internal/catalog/triggers_test.go`, `internal/components/sdd/triggerrules_test.go`, `internal/components/sdd/inject_test.go` |
| Golden files | `internal/testdata/golden/trigger-rules-default.golden` (new) |

## Contracts

```go
// internal/model/types.go
type TriggerEvent string
type TriggerMode string
type TriggerWhen struct { /* Always, PathGlobs, MinDiffLines, Phases, Combine */ }
type TriggerBinding struct { On TriggerEvent; When TriggerWhen; Run []string; Mode TriggerMode; Reason string }
type TriggerRuleSet struct { Events []TriggerEvent; Bindings []TriggerBinding }

// internal/catalog/triggers.go
func DefaultTriggerRuleSet() model.TriggerRuleSet
func SupportedTriggerEvents() []model.TriggerEvent      // closed event set
func KnownAgents() []string                              // closed agent set (4R + judgment-day + sdd-* phases)
func ValidateTriggerRuleSet(set model.TriggerRuleSet) error // rejects unknown on/run/mode/when

// internal/components/sdd/triggerrules.go
func RenderTriggerRules(set model.TriggerRuleSet) string

// internal/components/sdd/inject.go — injected section identifier
const sectionTriggerRules = "trigger-rules" // -> <!-- gentle-ai:trigger-rules -->
```

## Data Flow

```
install / sync ──► sdd.Inject(adapter)  [runs once per adapter in the registry]
                      │
                      │  catalog.DefaultTriggerRuleSet()  (Go data, no I/O)
                      ▼
              sdd.RenderTriggerRules(set)  ──► deterministic Markdown block (pure func)
                      │
        ┌─────────────┼───────────────────────────────────┐
        ▼             ▼                                     ▼
  Jinja agents   system-prompt agents                OpenCode / Kilocode
   write module +   InjectMarkdownSection(            append marker-wrapped block
   {% include %}    existing,"trigger-rules",block)   to gentle-orchestrator prompt
        └─────────────┴───────────────────────────────────┘
                      ▼
          idempotent: re-run REPLACES the marker section (no dup)
```

## Testing Strategy (Strict TDD — `go test ./...`)

Behavior-first, table-driven. Each unit lands test-first.

| Layer | What to prove | Approach |
|-------|---------------|----------|
| Schema (`model`) | Events and modes are exactly the closed sets; every default binding's `On` is a member of `SupportedTriggerEvents()`; every `Run` entry is non-empty | Table-driven assertions over the const sets and the default set |
| Default-set token shape (`catalog`) | `pre-commit` AND `pre-push` each run exactly one advisory lens with `When.Always`; `pre-pr` 4R fan-out is gated by `PathGlobs` OR `MinDiffLines>=400`; `judgment-day` only under `post-sdd-phase` with `Phases ⊆ {design,apply}`; `on-ci`/`on-schedule` have zero default bindings | Table-driven; assert binding shapes, reason presence, empty-binding events, and copy-isolation |
| Validation (`catalog`) | `ValidateTriggerRuleSet(DefaultTriggerRuleSet())` returns nil; unknown `Run`, `On`, `Mode`, `When`/`Phases` each return an error | Table-driven over good/bad sets |
| Renderer (`sdd`) | Deterministic output (golden); mode renders as `strongly recommend` vs `consider`; `when` phrasing matches closed vocabulary; "organic, not a gate" note present; marker-free | Golden file, plus focused table tests |
| Injection (`sdd`) | System prompt contains marker section after `Inject`; running `Inject` twice is idempotent (no dup); Jinja adapter writes module; OpenCode path embeds in orchestrator prompt | `t.TempDir()` home, assert file content and marker count |

No TUI work — this change touches no TUI surface.

## Migration / Rollout

Purely additive and injection-based. Old installs gain the section on next sync; reverting and re-syncing strips it via the empty-content branch. No runtime state, no migrations, no executed side effects. `go build ./...`, `go vet ./...`, `go test ./...` must pass clean.

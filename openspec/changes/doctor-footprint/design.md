# Design: Managed Block Footprint Diagnostic for `gentle-ai doctor`

Change: `doctor-footprint` · Issue: #1018 · Approach: Exploration Approach 2 (always-on compact check + opt-in `--footprint`).

## 0. Scope of this document

Concrete architecture for six work units: (1) generic section scanner in `filemerge`, (2) token-estimate helper, (3) `checkManagedBlockFootprint`, (4) `--footprint` flag plumbing + `RunDoctor` signature change, (5) rendering split, (6) test design. Each is specified down to signatures, branches, and seams so `sdd-tasks` can slice a RED → GREEN → REFACTOR sequence (`strict_tdd: true`).

**No sequence diagrams.** Confirmed against exploration §"Current State": doctor is synchronous, single-threaded, plain `fmt.Fprintf` text output — not Bubbletea/lipgloss, no async/message flow. `config.yaml` `rules.design` requires sequence diagrams only for "complex TUI flows"; this is neither TUI nor complex. The rule's other clause — "document architecture decisions with rationale" — is satisfied by the ADRs in §7.

## 1. Component map & data flow

```
app.go "doctor" case
  └─ cli.RunDoctor(ctx, args, w)
       ├─ ParseDoctorFlags(args) ────────────► *DoctorFlags{Footprint bool}      [WU4]
       ├─ osUserHomeDirDoctor()  (existing seam)
       ├─ checkToolBinaries / checkStateJSON / checkEngramReachable / checkDiskSpace  (unchanged)
       ├─ collectFootprint(homeDir) ─────────► FootprintSummary                  [WU3]
       │     ├─ newDoctorRegistry()  (new seam) ─► *agents.Registry
       │     ├─ state.Read(homeDir).InstalledAgents      (reused seam-free source)
       │     ├─ registry.Get(model.AgentID(id)) ─► Adapter        (replaces agentConfigDir switch)
       │     ├─ Adapter.SystemPromptFile(homeDir) ─► path
       │     ├─ os.ReadFile(path)  (rooted under overridable homeDir)
       │     └─ filemerge.ScanSections(content) ─► ScanResult{Sections, Anomalies}  [WU1]
       │            └─ estimateTokens(chars)  [WU2]
       ├─ footprintCheckResult(summary) ─────► CheckResult (compact, always-on)   [WU3/WU5]
       ├─ renderDoctorReport(w, report)   (UNCHANGED)
       └─ if flags.Footprint: renderFootprintDetail(w, summary)   (opt-in table)  [WU5]
```

Boundary: `filemerge` stays a pure text/marker library (knows sections, not "tokens" or "doctor"); token heuristic and all presentation live in `cli`. `agents`/`state` are reused read-only.

---

## 2. WU1 — Generic section scanner (`internal/components/filemerge/section_scan.go`)

New sibling file to `section.go` (keeps `section.go` focused on injection; scanner is a distinct read-only concern). Reuses the existing package constants `markerPrefix` (`<!-- gentle-ai:`), `markerSuffix` (` -->`), `closePrefix` (`<!-- /gentle-ai:`) — single source of truth for marker syntax.

### Types & signature

```go
// Section is one matched gentle-ai marker pair.
type Section struct {
	ID        string // marker id, e.g. "persona" (never a whitelist — whatever the file contains)
	Content   string // raw text between open and close markers, markers excluded
	StartLine int    // 1-based line number of the opening marker
	EndLine   int    // 1-based line number of the closing marker
	CharCount int    // len(Content) in bytes
	LineCount int    // number of content lines (0 when Content == "")
}

// AnomalyKind classifies a structural marker defect.
type AnomalyKind string

const (
	AnomalyOrphanOpen  AnomalyKind = "orphan-open"  // opener with no matching closer (unbounded block)
	AnomalyOrphanClose AnomalyKind = "orphan-close" // closer with no preceding opener
	AnomalyMismatch    AnomalyKind = "mismatch"     // closer id != currently-open id
)

// Anomaly is a structural marker defect surfaced by ScanSections.
type Anomaly struct {
	Kind AnomalyKind
	ID   string // id on the offending marker
	Line int    // 1-based line of the offending marker
}

// ScanResult is the full outcome of one pass over a markdown file.
type ScanResult struct {
	Sections  []Section
	Anomalies []Anomaly
}

// ScanSections finds every <!-- gentle-ai:ID --> ... <!-- /gentle-ai:ID --> pair
// in content, generically (no id whitelist), plus any structural anomalies.
func ScanSections(content string) ScanResult
```

**Deviation from the task-brief signature `func ScanSections(content string) []Section`, with rationale:** the always-on check's success criterion — "orphan/unclosed marker surfaces as WARN/FAIL" — requires orphan data. Returning `ScanResult` (sections + anomalies) from a single pass avoids a second parse and keeps the WARN/FAIL signal co-located with the measurement. A bare `[]Section` cannot carry the breakage signal.

### Algorithm (line-based, single pass, O(n))

1. Split on `\n`; track 1-based line numbers. Preserve `\r` handling by trimming a trailing `\r` per line before marker matching.
2. For each line, classify by `strings.TrimSpace(line)`:
   - **Closer** if it has prefix `closePrefix` and suffix `markerSuffix` → extract `ID = strings.TrimSuffix(strings.TrimPrefix(trimmed, closePrefix), markerSuffix)`.
   - **Opener** if it has prefix `markerPrefix`, suffix `markerSuffix`, and is NOT a closer → same extraction against `markerPrefix`. (Order matters: test closer first, because `closePrefix` shares no prefix with `markerPrefix` but the guard makes intent explicit.)
   - Otherwise a content line.
3. Flat pairing (the injector never nests, per `section.go`):
   - Opener while another opener is already active → emit `Anomaly{AnomalyOrphanOpen, prevID, prevLine}`, then start the new open context.
   - Closer with no active opener → emit `Anomaly{AnomalyOrphanClose, id, line}`.
   - Closer whose `id` != active opener id → emit `Anomaly{AnomalyMismatch, id, line}`; discard the active opener (do not emit a Section).
   - Closer matching the active opener id → emit a `Section` (Content = lines strictly between the two markers joined by `\n`; StartLine = opener line; EndLine = closer line; CharCount = `len(Content)`; LineCount = `0` when empty else `strings.Count(Content,"\n")+1`).
4. End of file with an active opener still open → `Anomaly{AnomalyOrphanOpen, id, line}`.

Generic by construction: IDs are read from the markers, never compared to a known list — so a new `gentle-ai:whatever` block is measured the day it is added (mitigates the top risk: scanner rot).

---

## 3. WU2 — Token estimate helper

```go
// estimateTokens returns a rough token count using the ~4-chars/token heuristic.
// Intentionally coarse: doctor labels every token figure "rough estimate" and no
// model-specific tokenizer is a dependency (out of scope per proposal).
func estimateTokens(chars int) int { return chars / 4 }
```

**Placement decision — `internal/cli/doctor.go` (unexported), NOT `filemerge`.** Rationale: "tokens" is a doctor/UX presentation concept, not a text-merge concept. `filemerge` measures bytes and lines (structural facts); mapping bytes → an LLM token estimate is a diagnostic interpretation that belongs with the consumer. Keeping it in `cli` preserves `filemerge`'s single responsibility and avoids leaking an LLM-cost abstraction into a generic markdown library. No reuse exists elsewhere today; if a second consumer appears, promote it then (YAGNI).

---

## 4. WU3 — `checkManagedBlockFootprint` / `collectFootprint`

Split into a **collector** (does the IO + measurement, returns data) and a **classifier** (`CheckResult`) so the detailed renderer and the compact line share one computation and each piece is unit-testable in isolation.

### New seam

```go
// Overridable for testing (added to the existing var block in doctor.go).
var newDoctorRegistry = agents.NewDefaultRegistry
```

This is **the only new seam var required.** Rationale: it lets a test inject a *subset* registry (`agents.NewRegistry(claude.NewAdapter())`) instead of coupling the footprint test to all 16 production adapters and their real config paths, and lets us exercise the registry-construction-error branch. It directly replaces `checkStateJSON`'s hardcoded `agentConfigDir` switch — resolution now flows through `registry.Get(model.AgentID(id))` + `Adapter.SystemPromptFile(homeDir)`, the same abstraction install/sync use.

**No new file-read seam is needed.** The existing `osUserHomeDirDoctor` seam already redirects `$HOME` to a `t.TempDir()`, and `SystemPromptFile(homeDir)` is rooted at that home — so tests inject "fake instruction file contents" simply by writing real fixture files under the temp home (exactly how `TestCheckStateJSON`/`TestRunDoctor` already work). `collectFootprint` reads via plain `os.ReadFile`; no interception layer.

### Data structures (in `doctor.go`)

```go
type AgentFootprint struct {
	AgentID    string
	Path       string              // "" when the agent id had no adapter
	Unresolved bool                // state listed an id with no registered adapter
	Present    bool                // instruction file existed and was read
	Sections   []filemerge.Section
	Anomalies  []filemerge.Anomaly
	CharCount  int                 // sum of section CharCounts
	LineCount  int                 // sum of section LineCounts
	TokenEst   int                 // estimateTokens(CharCount)
}

type FootprintSummary struct {
	Agents        []AgentFootprint
	TotalBlocks   int
	AgentsCovered int  // agents with >=1 measured block
	TotalChars    int
	TotalTokenEst int
	HasFail       bool // any AnomalyOrphanOpen or AnomalyMismatch anywhere
	HasWarn       bool // any AnomalyOrphanClose anywhere
	StateMissing  bool // state.json absent (first-time install)
	NoAgents      bool // state present but InstalledAgents empty
}
```

### Collector

```go
func collectFootprint(homeDir string) FootprintSummary
```

Flow:
1. `reg, err := newDoctorRegistry()` — on error, return a summary flagged so the classifier emits a single WARN (registry construction never fails in practice; defensive).
2. `s, err := state.Read(homeDir)` — mirror `checkStateJSON`: `os.IsNotExist` → `StateMissing=true` (WARN, "expected for first-time install"); other error → surfaced as WARN via classifier. `len(InstalledAgents)==0` → `NoAgents=true`.
3. For each `id` in `s.InstalledAgents`:
   - `adapter, ok := reg.Get(model.AgentID(id))`; `!ok` → `AgentFootprint{AgentID:id, Unresolved:true}`, continue.
   - `path := adapter.SystemPromptFile(homeDir)`. `os.ReadFile(path)`; `os.IsNotExist` → `Present:false` (agent installed but instruction file not written yet — informational, not breakage). Other read error → treat as absent + note (does not fail the whole check).
   - `res := filemerge.ScanSections(string(data))`; populate `Sections`, `Anomalies`, sums, `TokenEst`.
4. Aggregate totals; set `HasFail`/`HasWarn` from anomaly kinds across all agents.

### Classifier (the always-on `CheckResult`)

```go
func footprintCheckResult(sum FootprintSummary) CheckResult
```

`Name = "managed:footprint"` (colon namespace matches `state:json`, `disk:space`). Status precedence:
- `StateMissing` / `NoAgents` / registry error → **Warn** with the matching remedy (`Run 'gentle-ai install'` / `sync`), Detail explains why footprint is unavailable.
- `HasFail` (orphan-open or mismatch) → **Fail**, Detail names the affected agent(s) + marker id(s), Remedy `Run 'gentle-ai sync' to repair marker boundaries`. Rationale for FAIL: an unclosed opener / id mismatch is the exact `#301` corruption class — the injector would append duplicate blocks on the next sync, so it is actively harmful, not cosmetic.
- else `HasWarn` (orphan-close only) → **Warn**, `stray closing marker` Detail, same sync remedy. Rationale for WARN-not-FAIL: a lone closer is inert (the injector's `stripOrphanMarkers` removes it) — worth flagging, not breaking.
- else → **Pass**, compact Detail: `N block(s) across M agent(s), ~T tokens (rough estimate)`.

The compact line is a normal `CheckResult` appended to `report.Checks`, so it appears in the standard list on **every** `doctor` run.

---

## 5. WU4 — `--footprint` flag + signature change

### Flag parsing (mirrors `ParseSyncFlags`, `sync.go:74`)

```go
type DoctorFlags struct {
	Footprint bool
}

func ParseDoctorFlags(args []string) (*DoctorFlags, error) {
	opts := &DoctorFlags{}
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{}) // reuse existing cli.ioDiscard (install.go:99)
	fs.BoolVar(&opts.Footprint, "footprint", false, "show per-agent/per-block managed block footprint breakdown")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected doctor argument %q", fs.Arg(0))
	}
	return opts, nil
}
```

Follows the `flag.NewFlagSet(name, flag.ContinueOnError)` + `ioDiscard{}` + unexpected-arg guard shape exactly. Returns `*DoctorFlags` per the task brief (minor divergence from `ParseSyncFlags`'s value return — pointer is fine and keeps the nil-on-error contract explicit).

### `RunDoctor` signature change

```go
// before: func RunDoctor(ctx context.Context, w io.Writer) error
func RunDoctor(ctx context.Context, args []string, w io.Writer) error {
	flags, err := ParseDoctorFlags(args)
	if err != nil {
		return err
	}
	homeDir, err := osUserHomeDirDoctor()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}

	report := DoctorReport{}
	report.Checks = append(report.Checks, checkToolBinaries(pathDirsFn())...)
	report.Checks = append(report.Checks, checkStateJSON(homeDir))
	report.Checks = append(report.Checks, checkEngramReachable())
	report.Checks = append(report.Checks, checkDiskSpace(homeDir))

	summary := collectFootprint(homeDir)
	report.Checks = append(report.Checks, footprintCheckResult(summary))

	renderDoctorReport(w, report)
	if flags.Footprint {
		renderFootprintDetail(w, summary)
	}
	return nil
}
```

### Call-site update (`internal/app/app.go:230-231`)

```go
case "doctor":
	return cli.RunDoctor(context.Background(), args[1:], stdout)
```

`ctx` is currently unused inside `RunDoctor` (kept for signature stability); passing `args[1:]` matches every other subcommand's forwarding. **Land in the same work unit** (proposal risk): the two existing integration tests `TestRunDoctor` (`doctor_test.go:476`) and `TestRunDoctor_HomeDirError` (`:498`) call `RunDoctor(ctx, &buf)` and MUST migrate to `RunDoctor(ctx, nil, &buf)` in the same commit or the package won't compile.

---

## 6. WU5 — Rendering split

**Decision: do NOT modify `renderDoctorReport`; add a sibling `renderFootprintDetail`, gated at the `RunDoctor` call site.**

```go
// renderFootprintDetail prints the per-agent/per-block table. Only called when
// --footprint is set. Compact summary already rendered by renderDoctorReport
// via the managed:footprint CheckResult.
func renderFootprintDetail(w io.Writer, sum FootprintSummary)
```

Output shape (plain `fmt.Fprintf`, consistent with existing icons/columns):

```
Managed block footprint (rough estimate)
----------------------------------------
  claude-code  ~/.claude/CLAUDE.md
    persona            412 lines   9210 chars   ~2302 tokens
    engram-protocol    120 lines   3040 chars    ~760 tokens
    sdd-orchestrator    18 lines    540 chars    ~135 tokens
    subtotal           550 lines  12790 chars   ~3197 tokens
  opencode  ~/.config/opencode/AGENTS.md
    (no managed blocks / file absent)
  ----------------------------------------
  TOTAL              N blocks · M agents · ~T tokens (rough estimate)
```

Unresolved agents and absent files render an explicit one-line note rather than being silently dropped (diagnostics must be honest about gaps).

**Rationale for the split over threading a `bool` into `renderDoctorReport`:** `renderDoctorReport` is already unit-tested and has one job (render the flat check list + summary). Adding a mode flag would entangle two concerns and force edits to its existing tests. A separate function keeps single responsibility, leaves the existing renderer and its tests untouched, and puts the opt-in gate where the flag already lives (the call site). Cost: one extra header block — acceptable.

---

## 7. Architecture Decision Records

- **ADR-1 — Generic marker scan, not an ID whitelist.** There is no central registry of section IDs (exploration §"Managed Block Mechanism"); IDs are string literals scattered across components. A whitelist rots the moment a block is added. Scanner reads IDs from the markers themselves. *Rejected:* importing `uninstall/service.go`'s inline list (not exported, and would still need maintenance).
- **ADR-2 — `ScanResult{Sections, Anomalies}` over `[]Section`.** The WARN/FAIL-on-breakage success criterion needs orphan data; one pass, one return value, signal co-located with measurement. *Rejected:* bare slice + a second `ScanAnomalies` pass (double parse, drift risk).
- **ADR-3 — Resolve paths via `registry.Get` + `SystemPromptFile`, not a path switch.** Reuses the install/sync abstraction and actively *removes* the duplication smell `checkStateJSON` embodies (`agentConfigDir`). *Note:* this design does not refactor `checkStateJSON` itself (out of scope) but avoids repeating its mistake.
- **ADR-4 — Token helper in `cli`, not `filemerge`.** Preserves `filemerge`'s boundary as a byte/line text library; token estimation is doctor-side interpretation. (§3)
- **ADR-5 — Separate `renderFootprintDetail`, `renderDoctorReport` untouched.** Single responsibility, zero blast radius on existing renderer tests. (§6)
- **ADR-6 — One new seam only (`newDoctorRegistry`); reuse `osUserHomeDirDoctor` + temp-home real files for content injection.** Matches the established doctor test doctrine; no bespoke file-read interceptor. (§4)
- **ADR-7 — orphan-open/mismatch = FAIL, orphan-close = WARN.** Mapped to the injector's real failure modes (#301 duplicate-append vs inert stray closer). (§4)

## 8. Test design (WU6) — `internal/cli/doctor_test.go` + new `filemerge` test

All table-driven, stdlib `testing`, var-swap-and-`defer`-restore. No mock framework.

### 8.1 `filemerge` scanner (`section_scan_test.go`, pure — no seams)

`TestScanSections` table, one struct `{name, input, wantSections []Section, wantAnomalies []Anomaly}`:
- single well-formed block → 1 Section, correct StartLine/EndLine/CharCount/LineCount, 0 anomalies.
- multiple distinct blocks in one file → N Sections in document order.
- empty-content block (open immediately followed by close) → Section with CharCount 0, LineCount 0.
- unclosed opener at EOF → `AnomalyOrphanOpen`, 0 Sections.
- stray closer, no opener → `AnomalyOrphanClose`.
- closer id ≠ opener id → `AnomalyMismatch`, no Section.
- unknown/novel id (`gentle-ai:brand-new`) → measured normally (proves genericity).
- markers with leading indentation / trailing `\r` → still matched.
- file with zero markers → empty ScanResult.

### 8.2 `collectFootprint` / `footprintCheckResult` (uses seams)

Seam setup per case: `defer` restore `osUserHomeDirDoctor` and `newDoctorRegistry`; `homeDir := t.TempDir()`; write `~/.gentle-ai/state.json` via `state.Write` (or raw JSON like the existing test); set `newDoctorRegistry` to a subset registry, get each adapter, write its `SystemPromptFile` fixture under the temp home.

`TestCollectFootprint` / `TestFootprintCheckResult` cases:
- healthy: two agents, well-formed blocks → Pass, Detail has `block`, `agent`, `~` + `token`, correct totals.
- orphan-open in one agent's file → `HasFail`, classifier → Fail, Detail names agent+id, non-empty Remedy.
- stray closer only → `HasWarn`, classifier → Warn.
- state missing (`os.IsNotExist`) → `StateMissing`, Warn + install remedy.
- empty `InstalledAgents` → `NoAgents`, Warn.
- state lists an unknown agent id → `AgentFootprint.Unresolved`, not counted in AgentsCovered, no panic.
- installed agent whose instruction file is absent → `Present:false`, Pass overall (informational).

### 8.3 `ParseDoctorFlags`

`TestParseDoctorFlags` table `{args, wantFootprint, wantErr}`:
- `nil` / `[]` → `Footprint:false`, no err.
- `["--footprint"]` → `true`.
- `["--footprint=false"]` → `false`.
- `["--bogus"]` → err (unknown flag).
- `["extra"]` → err (`unexpected doctor argument`).

### 8.4 `renderFootprintDetail`

`TestRenderFootprintDetail`: feed a fixed `FootprintSummary`, assert the buffer contains the header, a per-agent line, per-block `lines`/`chars`/`~…tokens`, the `TOTAL` line, and the absent-file note. Table over {healthy, absent-file, unresolved-agent}.

### 8.5 Signature-migration (existing tests)

Update `TestRunDoctor` (`:476`) and `TestRunDoctor_HomeDirError` (`:498`) call sites to `RunDoctor(ctx, nil, &buf)`. Add `TestRunDoctor_FootprintFlag`: temp home + subset registry + fixture files, run with `[]string{"--footprint"}`, assert detail table present; run with `nil`, assert compact `managed:footprint` line present but no detail header.

### RED → GREEN → REFACTOR sequencing hint for `sdd-tasks`

Per work unit, in dependency order: WU1 scanner (RED tests 8.1 → GREEN scanner) → WU2 helper (trivial, fold into WU3) → WU3 collector+classifier (RED 8.2 → GREEN) → WU4 flag+signature (RED 8.3 + migrate 8.5 → GREEN, compiles) → WU5 render (RED 8.4 → GREEN) → WU4/WU6 integration `TestRunDoctor_FootprintFlag`. `go vet ./...`, `gofmt`, `go test ./...` gate at the end (`config.yaml`).

## 9. Residual risks / assumptions

- **Signature change ordering** — WU4 does not compile until the two existing `RunDoctor` call sites (app.go + 2 tests) are migrated in the same commit. Flagged as the sequencing constraint above.
- **`config.yaml strict_tdd: true` vs proposal note** — proposal §Dependencies flags a possible task-brief contradiction; this repo's config is authoritative → design is TDD-sliceable. Confirm at `sdd-tasks`.
- **Assumption: `state.InstalledAgents` strings equal `model.AgentID` constant values** — verified against `model/types.go` (`"claude-code"` etc.). If a legacy state file holds a stale id, it lands as `Unresolved` (handled, no panic).
- **Assumption: markers occupy their own line** — true for all injector output (`section.go`); the scanner tolerates indentation/`\r` but does not attempt to parse two markers on one physical line (never produced by the injector).

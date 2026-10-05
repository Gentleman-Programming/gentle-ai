//go:build bench_fixture

package reviewtransaction

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// benchCrashAtPhaseFired makes the deterministic crash-position hook below
// fire at most once per process. Every bench journey step launches this
// product as a brand-new subprocess, so this package-level flag starts
// fresh false on every invocation — the crash-inducing run and the
// following resume run are always separate processes, so no explicit reset
// between them is needed or possible.
var benchCrashAtPhaseFired bool

// init overrides compactReclaimPhaseHook — the exact seam
// TestAuthorityDispositionResumeCrashPositionMatrix (authority_disposition_resume_test.go)
// already overrides in-process to prove convergence at every one of a
// 3-node closure's 6 ordered positions — so a bench journey compiled with
// this build tag can drive the identical, deterministic interruption
// through the real `review repair` binary instead of authoring a
// pre-broken on-disk state (Wave 6 fix cycle 2, WARNING: "ds11 authors
// rather than interrupts").
//
// GENTLE_AI_BENCH_CRASH_AT_PHASE, formatted "<phase>:<lineage_id>", names
// the exact (phase, lineage) pair the Go matrix's own `positions`/`targetLineage`
// pair names. On a match the process refuses to continue past that phase
// for that lineage — a genuine interruption of the real command, not a
// hand-authored on-disk state — with an error distinct from every
// production refusal, so the crash-inducing bench step can assert it saw
// exactly this and nothing else. Ordinary product binaries never link this
// file, so GENTLE_AI_BENCH_CRASH_AT_PHASE has no effect outside a binary
// deliberately built with `-tags bench_fixture` for this exact proof.
func init() {
	original := compactReclaimPhaseHook
	compactReclaimPhaseHook = func(ctx context.Context, current string, record CompactReclaimRecord) error {
		if target := os.Getenv("GENTLE_AI_BENCH_CRASH_AT_PHASE"); target != "" && !benchCrashAtPhaseFired {
			if phase, lineage, found := strings.Cut(target, ":"); found && current == phase && record.LineageID == lineage {
				benchCrashAtPhaseFired = true
				// refusal:by-design world-action: this is a deliberately-injected simulated fault for bench crash-position verification, not a genuine product refusal — it exists only in a binary built with `-tags bench_fixture` and only when a bench journey set GENTLE_AI_BENCH_CRASH_AT_PHASE; the "resolution" is that the caller (the crash-inducing bench step) asked for exactly this, so no product-side command could honestly be named
				return fmt.Errorf("bench_fixture: deterministic crash injected at phase %q for lineage %q", phase, lineage)
			}
		}
		return original(ctx, current, record)
	}
}

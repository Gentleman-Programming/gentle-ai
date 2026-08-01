package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

type compactAttemptOutput struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
	Token  string `json:"token,omitempty"`
}

func TestRunSDDAttemptCompactOutputStaysBoundedAcrossHistory(t *testing.T) {
	repo := initReviewCLIRepo(t)
	const change = "compact-history"

	var acquireSize, settleSize int
	for attempt := 1; attempt <= 10; attempt++ {
		acquired, acquirePayload := runCompactSDDAttempt(t, []string{
			"acquire", "--cwd", repo, "--change", change,
			"--request-id", fmt.Sprintf("compact-acquire-%d", attempt),
			"--work-unit", "runtime-proof", "--evidence-goal", "prove compact orchestration",
			"--max-attempts", "12", "--max-changed-lines", "200",
		})
		if acquired.State != "proceed" || acquired.Reason != "" || !strings.HasPrefix(acquired.Token, "sha256:") {
			t.Fatalf("acquire %d = %#v", attempt, acquired)
		}
		assertCompactPayloadKeys(t, acquirePayload, "state", "token")
		if attempt == 1 {
			acquireSize = len(acquirePayload)
		} else if len(acquirePayload) != acquireSize {
			t.Fatalf("acquire output grew from %d to %d bytes at attempt %d", acquireSize, len(acquirePayload), attempt)
		}

		settled, settlePayload := runCompactSDDAttempt(t, []string{
			"settle", "--cwd", repo, "--change", change, "--token", acquired.Token,
			"--request-id", fmt.Sprintf("compact-settle-%d", attempt), "--outcome", "failed",
			"--evidence-revision", cliAttemptHash(byte('a' + attempt%6)),
			"--diagnosis", "bounded execution produced retryable evidence", "--harness-disposition", "reused",
			"--cleanup-evidence", "process group exited", "--process-evidence", "no descendants remained",
		})
		if settled != (compactAttemptOutput{State: "proceed"}) {
			t.Fatalf("settle %d = %#v", attempt, settled)
		}
		assertCompactPayloadKeys(t, settlePayload, "state")
		if attempt == 1 {
			settleSize = len(settlePayload)
		} else if len(settlePayload) != settleSize {
			t.Fatalf("settle output grew from %d to %d bytes at attempt %d", settleSize, len(settlePayload), attempt)
		}
	}

	var legacy bytes.Buffer
	if err := RunSDDAttempt([]string{"status", "--cwd", repo, "--change", change}, &legacy); err != nil {
		t.Fatal(err)
	}
	var status sddstatus.RuntimeStatus
	if err := json.Unmarshal(legacy.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Attempts) != 10 || acquireSize > 160 || settleSize > 80 || acquireSize >= legacy.Len() {
		t.Fatalf("bounded sizes acquire=%d settle=%d legacy=%d attempts=%d", acquireSize, settleSize, legacy.Len(), len(status.Attempts))
	}
}

func TestRunSDDAttemptLegacyStatusJSONIsUnchanged(t *testing.T) {
	repo := initReviewCLIRepo(t)
	var output bytes.Buffer
	if err := RunSDDAttempt([]string{"status", "--cwd", repo, "--change", "legacy-json"}, &output); err != nil {
		t.Fatal(err)
	}
	want := `{
  "schema": "gentle-ai.sdd-runtime-status/v1",
  "change": "legacy-json",
  "revision": "",
  "attempts": [],
  "objective_generation": 0,
  "next_ordinal": 1,
  "cumulative_attempts": 0,
  "cumulative_changed_lines": 0,
  "lifetime_attempts": 0,
  "lifetime_changed_lines": 0,
  "evidence_revision": "",
  "decision_required": false,
  "complete": false,
  "next_action": "begin",
  "binding_revision": ""
}
`
	if output.String() != want {
		t.Fatalf("legacy status JSON changed:\n%s", output.String())
	}
}

func TestRunSDDAttemptCompactBlocksWithoutMutation(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, string, string, sddstatus.RuntimeStore) (args []string, wantReason, wantToken string)
	}{
		{
			name: "active attempt",
			prepare: func(t *testing.T, repo, change string, _ sddstatus.RuntimeStore) ([]string, string, string) {
				started, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "active-owner", 2))
				return compactAcquireArgs(repo, change, "active-contender", 2), "active_attempt", started.Token
			},
		},
		{
			name: "maintainer decision",
			prepare: func(t *testing.T, repo, change string, _ sddstatus.RuntimeStore) ([]string, string, string) {
				started, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "decision-acquire", 1))
				settled, _ := runCompactSDDAttempt(t, compactSettleArgs(repo, change, started.Token, "decision-settle", "failed"))
				if settled.Reason != "maintainer_decision" {
					t.Fatalf("exhausting settle = %#v", settled)
				}
				return compactAcquireArgs(repo, change, "decision-retry", 1), "maintainer_decision", ""
			},
		},
		{
			name: "corrupt authority",
			prepare: func(t *testing.T, repo, change string, store sddstatus.RuntimeStore) ([]string, string, string) {
				runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "corrupt-acquire", 2))
				if err := os.WriteFile(filepath.Join(store.Dir, "HEAD"), []byte("corrupt\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return compactAcquireArgs(repo, change, "corrupt-retry", 2), "corrupt_authority", ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			change := "blocked-" + strings.ReplaceAll(tt.name, " ", "-")
			store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
			if err != nil {
				t.Fatal(err)
			}
			args, wantReason, wantToken := tt.prepare(t, repo, change, store)
			before := snapshotRuntimeAuthorityFiles(t, store.Dir)
			result, payload := runCompactSDDAttempt(t, args)
			after := snapshotRuntimeAuthorityFiles(t, store.Dir)
			if result.State != "blocked" || result.Reason != wantReason || result.Token != wantToken {
				t.Fatalf("blocked result = %#v, want reason=%q token=%q", result, wantReason, wantToken)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("blocked operation mutated authority\nbefore=%v\nafter=%v", before, after)
			}
			keys := []string{"state", "reason"}
			if wantToken != "" {
				keys = append(keys, "token")
			}
			assertCompactPayloadKeys(t, payload, keys...)
		})
	}
}

func TestRunSDDAttemptCompactPreservesTokenCASAndIdempotentReplay(t *testing.T) {
	repo := initReviewCLIRepo(t)
	const change = "compact-replay"
	acquireArgs := compactAcquireArgs(repo, change, "replay-acquire", 2)
	first, firstPayload := runCompactSDDAttempt(t, acquireArgs)
	replayed, replayedPayload := runCompactSDDAttempt(t, acquireArgs)
	if first.State != "proceed" || first.Token == "" || replayed != first || !bytes.Equal(firstPayload, replayedPayload) {
		t.Fatalf("acquire replay first=%#v replayed=%#v", first, replayed)
	}

	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	beforeWrongToken := snapshotRuntimeAuthorityFiles(t, store.Dir)
	wrong, _ := runCompactSDDAttempt(t, compactSettleArgs(repo, change, cliAttemptHash('f'), "wrong-token", "passed"))
	if wrong.State != "blocked" || wrong.Reason != "active_attempt" || wrong.Token != first.Token {
		t.Fatalf("wrong-token settle = %#v", wrong)
	}
	if after := snapshotRuntimeAuthorityFiles(t, store.Dir); !reflect.DeepEqual(beforeWrongToken, after) {
		t.Fatal("wrong-token settle mutated authority")
	}

	settleArgs := compactSettleArgs(repo, change, first.Token, "replay-settle", "passed")
	completed, completedPayload := runCompactSDDAttempt(t, settleArgs)
	completedReplay, completedReplayPayload := runCompactSDDAttempt(t, settleArgs)
	if completed != (compactAttemptOutput{State: "complete"}) || completedReplay != completed || !bytes.Equal(completedPayload, completedReplayPayload) {
		t.Fatalf("settle replay completed=%#v replayed=%#v", completed, completedReplay)
	}
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Attempts) != 1 || status.ActiveAttempt != nil || !status.Complete {
		t.Fatalf("replayed compact lifecycle status = %#v", status)
	}
}

func compactAcquireArgs(repo, change, requestID string, maxAttempts int) []string {
	return []string{
		"acquire", "--cwd", repo, "--change", change, "--request-id", requestID,
		"--work-unit", "compact-unit", "--evidence-goal", "prove compact attempt",
		"--max-attempts", fmt.Sprint(maxAttempts), "--max-changed-lines", "20",
	}
}

func compactSettleArgs(repo, change, token, requestID, outcome string) []string {
	return []string{
		"settle", "--cwd", repo, "--change", change, "--token", token, "--request-id", requestID,
		"--outcome", outcome, "--evidence-revision", cliAttemptHash('e'),
		"--diagnosis", "compact attempt produced conclusive evidence", "--harness-disposition", "reused",
		"--cleanup-evidence", "process group exited", "--process-evidence", "no descendants remained",
	}
}

func runCompactSDDAttempt(t *testing.T, args []string) (compactAttemptOutput, []byte) {
	t.Helper()
	var output bytes.Buffer
	if err := RunSDDAttempt(args, &output); err != nil {
		t.Fatalf("RunSDDAttempt(%v): %v", args, err)
	}
	var result compactAttemptOutput
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode compact SDD attempt: %v\n%s", err, output.String())
	}
	return result, append([]byte(nil), output.Bytes()...)
}

func assertCompactPayloadKeys(t *testing.T, payload []byte, keys ...string) {
	t.Helper()
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	if len(document) != len(keys) {
		t.Fatalf("compact keys = %v, want %v", document, keys)
	}
	for _, key := range keys {
		if _, ok := document[key]; !ok {
			t.Fatalf("compact output missing %q: %s", key, payload)
		}
	}
}

func snapshotRuntimeAuthorityFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snapshot[relative] = string(payload)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return snapshot
}

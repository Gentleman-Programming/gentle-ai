package cli

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// reviewAbandonAuthorizationLines is the number of lines the abandon
// authorization binding carries. The refusal prints the binding template as
// its final lines, so a consumer — human or test — reads them off the end.
const reviewAbandonAuthorizationLines = 9

// TestReviewAbandonRefusalTerminatesInASuccessfulAbandon consumes the refusal
// the product actually emits and nothing else. The command the refusal names
// is run verbatim through the real router, every value is read out of that
// command's own JSON at the field names the refusal prints, the authorization
// is assembled from the template the refusal prints, and the resulting
// review abandon must succeed. No wording is asserted: the only claim under
// test is that a consumer who follows the message reaches a terminal state.
//
// The lineage is deliberately stale — the fixture restores a clean worktree
// after persisting the authority — because that is the case the abandon help
// advertises as still abandonable, and it is the case in which any
// worktree-derived surface would hand the operator the wrong identity.
func TestReviewAbandonRefusalTerminatesInASuccessfulAbandon(t *testing.T) {
	repo, _, _, _, _ := partiallyCapturedReview(t)

	const actor = "maintainer"
	const reason = reviewtransaction.CompactAbandonReasonOperatorDisposition

	var refused bytes.Buffer
	err := RunReview([]string{"abandon", "--cwd", repo}, &refused)
	if err == nil {
		t.Fatalf("review abandon without inputs succeeded: %s", refused.String())
	}
	message := err.Error()

	lookup := runReviewCommandNamedBy(t, message)
	values := reviewLookupValues(t, message, lookup)
	values["--actor"], values["--reason"] = actor, reason
	authorization := assembleReviewAuthorization(t, message, values)

	var abandoned bytes.Buffer
	if err := RunReview([]string{
		"abandon", "--cwd", repo,
		"--lineage", values["--lineage"],
		"--expected-revision", values["--expected-revision"],
		"--reason", reason,
		"--actor", actor,
		"--maintainer-authorization", authorization,
	}, &abandoned); err != nil {
		t.Fatalf("the refusal's own recipe does not abandon: %v\nmessage:\n%s\nauthorization:\n%s", err, message, authorization)
	}
	var result ReviewAbandonResult
	decodeStrictReviewJSON(t, abandoned.Bytes(), &result)
	if result.Record.Status != reviewtransaction.CompactReclaimCommitted {
		t.Fatalf("abandon record status = %q, want committed", result.Record.Status)
	}
	if result.Record.LineageID != values["--lineage"] {
		t.Fatalf("abandon record lineage = %q, want %q", result.Record.LineageID, values["--lineage"])
	}
}

// runReviewCommandNamedBy requires the message to name exactly one runnable
// gentle-ai command, then runs that exact argv through the real router and
// returns its decoded JSON object. A message that names no command, or names
// more than one so the reader must guess, fails here.
func runReviewCommandNamedBy(t *testing.T, message string) map[string]any {
	t.Helper()
	named := []string{}
	for _, line := range strings.Split(message, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "gentle-ai review ") {
			named = append(named, strings.TrimSpace(line))
		}
	}
	if len(named) != 1 {
		t.Fatalf("refusal names %d runnable commands, want exactly one:\n%s", len(named), message)
	}
	argv := strings.Fields(named[0])
	var output bytes.Buffer
	if err := RunReview(argv[2:], &output); err != nil {
		t.Fatalf("the command the refusal named does not run as printed (%q): %v", named[0], err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("the command the refusal named emitted no JSON object: %v", err)
	}
	return envelope
}

// reviewLookupValues reads each "<flag> = entries[].<field>" line the message
// prints and resolves it against the named command's own envelope.
func reviewLookupValues(t *testing.T, message string, envelope map[string]any) map[string]string {
	t.Helper()
	entries, _ := envelope["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("named command reported %d authority entries, want exactly one", len(entries))
	}
	entry, _ := entries[0].(map[string]any)
	values := map[string]string{}
	for field, published := range entry {
		if text, ok := published.(string); ok {
			values["entries[]."+field] = text
		}
	}
	discarded := entry["discarded_work"].(map[string]any)
	capturedResults := discarded["captured_lens_results"].([]any)
	if len(capturedResults) == 0 {
		t.Fatal("refusal fixture did not expose captured lens results")
	}
	capturedLenses := make([]string, len(capturedResults))
	for index, lens := range capturedResults {
		capturedLenses[index] = lens.(string)
	}
	values["entries[].discarded_work.captured_lens_results comma-joined with \",\" in listed order"] = strings.Join(capturedLenses, ",")
	values["entries[].discarded_work.findings_present"] = strconv.FormatBool(discarded["findings_present"].(bool))
	values["entries[].discarded_work.evidence_records_present"] = strconv.FormatBool(discarded["evidence_records_present"].(bool))
	for _, line := range strings.Split(message, "\n") {
		flag, path, found := strings.Cut(strings.TrimSpace(line), " = entries[].")
		if !found || !strings.HasPrefix(flag, "--") {
			continue
		}
		text, ok := entry[path].(string)
		if !ok || text == "" {
			t.Fatalf("refusal points %s at entries[].%s, which the named command does not publish", flag, path)
		}
		values[flag] = text
	}
	for _, required := range []string{"--lineage", "--expected-revision"} {
		if values[required] == "" {
			t.Fatalf("refusal never says where to read %s:\n%s", required, message)
		}
	}
	return values
}

// assembleReviewAuthorization takes the binding template off the end of the
// message and substitutes every <placeholder> it carries, resolving
// entries[].<field> placeholders against the named command's envelope and
// --flag placeholders against the values already in hand. An unresolvable
// placeholder is a message that leaves the reader stranded.
func assembleReviewAuthorization(t *testing.T, message string, values map[string]string) string {
	t.Helper()
	lines := strings.Split(strings.TrimRight(message, "\n"), "\n")
	if len(lines) < reviewAbandonAuthorizationLines {
		t.Fatalf("refusal is shorter than the binding it demands:\n%s", message)
	}
	template := append([]string{}, lines[len(lines)-reviewAbandonAuthorizationLines:]...)
	for index, line := range template {
		for {
			open := strings.Index(line, "<")
			if open < 0 {
				break
			}
			close := strings.Index(line[open:], ">")
			if close < 0 {
				t.Fatalf("binding template line %q carries an unterminated placeholder", line)
			}
			token := line[open+1 : open+close]
			replacement, ok := values[token]
			if !ok {
				t.Fatalf("binding template placeholder <%s> is never sourced by the message:\n%s", token, message)
			}
			line = line[:open] + replacement + line[open+close+1:]
		}
		template[index] = line
	}
	return strings.Join(template, "\n")
}

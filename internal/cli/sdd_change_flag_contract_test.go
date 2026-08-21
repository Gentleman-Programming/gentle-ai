package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// #2116, second half: each command rejected the spelling the other required.
// `sdd-status <change>` was the only shape status took and `--change <change>`
// was the only shape sdd-attempt took, so the change identity had to be
// re-spelled between two commands that resolve the same thing. Both now accept
// both, and the same identity reaches the same place either way.
func TestSDDStatusAcceptsBothChangeSpellings(t *testing.T) {
	for _, args := range [][]string{
		{"with space", "--cwd", "/repo"},
		{"--change", "with space", "--cwd", "/repo"},
		{"--change=with space", "--cwd", "/repo"},
		{"--cwd", "/repo", "--change", "with space"},
	} {
		parsed, err := sddstatus.ParseCommandArgs(args)
		if err != nil {
			t.Fatalf("ParseCommandArgs(%q) error = %v", args, err)
		}
		if parsed.ChangeName != "with space" || parsed.CWD != "/repo" {
			t.Fatalf("ParseCommandArgs(%q) = %+v, want change %q and cwd %q", args, parsed, "with space", "/repo")
		}
	}
}

func TestSDDStatusRefusesTheChangeIdentityTwice(t *testing.T) {
	_, err := sddstatus.ParseCommandArgs([]string{"first", "--change", "second"})
	if err == nil || !strings.Contains(err.Error(), "already set") {
		t.Fatalf("ParseCommandArgs() error = %v, want a refusal naming the identity already set", err)
	}
}

// The unknown-argument refusal has to name the shapes the command does accept.
// The original reporter found the working invocation by trial and error.
func TestSDDStatusUnknownArgumentNamesBothAcceptedShapes(t *testing.T) {
	_, err := sddstatus.ParseCommandArgs([]string{"--mystery"})
	if err == nil {
		t.Fatal("ParseCommandArgs() error = nil, want a refusal")
	}
	for _, want := range []string{"gentle-ai sdd-status <change>", "--change <change>"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %q", err.Error(), want)
		}
	}
}

func TestSDDAttemptAcceptsBothChangeSpellings(t *testing.T) {
	repo := initReviewCLIRepo(t)
	positional := runSDDAttemptStatus(t, []string{"status", "cli-both-shapes", "--cwd", repo})
	flagged := runSDDAttemptStatus(t, []string{"status", "--cwd", repo, "--change", "cli-both-shapes"})
	if positional.Change != "cli-both-shapes" || flagged.Change != positional.Change {
		t.Fatalf("positional change = %q, flag change = %q, want both %q", positional.Change, flagged.Change, "cli-both-shapes")
	}
	if positional.Schema != flagged.Schema || positional.NextAction != flagged.NextAction {
		t.Fatalf("the two spellings reached different states: %#v vs %#v", positional, flagged)
	}
}

// A positional identity that would be mistaken for anything else must still be
// refused rather than guessed at.
func TestSDDAttemptChangeOperandRefusals(t *testing.T) {
	repo := initReviewCLIRepo(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "given twice",
			args: []string{"status", "positional-change", "--cwd", repo, "--change", "flag-change"},
			want: "takes the change identity once",
		},
		{
			// stdlib flag stops at the first non-flag argument, so a
			// positional after a flag would swallow everything behind it.
			// It stays refused, and the refusal names both accepted shapes.
			name: "positional after flags",
			args: []string{"status", "--cwd", repo, "late-change"},
			want: "unexpected sdd-attempt argument",
		},
		{
			name: "refusal names the accepted shapes",
			args: []string{"status", "--cwd", repo, "late-change"},
			want: "gentle-ai sdd-attempt status <change>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			err := RunSDDAttempt(tt.args, &output)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("RunSDDAttempt(%v) err = %v, want %q", tt.args, err, tt.want)
			}
		})
	}
}

// The five identities that #2204 left behind: sdd-status resolved them on
// released v2.4.0 and the attempt gate refused them. Through the real CLI,
// both surfaces now answer for the same identity.
func TestSDDAttemptServesTheIdentitiesSDDStatusResolves(t *testing.T) {
	repo := initReviewCLIRepo(t)
	for _, change := range []string{"3.14", "dot.in.name", "with space", "at@sign", "mixed_Case-1.2"} {
		status := runSDDAttemptStatus(t, []string{"status", "--cwd", repo, "--change", change})
		if status.Change != change {
			t.Fatalf("sdd-attempt status --change %q returned change %q", change, status.Change)
		}
		if status.Schema != sddstatus.RuntimeStatusSchema {
			t.Fatalf("sdd-attempt status --change %q returned schema %q", change, status.Schema)
		}
	}
}

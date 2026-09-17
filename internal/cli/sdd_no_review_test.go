package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"
)

func TestSDDCommandsNeverOfferReview(t *testing.T) {
	for _, mode := range []string{"enabled", "disabled", "unreadable"} {
		t.Run(mode, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := t.TempDir()
			seedArchiveGatedSDDChange(t, repo)
			if mode != "enabled" {
				disableReviewForClone(t, repo)
			}
			if mode == "unreadable" {
				corruptCloneLocalReviewMode(t, repo)
			}
			authorityRoot := filepath.Join(repo, ".git", "gentle-ai")
			before := snapshotAuthorityTree(t, authorityRoot)
			for _, command := range []struct {
				name string
				run  func([]string, io.Writer) error
			}{{"status", RunSDDStatus}, {"continue", RunSDDContinue}} {
				t.Run(command.name, func(t *testing.T) {
					var output bytes.Buffer
					if err := command.run([]string{"thin", "--cwd", repo, "--json"}, &output); err != nil {
						t.Fatal(err)
					}
					var document map[string]json.RawMessage
					if err := json.Unmarshal(output.Bytes(), &document); err != nil {
						t.Fatal(err)
					}
					for _, key := range []string{"reviewOffer", "reviewGate", "reviewTransaction"} {
						if _, present := document[key]; present {
							t.Errorf("SDD exposes retired review field %q", key)
						}
					}
					if string(document["nextRecommended"]) != `"archive"` {
						t.Errorf("nextRecommended = %s, want archive", document["nextRecommended"])
					}
					if after := snapshotAuthorityTree(t, authorityRoot); after != before {
						t.Error("SDD changed review authority or mode")
					}
				})
			}
		})
	}
}

//go:build linux

package shellinstaller

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestUserSharedPreviewDisclosesAgentToolsAndUnpublishedRecovery(t *testing.T) {
	req, _ := userSharedToolFixture(t)
	token, err := InspectUserInstall(req)
	if err != nil {
		t.Fatal(err)
	}
	before := userSnapshot(t, req.SharedAgent)
	preview, err := PreviewUserInstall(req, token)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		filepath.Join(req.SharedAgent, "bin/fd"), filepath.Join(req.SharedAgent, "bin/rg"), "existing fd/rg",
		"gentle-ai shell recover " + req.Destination + " inspect", "WORKSPACE/installed", "never a prefix or agent",
	} {
		if !strings.Contains(preview, want) {
			t.Errorf("shared preview missing %q: %q", want, preview)
		}
	}
	if userSnapshot(t, req.SharedAgent) != before {
		t.Fatal("preview changed selected agent")
	}
}

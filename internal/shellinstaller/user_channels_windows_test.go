//go:build windows

package shellinstaller

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestUserWindowsRetainedSelection(t *testing.T) {
	root := `C:\owned home`
	base, _ := json.Marshal(UserInstallRequest{Destination: root, Mode: "separate", Channel: "stable"})
	if _, err := userWindowsSelection(base, root); err != nil {
		t.Fatal(err)
	}
	legacy := strings.Replace(string(base), `,"Channel":"stable"`, "", 1)
	if _, err := userWindowsSelection([]byte(legacy), root); err == nil {
		t.Fatal("channel-less retained installation was accepted")
	}
	for _, tt := range []struct{ field, value string }{
		{"Destination", `C:\foreign home`}, {"Mode", "shared"}, {"unrecognized", "injected"},
		{"Channel", "nightly"}, {"SharedPrefix", "foreign"},
	} {
		t.Run(tt.field, func(t *testing.T) {
			var fields map[string]string
			if err := json.Unmarshal(base, &fields); err != nil {
				t.Fatal(err)
			}
			fields[tt.field] = tt.value
			data, _ := json.Marshal(fields)
			if tt.field != "unrecognized" {
				var changed UserInstallRequest
				_ = json.Unmarshal(data, &changed)
				data, _ = json.Marshal(changed) // Keep canonical encoding; exercise the field check.
			}
			if req, err := userWindowsSelection(data, root); err == nil || req != (UserInstallRequest{}) {
				t.Fatalf("rewritten selection remains actionable: %+v, %v", req, err)
			}
		})
	}
}

func TestUserWindowsMainArtifactDescriptor(t *testing.T) {
	commit := "aa2c03896be9866ab0af89bbc621d4c2a8fcf9c2"
	sha := strings.Repeat("ab", 32) // Synthetic digest; no provenance claim.
	for _, tt := range []struct {
		name, commit, sha string
		wantErr           bool
	}{
		{"frozen canonical source", commit, sha, false},
		{"alternate frozen source", strings.Repeat("b", 40), strings.Repeat("cd", 32), false},
		{"floating ref", "main", sha, true},
		{"nonhex commit", strings.Repeat("z", 40), sha, true},
		{"missing commit", "", sha, true},
		{"nonhex digest", commit, strings.Repeat("z", 64), true},
		{"short digest", commit, "ab", true},
		{"missing digest", commit, "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := userWindowsMainArtifact(tt.commit, tt.sha)
			if tt.wantErr {
				if err == nil || got != (userWindowsArtifact{}) {
					t.Fatalf("invalid Main metadata returned artifact: %+v, %v", got, err)
				}
				return
			}
			want := userWindowsArtifact{URL: "https://codeload.github.com/Gentleman-Programming/gentle-shell/zip/" + tt.commit, SHA: tt.sha, Archive: "main.zip", Prefix: "gentle-shell-" + tt.commit, Bound: 32 << 20}
			if err != nil || got != want {
				t.Fatalf("frozen Main descriptor = %+v, %v; want %+v", got, err, want)
			}
		})
	}
}

func TestUserWindowsMainAcquisitionFailsClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	artifact, data, err := userWindowsMainSnapshot(ctx)
	if err == nil || artifact != (userWindowsArtifact{}) || data != nil {
		t.Fatalf("canceled acquisition returned authority: %+v, %v", artifact, err)
	}
	req := UserInstallRequest{Destination: `C:\not-created`, Mode: "separate", Channel: "main"}
	mainToken := userConfirmation(req, "fixed physical identity")
	req.Channel = "stable"
	if mainToken == userConfirmation(req, "fixed physical identity") {
		t.Fatal("confirmation does not bind channel")
	}
}

func TestUserWindowsInstallChannels(t *testing.T) {
	base := []string{`C:\owned home`, "separate", "", "", "confirmed"}
	for _, tt := range []struct {
		name    string
		options []string
		channel string
		wantErr bool
	}{
		{name: "default stable", channel: "stable"},
		{name: "explicit stable", options: []string{"--channel", "stable"}, channel: "stable"},
		{name: "explicit main", options: []string{"--channel", "main"}, channel: "main"},
		{name: "unknown channel", options: []string{"--channel", "nightly"}, wantErr: true},
		{name: "missing channel", options: []string{"--channel"}, wantErr: true},
		{name: "empty channel", options: []string{"--channel", ""}, wantErr: true},
		{name: "unknown flag", options: []string{"--unknown", "stable"}, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			args := append(append([]string(nil), base...), tt.options...)
			req, err := UserInstallFromEntry(args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parse error = %v; wantErr %t", err, tt.wantErr)
			}
			if tt.wantErr {
				if req != (UserInstallRequest{}) {
					t.Fatalf("rejected channel returned actionable request: %+v", req)
				}
				return
			}
			if req.Destination != base[0] || req.Mode != base[1] || req.SharedPrefix != base[2] || req.SharedAgent != base[3] || req.Confirmation != base[4] {
				t.Fatalf("channel selection changed install arguments: %+v", req)
			}
			// Inspect the returned value without requiring a not-yet-added field.
			data, err := json.Marshal(req)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]string
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			if got := fields["Channel"]; got != tt.channel {
				t.Fatalf("request Channel = %q; want %q", got, tt.channel)
			}
		})
	}
}

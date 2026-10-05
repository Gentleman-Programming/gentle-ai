package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolicyRuntimeVersionFixture(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    []string
		handled bool
		code    int
		output  string
	}{
		{"unix", []string{"/tmp/opencode", "--version"}, true, 0, "1.18.10\n"},
		{"windows", []string{`C:\fixture\opencode.exe`, "--version"}, true, 0, "1.18.10\n"},
		{"reject runtime call", []string{"opencode", "run"}, true, 2, ""},
		{"reject extra args", []string{"opencode", "--version", "extra"}, true, 2, ""},
		{"bench unchanged", []string{"gentle-ai-bench", "run"}, false, 0, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			handled, code := policyRuntimeVersionFixture(tt.args, &out)
			if handled != tt.handled || code != tt.code || out.String() != tt.output {
				t.Fatalf("got %v %d %q", handled, code, out.String())
			}
		})
	}
}

func TestPolicyRuntimeFixturePathPrecedence(t *testing.T) {
	s, err := newSandbox("gentle-ai", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.PathOverride = filepath.Join(s.Root, "explicit")
	for _, env := range s.env() {
		if strings.HasPrefix(env, "PATH=") {
			parts := strings.Split(strings.TrimPrefix(env, "PATH="), string(os.PathListSeparator))
			if len(parts) < 2 || parts[0] != s.PathOverride || parts[1] != filepath.Join(s.Root, "policy-runtime-bin") {
				t.Fatalf("PATH precedence=%v", parts)
			}
			return
		}
	}
	t.Fatal("missing PATH")
}

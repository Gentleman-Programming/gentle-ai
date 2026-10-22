package gga

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestInjectProviderPreservation(t *testing.T) {
	for _, agents := range [][]model.AgentID{{model.AgentCursor}, {model.AgentClaudeCode}, {model.AgentCursor, model.AgentOpenCode}, {model.AgentOpenCode, model.AgentCursor}} {
		t.Run(strings.TrimSpace(strings.Join(agentNames(agents), "-")), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("APPDATA", filepath.Join(home, "roaming"))
			path := ConfigPath(home)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			original := string(BuildConfig("ollama:future-model"))
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				res, err := Inject(home, agents)
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != original || res.ConfigChanged {
					t.Fatalf("provider overwritten: changed=%v config=%s", res.ConfigChanged, got)
				}
			}
		})
	}
}

// Existing custom config is not managed when selection has no provider mapping.
func TestInjectUnmappedPreservesWholeConfig(t *testing.T) {
	for _, agents := range [][]model.AgentID{{model.AgentPi}, {model.AgentCursor}, {}} {
		t.Run(strings.Join(agentNames(agents), "-"), func(t *testing.T) {
			home := t.TempDir()
			for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "PI_CODING_AGENT_DIR", "TEMP", "TMP", "TMPDIR"} {
				t.Setenv(key, home)
			}
			path := ConfigPath(home)
			// A safe literal assignment, with intentionally non-generated formatting.
			original := "# Keep my formatting and custom settings\n  export PROVIDER='opencode' # chosen provider\nFILE_PATTERNS='*.rs,*.zig'\nEXCLUDE_PATTERNS='vendor/*'\nTIMEOUT=17\n"
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(AgentsTemplatePath(home), []byte("stale starter\n"), 0600); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				res, err := Inject(home, agents)
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(path)
				if err != nil || string(got) != original {
					t.Errorf("custom config rewritten: %q (%v)", got, err)
				}
				if res.ConfigChanged || res.ConfigFile != "" || res.ConfigCreationSkipped {
					t.Errorf("existing config reported as written/skipped creation: %+v", res)
				}
				for _, written := range res.FilesWritten() {
					if written == path {
						t.Error("unchanged config counted as written")
					}
				}
				if res.AgentsChanged != (i == 0) {
					t.Errorf("starter refresh on run %d: %+v", i, res)
				}
				starter, err := os.ReadFile(AgentsTemplatePath(home))
				if err != nil || len(starter) == 0 || string(starter) == "stale starter\n" {
					t.Fatalf("starter not refreshed: %q (%v)", starter, err)
				}
				for _, notice := range res.Notices {
					if strings.Contains(notice, "no provider is configured") {
						t.Errorf("valid provider described as missing: %s", notice)
					}
				}
			}
			// Independently seed the mapped control, even if preservation failed.
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			// Positive control: mapped selection still regenerates managed defaults.
			res, err := Inject(home, []model.AgentID{model.AgentCodex})
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			want := strings.Replace(string(BuildConfig("codex")), `PROVIDER="codex"`, "PROVIDER='opencode'", 1)
			if err != nil || string(got) != want || !res.ConfigChanged {
				t.Fatalf("mapped defaults no longer refreshed: %+v, %q (%v)", res, got, err)
			}
		})
	}
}

func agentNames(ids []model.AgentID) []string {
	var names []string
	for _, id := range ids {
		names = append(names, string(id))
	}
	return names
}

func TestInjectProviderRefusal(t *testing.T) {
	for _, original := range []string{"", "# user config\n", "PROVIDER=$(touch sentinel)\n", "PROVIDER=\"$FUTURE\"\n", "PROVIDER=claude\nPROVIDER=codex\n", "PROVIDER='broken\n", "if true; then PROVIDER=codex; fi\n", "PROVIDER=claude; touch sentinel\n", "PROVIDER=\n"} {
		t.Run(original, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("APPDATA", filepath.Join(home, "roaming"))
			path := ConfigPath(home)
			original = strings.ReplaceAll(original, "sentinel", "'"+filepath.ToSlash(filepath.Join(home, "sentinel"))+"'")
			if original != "" {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
			}
			for i := 0; i < 2; i++ {
				agents := []model.AgentID{model.AgentCursor}
				if i == 1 && strings.Contains(original, "PROVIDER") {
					agents = []model.AgentID{model.AgentClaudeCode}
				}
				res, err := Inject(home, agents)
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(path)
				if original == "" {
					if !os.IsNotExist(err) {
						t.Fatalf("config created: %s (%v)", got, err)
					}
				} else if err != nil || string(got) != original {
					t.Fatalf("config modified: %q (%v)", got, err)
				}
				if res.ConfigChanged || res.ConfigFile != "" {
					t.Fatal("skip reported config write")
				}
				if res.ConfigCreationSkipped != (original == "") {
					t.Fatalf("incorrect creation skip status: %+v", res)
				}
				if len(res.Notices) != 1 || !strings.Contains(res.Notices[0], "rerun gentle-ai sync") {
					t.Fatalf("missing actionable notice: %v", res.Notices)
				}
				if i == 1 && res.AgentsChanged {
					t.Fatal("repeat changed template")
				}
			}
			if _, err := os.Stat(filepath.Join(home, "sentinel")); !os.IsNotExist(err) {
				t.Fatal("dynamic shell executed")
			}
		})
	}
}

func TestInjectProviderLiteral(t *testing.T) {
	home := t.TempDir()
	t.Setenv("APPDATA", filepath.Join(home, "roaming"))
	path := ConfigPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	// Shell metacharacters inside single quotes are literal, never expansions.
	if err := os.WriteFile(path, []byte("export PROVIDER='future:$HOME`id`\\model' # literal\n"), 0600); err != nil {
		t.Fatal(err)
	}
	res, err := Inject(home, []model.AgentID{model.AgentClaudeCode})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "PROVIDER='future:$HOME`id`\\model'") {
		t.Fatalf("literal reinterpreted: %s", got)
	}
	if !res.ConfigChanged {
		t.Fatal("managed defaults were not refreshed")
	}
}

func TestInjectProviderInitialization(t *testing.T) {
	for _, tt := range []struct {
		name   string
		agents []model.AgentID
		want   string
	}{
		{"claude priority", []model.AgentID{model.AgentCodex, model.AgentClaudeCode}, "claude"},
		{"opencode priority", []model.AgentID{model.AgentGeminiCLI, model.AgentOpenCode}, "opencode"},
		{"gemini", []model.AgentID{model.AgentGeminiCLI}, "gemini"},
		{"antigravity", []model.AgentID{model.AgentAntigravity}, "gemini"},
		{"codex", []model.AgentID{model.AgentCodex}, "codex"},
		{"mixed", []model.AgentID{model.AgentCursor, model.AgentOpenCode}, "opencode"},
		{"mixed reversed", []model.AgentID{model.AgentOpenCode, model.AgentCursor}, "opencode"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("APPDATA", filepath.Join(home, "roaming"))
			path := ConfigPath(home)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("# no provider yet\nTIMEOUT=100\n"), 0600); err != nil {
				t.Fatal(err)
			}
			res, err := Inject(home, tt.agents)
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(BuildConfig(tt.want)) || !res.ConfigChanged || len(res.Notices) != 0 {
				t.Fatalf("mapped initialization: %+v, %s", res, got)
			}
		})
	}
}

func TestInjectProviderShellWhitespace(t *testing.T) {
	for _, ws := range []string{"\u00a0", "\u2003", "\u0085", "\v", "\f"} {
		for _, format := range []string{
			"PROVIDER=codex%s#${MODEL}\n", // Not a shell comment boundary.
			"%sPROVIDER=codex\n", "PROVIDER=%scodex\n",
			"PROVIDER=codex%s\n", "PROVIDER='codex'%s\n",
			"PROVIDER=\"codex\" %s# comment\n", "PROVIDER=%s'codex'\n",
		} {
			for _, agent := range []model.AgentID{model.AgentCursor, model.AgentClaudeCode} {
				t.Run(fmt.Sprintf("%x/%s/%s", []byte(ws), format, agent), func(t *testing.T) {
					home := t.TempDir()
					t.Setenv("APPDATA", filepath.Join(home, "roaming"))
					path := ConfigPath(home)
					original := fmt.Sprintf(format, ws)
					if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(original), 0600); err != nil {
						t.Fatal(err)
					}
					for i := 0; i < 2; i++ {
						res, err := Inject(home, []model.AgentID{agent})
						if err != nil {
							t.Fatal(err)
						}
						got, err := os.ReadFile(path)
						if err != nil || string(got) != original {
							t.Fatalf("unsafe whitespace rewritten: %q (%v)", got, err)
						}
						want := fmt.Sprintf("GGA config unchanged: %s contains ambiguous, dynamic, duplicate or unsupported assignments. Set one literal PROVIDER assignment in a simple assignment-only config, then rerun gentle-ai sync; no shell was evaluated.", path)
						if len(res.Notices) != 1 || res.Notices[0] != want || res.ConfigChanged || res.ConfigFile != "" || res.ConfigCreationSkipped {
							t.Fatalf("unsafe whitespace not skipped: %+v", res)
						}
						if i == 1 && res.AgentsChanged {
							t.Fatal("repeat changed template")
						}
					}
				})
			}
		}
	}
}

func TestInjectProviderASCIIWhitespaceAndQuotedLiteral(t *testing.T) {
	for _, word := range []string{"codex", "'future\u00a0model'", "\"future\u2003model\""} {
		t.Run(word, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("APPDATA", filepath.Join(home, "roaming"))
			path := ConfigPath(home)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(" \texport\tPROVIDER="+word+"\t # comment\n"), 0600); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				res, err := Inject(home, []model.AgentID{model.AgentClaudeCode})
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(got), "PROVIDER="+word+"\n") {
					t.Fatalf("literal changed: %q (%v)", got, err)
				}
				if len(res.Notices) != 0 || res.ConfigCreationSkipped || i == 1 && res.ConfigChanged {
					t.Fatalf("literal not updated safely: %+v", res)
				}
			}
		})
	}
}

func TestInjectProviderUnreadable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("APPDATA", filepath.Join(home, "roaming"))
	if err := os.MkdirAll(ConfigPath(home), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Inject(home, []model.AgentID{model.AgentClaudeCode}); err == nil {
		t.Fatal("unreadable config accepted")
	}
	if _, err := os.Stat(AgentsTemplatePath(home)); !os.IsNotExist(err) {
		t.Fatal("template written after read failure")
	}
}

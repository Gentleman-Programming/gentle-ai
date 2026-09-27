package reviewassets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

var codexNicknamePattern = regexp.MustCompile(`^[A-Za-z0-9 _-]+$`)

// TestRenderCodexAgentRoleTOMLRoundTripsEscaping is the renderer contract: the
// emitted TOML must parse back to the exact same four values, even when the
// developer instructions contain a backslash, a double quote, and a triple
// single-quote sequence.
func TestRenderCodexAgentRoleTOMLRoundTripsEscaping(t *testing.T) {
	name := "review-risk"
	description := `R1 Risk reviewer — "quotes", \backslashes\, and '''triples'''.`
	instructions := "first line with a \\ backslash\n" +
		"second line with a \" double quote\n" +
		"third line with ''' triple single quotes\n" +
		"fourth line with \"\"\" triple double quotes\n" +
		"fifth line with a literal \\n and a trailing backslash \\"
	nicknames := []string{"review risk", "review-risk"}

	encoded, err := renderCodexAgentRoleTOML(name, description, instructions, nicknames)
	if err != nil {
		t.Fatalf("renderCodexAgentRoleTOML() error = %v", err)
	}
	var parsed codexAgentRoleTOML
	if _, err := toml.Decode(encoded, &parsed); err != nil {
		t.Fatalf("emitted TOML does not parse: %v\n%s", err, encoded)
	}
	if parsed.Name != name {
		t.Errorf("name round-trip = %q, want %q", parsed.Name, name)
	}
	if parsed.Description != description {
		t.Errorf("description round-trip = %q, want %q", parsed.Description, description)
	}
	if parsed.DeveloperInstructions != instructions {
		t.Errorf("developer_instructions round-trip = %q, want %q", parsed.DeveloperInstructions, instructions)
	}
	if !reflect.DeepEqual(parsed.NicknameCandidates, nicknames) {
		t.Errorf("nickname_candidates round-trip = %v, want %v", parsed.NicknameCandidates, nicknames)
	}
}

// TestRenderCodexAgentRoleTOMLUsesOnlyCodexSchemaKeys guards the Codex
// deny_unknown_fields parser: exactly name, description, developer_instructions,
// and nickname_candidates may appear at the top level.
func TestRenderCodexAgentRoleTOMLUsesOnlyCodexSchemaKeys(t *testing.T) {
	encoded, err := renderCodexAgentRoleTOML("jd-judge-a", "description", "instructions", []string{"jd judge a", "jd-judge-a"})
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if _, err := toml.Decode(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"name": true, "description": true, "developer_instructions": true, "nickname_candidates": true,
	}
	if len(raw) != len(want) {
		t.Fatalf("top-level keys = %v, want exactly %v", raw, want)
	}
	for key := range raw {
		if !want[key] {
			t.Fatalf("unexpected top-level key %q (Codex rejects unknown keys)", key)
		}
	}
}

// TestCodexAgentRoleNamesMirrorTheClaudeManagedSet pins the managed role set to
// the shared manifest so the two projections can never drift.
func TestCodexAgentRoleNamesMirrorTheClaudeManagedSet(t *testing.T) {
	got := CodexAgentRoleNames()
	want := make([]string, 0, len(NativeAgentManifest[model.AgentClaudeCode]))
	for _, name := range NativeAgentManifest[model.AgentClaudeCode] {
		want = append(want, strings.TrimSuffix(name, ".md")+".toml")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CodexAgentRoleNames() = %v, want %v", got, want)
	}
	if len(got) != 8 {
		t.Fatalf("managed Codex role count = %d, want 8", len(got))
	}
}

// TestRenderManagedCodexAgentRolesParseWithCodexSchema renders every managed
// role and asserts the Codex parser contract plus the nickname rules.
func TestRenderManagedCodexAgentRolesParseWithCodexSchema(t *testing.T) {
	for _, fileName := range CodexAgentRoleNames() {
		role := strings.TrimSuffix(fileName, ".toml")
		t.Run(role, func(t *testing.T) {
			encoded, err := RenderCodexAgentRole(role)
			if err != nil {
				t.Fatalf("RenderCodexAgentRole() error = %v", err)
			}
			var parsed codexAgentRoleTOML
			if _, err := toml.Decode(encoded, &parsed); err != nil {
				t.Fatalf("role TOML does not parse: %v\n%s", err, encoded)
			}
			if parsed.Name != role {
				t.Errorf("name = %q, want %q", parsed.Name, role)
			}
			if strings.TrimSpace(parsed.Description) == "" {
				t.Error("description is blank")
			}
			if strings.TrimSpace(parsed.DeveloperInstructions) == "" {
				t.Fatal("developer_instructions is blank")
			}
			if !strings.HasPrefix(parsed.DeveloperInstructions, codexRuntimeProfileHeader) {
				t.Errorf("developer_instructions does not start with the Codex runtime-profile header")
			}
			for _, marker := range []string{
				"<!-- gentle-ai:agent-language-contract -->",
				"<!-- gentle-ai:remote-authorization -->",
			} {
				if !strings.Contains(parsed.DeveloperInstructions, marker) {
					t.Errorf("developer_instructions missing injected marker %q", marker)
				}
			}
			if len(parsed.NicknameCandidates) == 0 {
				t.Fatal("nickname_candidates must have at least one entry")
			}
			seen := map[string]bool{}
			for _, nickname := range parsed.NicknameCandidates {
				if strings.TrimSpace(nickname) == "" {
					t.Errorf("nickname_candidates contains a blank entry: %v", parsed.NicknameCandidates)
				}
				if seen[nickname] {
					t.Errorf("nickname_candidates contains a duplicate %q", nickname)
				}
				seen[nickname] = true
				if !codexNicknamePattern.MatchString(nickname) {
					t.Errorf("nickname %q violates [A-Za-z0-9 _-]+", nickname)
				}
			}
			wantNicknames := []string{strings.ReplaceAll(role, "-", " "), role}
			if !reflect.DeepEqual(parsed.NicknameCandidates, wantNicknames) {
				t.Errorf("nickname_candidates = %v, want %v", parsed.NicknameCandidates, wantNicknames)
			}
		})
	}
}

// TestRenderCodexAgentRoleCarriesNativeReviewerAndJudgeContract is the parity
// contract for the Codex projection. For a lens reviewer the native claude
// projection replaces the authored body with ClaudeReviewerPrompt, and for a
// jd-judge-* role it replaces the ledger section with the shared Judgment Day
// contract; the Codex projection must materialize the same body. The authored
// pre-render lens body must not survive, because no runtime installs it. The
// expectation is derived from the shared contract function and the native
// projection, never from a copy of the prompt text.
func TestRenderCodexAgentRoleCarriesNativeReviewerAndJudgeContract(t *testing.T) {
	adapter := claude.NewAdapter()
	for _, fileName := range CodexAgentRoleNames() {
		role := strings.TrimSuffix(fileName, ".toml")
		t.Run(role, func(t *testing.T) {
			instructions := codexDeveloperInstructions(t, role)
			authoredBody := codexAuthoredRoleBody(t, role)

			if prompt, reviewer := ClaudeReviewerPrompt(role); reviewer {
				native, err := renderNativeAgent(adapter, role+".md", InstallOptions{})
				if err != nil {
					t.Fatalf("renderNativeAgent(%s) error = %v", role, err)
				}
				_, nativeBody, err := splitAgentFrontmatter(native)
				if err != nil {
					t.Fatalf("split native %s: %v", role, err)
				}
				if expected := stripCodexInjectedSections(nativeBody); expected != prompt {
					t.Fatalf("native %s lens body drifted from the shared Claude lens transport", role)
				}
				if !strings.Contains(instructions, prompt) {
					t.Errorf("%s developer_instructions do not carry the native review-lens body", role)
				}
				if lead := authoredLeadSentence(authoredBody); lead != "" && strings.Contains(instructions, lead) {
					t.Errorf("%s developer_instructions still carry the authored pre-render lens body %q", role, lead)
				}
				return
			}
			if strings.HasPrefix(role, "jd-judge-") {
				native, err := renderNativeAgent(adapter, role+".md", InstallOptions{})
				if err != nil {
					t.Fatalf("renderNativeAgent(%s) error = %v", role, err)
				}
				_, nativeBody, err := splitAgentFrontmatter(native)
				if err != nil {
					t.Fatalf("split native %s: %v", role, err)
				}
				contract := JudgmentDayReviewerContract()
				if !strings.Contains(stripCodexInjectedSections(nativeBody), contract) {
					t.Fatalf("native %s does not carry the shared Judgment Day contract; cannot derive the expectation", role)
				}
				if !strings.Contains(instructions, contract) {
					t.Errorf("%s developer_instructions do not carry the shared Judgment Day contract", role)
				}
				return
			}
			// Non-reviewer, non-judge roles keep their authored body verbatim.
			if !strings.Contains(instructions, strings.TrimSpace(authoredBody)) {
				t.Errorf("%s developer_instructions dropped the authored body", role)
			}
		})
	}
}

// codexDeveloperInstructions renders one managed role and decodes its
// developer_instructions field.
func codexDeveloperInstructions(t *testing.T, role string) string {
	t.Helper()
	encoded, err := RenderCodexAgentRole(role)
	if err != nil {
		t.Fatalf("RenderCodexAgentRole(%s) error = %v", role, err)
	}
	var parsed codexAgentRoleTOML
	if _, err := toml.Decode(encoded, &parsed); err != nil {
		t.Fatalf("decode %s: %v", role, err)
	}
	return parsed.DeveloperInstructions
}

// codexAuthoredRoleBody returns the authored Codex asset body, the text that is
// dead for lens reviewers once the native projection replaces it.
func codexAuthoredRoleBody(t *testing.T, role string) string {
	t.Helper()
	source, err := assets.Read("codex/agents/" + role + ".md")
	if err != nil {
		t.Fatalf("read authored %s: %v", role, err)
	}
	_, body, err := splitAgentFrontmatter(source)
	if err != nil {
		t.Fatalf("split authored %s: %v", role, err)
	}
	return body
}

// authoredLeadSentence is the `You are **R1 Risk**`-style lead sentence that
// opens a lens asset body.
func authoredLeadSentence(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "You are") {
			return line
		}
	}
	return ""
}

// stripCodexInjectedSections removes the trailer sections every projection
// appends, so only the reviewer/judge body is compared.
func stripCodexInjectedSections(body string) string {
	for _, marker := range []string{
		"<!-- gentle-ai:agent-language-contract -->",
		"<!-- gentle-ai:remote-authorization -->",
	} {
		if idx := strings.Index(body, marker); idx >= 0 {
			body = body[:idx]
		}
	}
	return strings.TrimSpace(body)
}

// TestInstallCodexAgentRolesIsIdempotentAndPreservesUserFiles proves the shared
// ownership machinery: the second install is a no-op, and a pre-existing role
// file Gentle AI does not own is never overwritten or deleted.
func TestInstallCodexAgentRolesIsIdempotentAndPreservesUserFiles(t *testing.T) {
	home := t.TempDir()
	dir := CodexAgentRoleDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	userFile := filepath.Join(dir, "review-risk.toml")
	userBytes := []byte("name = \"review-risk\"\ndescription = \"my own role\"\ndeveloper_instructions = \"mine\"\n")
	if err := os.WriteFile(userFile, userBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(filepath.Dir(dir), "config.toml")
	configBytes := []byte("[agents]\nmax_depth = 2\nmax_threads = 4\n")
	if err := os.WriteFile(configFile, configBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := InstallCodexAgentRoles(home)
	if err != nil {
		t.Fatalf("first install: %v", err)
	}
	if !first.Changed {
		t.Fatal("first install reported no change")
	}
	if len(first.Skipped) != 1 || first.Skipped[0] != userFile {
		t.Fatalf("skipped = %v, want the pre-existing user file %s", first.Skipped, userFile)
	}
	if got, err := os.ReadFile(userFile); err != nil || string(got) != string(userBytes) {
		t.Fatalf("user file = %q, %v; want preserved", got, err)
	}

	afterFirst := readRoleDir(t, dir)
	second, err := InstallCodexAgentRoles(home)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if second.Changed || len(second.Files) != 0 {
		t.Fatalf("second install changed files: %v", second.Files)
	}
	if afterSecond := readRoleDir(t, dir); !reflect.DeepEqual(afterFirst, afterSecond) {
		t.Fatal("second install produced different bytes")
	}
	if got, err := os.ReadFile(configFile); err != nil || string(got) != string(configBytes) {
		t.Fatalf("config.toml = %q, %v; want untouched", got, err)
	}
	for _, fileName := range CodexAgentRoleNames() {
		if fileName == "review-risk.toml" {
			continue
		}
		if _, ok := afterFirst[fileName]; !ok {
			t.Errorf("managed role %s not installed", fileName)
		}
	}
}

func readRoleDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name()] = string(content)
	}
	return files
}

func writeCodexLedger(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(struct {
		Version int               `json:"version"`
		Files   map[string]string `json:"files"`
	}{Version: ownershipVersion, Files: files}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, OwnershipLedgerFilename), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readCodexLedger(t *testing.T, dir string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, OwnershipLedgerFilename))
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Version int               `json:"version"`
		Files   map[string]string `json:"files"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	return parsed.Files
}

// TestInstallCodexAgentRolesRemovesRetiredOwnedRole pins the retirement path: a
// role removed from the managed set and listed in the retired manifest is
// removed when Gentle AI owns it, and its ledger key is dropped, instead of the
// install hard-failing.
func TestInstallCodexAgentRolesRemovesRetiredOwnedRole(t *testing.T) {
	home := t.TempDir()
	if _, err := InstallCodexAgentRoles(home); err != nil {
		t.Fatalf("seed install: %v", err)
	}
	dir := CodexAgentRoleDir(home)
	legacyName := "review-legacy.toml"
	legacyBytes := []byte("name = \"review-legacy\"\ndescription = \"legacy role\"\ndeveloper_instructions = \"legacy\"\n")
	if err := os.WriteFile(filepath.Join(dir, legacyName), legacyBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	ledger := readCodexLedger(t, dir)
	ledger[legacyName] = installedHash(legacyBytes)
	writeCodexLedger(t, dir, ledger)

	res, err := installCodexAgentRoles(home, []string{legacyName})
	if err != nil {
		t.Fatalf("install with retired role: %v", err)
	}
	legacyPath := filepath.Join(dir, legacyName)
	if _, err := os.Lstat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("retired role still present: %v", err)
	}
	after := readCodexLedger(t, dir)
	if _, ok := after[legacyName]; ok {
		t.Fatal("retired ledger key was not dropped")
	}
	if len(after) != len(CodexAgentRoleNames()) {
		t.Fatalf("ledger entries = %d, want %d", len(after), len(CodexAgentRoleNames()))
	}
	if !codexContains(res.Files, legacyPath) {
		t.Errorf("removed path not reported in Files: %v", res.Files)
	}
}

// TestInstallCodexAgentRolesFailsClosedOnUnknownLedgerKey preserves the safety
// property: a ledger key in neither the live set nor the retired manifest is a
// hard failure, never an automatic deletion.
func TestInstallCodexAgentRolesFailsClosedOnUnknownLedgerKey(t *testing.T) {
	home := t.TempDir()
	dir := CodexAgentRoleDir(home)
	writeCodexLedger(t, dir, map[string]string{"ghost.toml": strings.Repeat("a", 64)})

	_, err := InstallCodexAgentRoles(home)
	if err == nil {
		t.Fatal("expected a hard failure for a ledger key in neither list")
	}
	if !strings.Contains(err.Error(), `invalid ownership ledger entry "ghost.toml"`) {
		t.Fatalf("error = %v, want invalid ownership ledger entry", err)
	}
}

func codexContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

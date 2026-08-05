package sdd

import (
	"encoding/json"
	"io/fs"
	"sort"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// The reviewer envelope was described independently in three places — the
// published schema, the shared orchestrator contract, and the generated lens
// prompt — and nothing checked that they agreed. A lens agent consequently
// returned findings/evidence with no subject_hash and no inspection, which
// admission correctly rejected (community report, PR #1801).
//
// These guards derive BOTH sides from source: the required shape comes from
// reviewtransaction.NewReviewerResultEnvelope (parsed out of the schema that
// sits beside AdmitArtifact) and the prompt set comes from walking the embedded
// assets tree. A lens added later, or a field added to admission later, fails
// here instead of drifting silently.

// lensAgentAssetPaths returns every embedded lens agent asset, keyed by the
// runtime directory that ships it. The lens identities come from the published
// schema, never from a list restated here.
func lensAgentAssetPaths(t *testing.T, lensAgentNames []string) map[string][]string {
	t.Helper()
	byRuntime := map[string][]string{}
	err := fs.WalkDir(assets.FS, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		directory, file := splitAssetPath(path)
		if !strings.HasSuffix(directory, "/agents") {
			return nil
		}
		// Only Markdown carries the prompt. A sibling manifest such as Kimi's
		// review-risk.yaml only points at the .md whose body ships.
		for _, lens := range lensAgentNames {
			if file == lens+".md" {
				byRuntime[directory] = append(byRuntime[directory], path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded assets: %v", err)
	}
	if len(byRuntime) == 0 {
		t.Fatalf("no lens agent assets found for lenses %v; the guard would pass vacuously", lensAgentNames)
	}
	for _, paths := range byRuntime {
		sort.Strings(paths)
	}
	return byRuntime
}

func splitAssetPath(path string) (string, string) {
	slash := strings.LastIndex(path, "/")
	if slash < 0 {
		return "", path
	}
	return path[:slash], path[slash+1:]
}

// singleJSONObjectExample returns the one JSON object embedded in a rendered
// prompt that carries the required top-level keys. Parsing the example the
// prompt actually shows — rather than trusting its prose — is what makes this
// guard structural.
func promptEnvelopeExample(t *testing.T, prompt string, required []string) map[string]json.RawMessage {
	t.Helper()
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") || !strings.HasSuffix(line, "}") {
			continue
		}
		var object map[string]json.RawMessage
		if json.Unmarshal([]byte(line), &object) != nil {
			continue
		}
		complete := true
		for _, field := range required {
			if _, ok := object[field]; !ok {
				complete = false
				break
			}
		}
		if complete {
			return object
		}
	}
	return nil
}

func TestLensAgentPromptsStateTheAdmissionEnvelope(t *testing.T) {
	envelope := reviewtransaction.NewReviewerResultEnvelope()
	if len(envelope.RequiredTopLevelFields) == 0 || len(envelope.LensAgentNames) == 0 {
		t.Fatalf("envelope derived nothing from the published schema: %+v", envelope)
	}

	// The binding name is derived from the orchestrator contract that issues
	// it, so renaming the binding forces every lens prompt to follow.
	contract := assets.MustRead(boundedReviewContractAsset)
	const bindingToken = "GENTLE_AI_REVIEW_BINDING"
	if !strings.Contains(contract, bindingToken) {
		t.Fatalf("%s no longer names %s; update the guard's derivation", boundedReviewContractAsset, bindingToken)
	}

	for runtime, paths := range lensAgentAssetPaths(t, envelope.LensAgentNames) {
		if len(paths) != len(envelope.LensAgentNames) {
			t.Errorf("%s ships %d lens agent(s) %v; the published schema declares %d: %v",
				runtime, len(paths), paths, len(envelope.LensAgentNames), envelope.LensAgentNames)
		}
		for _, path := range paths {
			prompt := renderBoundedReviewAsset(agentForAssetPath(t, path), path)

			for _, field := range envelope.RequiredTopLevelFields {
				if !strings.Contains(prompt, field) {
					t.Errorf("%s does not name the required top-level field %q that admission demands", path, field)
				}
			}

			example := promptEnvelopeExample(t, prompt, envelope.RequiredTopLevelFields)
			if example == nil {
				t.Errorf("%s shows no JSON example carrying every required field %v", path, envelope.RequiredTopLevelFields)
				continue
			}
			if len(example) != len(envelope.RequiredTopLevelFields) {
				t.Errorf("%s example has top-level keys beyond the required set %v", path, envelope.RequiredTopLevelFields)
			}
			var inspection struct {
				Status string   `json:"status"`
				Paths  []string `json:"paths"`
			}
			if err := json.Unmarshal(example["inspection"], &inspection); err != nil {
				t.Errorf("%s example inspection is not an object: %v", path, err)
			} else if inspection.Status != envelope.CompletedInspectionStatus {
				t.Errorf("%s example inspection.status = %q, want %q", path, inspection.Status, envelope.CompletedInspectionStatus)
			} else if len(inspection.Paths) == 0 {
				t.Errorf("%s example inspection carries no paths", path)
			}

			// Provenance is the part the failing report actually needed: a
			// reviewer that does not know where subject_hash comes from
			// invents nothing and simply omits it.
			if !strings.Contains(prompt, bindingToken) {
				t.Errorf("%s never names %s, so the reviewer is not told where subject_hash comes from", path, bindingToken)
			}
			if !strings.Contains(strings.ToLower(prompt), "never invent") {
				t.Errorf("%s does not forbid inventing subject_hash", path)
			}
		}
	}
}

// executionToolVocabulary names the capabilities that would let a reviewer run
// commands. It is a policy list, not a copy of any prompt: a lens that gains
// one of these must stop claiming it cannot execute, and a lens that keeps its
// read-only declaration must keep saying so.
var executionToolVocabulary = []string{
	"bash", "shell", "execute", "terminal", "command", "run",
	"write", "edit", "task", "apply", "patch",
}

// declaredLensTools reads the capability declaration out of an agent asset's
// frontmatter. Runtimes spell it differently — Claude lists `tools: Read, Grep,
// Glob`, Kiro lists `tools: ["read"]`, Cursor and Kimi assert `readonly: true`
// — so the guard reads whichever the file actually carries instead of assuming
// one shape.
func declaredLensTools(t *testing.T, path string) (tools []string, readOnlyAsserted bool) {
	t.Helper()
	content := assets.MustRead(path)
	parts := strings.SplitN(content, "---", 3)
	if len(parts) != 3 {
		t.Fatalf("%s has no frontmatter to derive tool declarations from", path)
	}
	for _, line := range strings.Split(parts[1], "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "readonly:"):
			readOnlyAsserted = strings.Contains(line, "true")
		case strings.HasPrefix(line, "tools:"):
			value := strings.TrimPrefix(line, "tools:")
			for _, token := range strings.Split(strings.Trim(strings.TrimSpace(value), "[]"), ",") {
				token = strings.Trim(strings.TrimSpace(token), `"'`)
				if token != "" {
					tools = append(tools, token)
				}
			}
		}
	}
	if len(tools) == 0 && !readOnlyAsserted {
		t.Fatalf("%s declares neither a tool list nor readonly: true; the guard cannot tell what it may do", path)
	}
	return tools, readOnlyAsserted
}

// TestLensAgentPromptsStateWhereTheirInputComesFrom is the input-side twin of
// the envelope guard. Claude Code receives immutable prompt context, while
// unsupported runtimes stop before they can inspect mutable workspace files.
func TestLensAgentPromptsStateWhereTheirInputComesFrom(t *testing.T) {
	envelope := reviewtransaction.NewReviewerResultEnvelope()

	// The orchestrator contract is the source for the current immutable
	// inspection route that rendered reviewer prompts must preserve.
	contract := assets.MustRead(boundedReviewContractAsset)
	if !strings.Contains(contract, "Claude Code carries immutable candidate evidence only in its provider-built prompt") ||
		!strings.Contains(contract, "read-only native Git commands") {
		t.Fatalf("%s no longer requires native-Git frozen-tree inspection; update the guard's derivation", boundedReviewContractAsset)
	}

	for _, paths := range lensAgentAssetPaths(t, envelope.LensAgentNames) {
		for _, path := range paths {
			if strings.HasPrefix(path, "claude/agents/") {
				continue // Claude's separate prompt transport is pinned in bounded_review_contract_test.go.
			}
			prompt := strings.ToLower(renderBoundedReviewAsset(agentForAssetPath(t, path), path))
			for claim, why := range map[string]string{
				"artifact_subject":      "does not name the bound artifact subject",
				"changed_path_manifest": "does not name the ordered manifest",
				"base_tree":             "does not name the immutable base tree",
				"candidate_tree":        "does not name the immutable candidate tree",
				"inspect-candidate":     "does not require provider-owned native inspection",
				"--operation numstat":   "does not require compact numstat discovery",
				"--path-index":          "does not select paths by canonical manifest index",
				"provider binding":      "does not resolve trees from the provider binding",
				"never read the live":   "does not prohibit mutable workspace inspection",
			} {
				if !strings.Contains(prompt, claim) {
					t.Errorf("%s %s (missing %q)", path, why, claim)
				}
			}
		}
	}
}

// TestJudgmentDayPromptsDoNotClaimTheLensEnvelope pins the resolution of the
// contradiction between the two documents: a judgment-day judge result is a
// different artifact (Transaction mode judgment_day records judge proofs and
// selects no lenses), so it must NOT carry the capture-result envelope, and it
// must say so rather than leaving a reader to infer which shape wins.
func TestJudgmentDayPromptsDoNotClaimTheLensEnvelope(t *testing.T) {
	envelope := reviewtransaction.NewReviewerResultEnvelope()
	judgePaths := []string{}
	err := fs.WalkDir(assets.FS, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		_, file := splitAssetPath(path)
		if strings.HasPrefix(file, "jd-judge-") {
			judgePaths = append(judgePaths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded assets: %v", err)
	}
	if len(judgePaths) == 0 {
		t.Fatal("no judgment-day judge assets found; the guard would pass vacuously")
	}
	sort.Strings(judgePaths)

	for _, path := range judgePaths {
		prompt := renderBoundedReviewAsset(agentForAssetPath(t, path), path)
		// Match the JSON key forms, not the English words: a judge prompt may
		// legitimately talk about inspecting a target.
		if strings.Contains(prompt, `"subject_hash"`) || strings.Contains(prompt, `"inspection"`) {
			t.Errorf("%s claims the capture-result lens envelope; judgment-day judge results carry neither field", path)
		}
		if !strings.Contains(prompt, "capture-result") {
			t.Errorf("%s does not say which artifact it produces, so it still contradicts the lens envelope", path)
		}
	}

	// The published judgment-day reference must scope its own restriction the
	// same way, since that unqualified sentence is the document a reader lands
	// on when the lens envelope and the judge envelope disagree.
	reference := assets.MustRead("skills/judgment-day/references/prompts-and-formats.md")
	if !strings.Contains(reference, "capture-result") {
		t.Errorf("skills/judgment-day/references/prompts-and-formats.md restricts top-level fields without naming the artifact it governs")
	}
	for _, field := range envelope.RequiredTopLevelFields {
		if field == "findings" || field == "evidence" {
			continue
		}
		if strings.Contains(reference, `"`+field+`"`) && !strings.Contains(reference, "not a `gentle-ai review capture-result`") {
			t.Errorf("skills/judgment-day/references/prompts-and-formats.md mentions %q without disclaiming the lens artifact", field)
		}
	}
}

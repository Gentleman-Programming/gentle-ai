package sdd

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// SDD delivery strategy has a producer and a consumer. The producer is the
// session-preflight label->canonical mapping; the consumer is every phase skill
// and orchestrator branch that reads `delivery_strategy`. In the Claude and
// OpenCode orchestrators both halves live in the same document about a hundred
// lines apart, and OpenCode carries a third copy of the producer compiled into
// ensurePreservedOpenCodeOrchestratorPreflight, which re-injects it into
// installed configs on every sync.
//
// Nothing asserted the halves agreed, so the preflight emitted `ask-always`,
// `single-pr-default`, `force-chained`, and `auto-forecast` into a consumer
// whose whole declared domain was `ask-on-risk | auto-chain | single-pr |
// exception-ok` (community report by @MarcosArispe). Every consumer branch list
// is a bare four-way match, so an out-of-domain value matched nothing and the
// orchestrator model picked a delivery strategy freely.
//
// Both halves below are derived from shipped artifacts, never from a list
// restated here: the domain from the phase skills that declare it as their
// input contract, the producer from the preflight UI label list plus the
// mapping block in the same document, and the producer sites from walking the
// embedded assets tree. A renamed label, a fifth canonical value, or a typo on
// either side fails here instead of drifting silently.

// deliveryDomainDeclaration matches a phase skill's declared input domain for
// delivery strategy: one backticked, pipe-separated list led by the default
// value. Keeping the pattern anchored on that shape is what lets the guard read
// the domain out of the shipped skill instead of hardcoding it.
var deliveryDomainDeclaration = regexp.MustCompile("`(ask-on-risk(?: \\| [a-z][a-z0-9-]*)+)`")

// preflightPRGroupLabels matches the user-facing PR option list the preflight
// renders, e.g. "3. PRs: Ask me, Single PR, Auto."
var preflightPRGroupLabels = regexp.MustCompile(`(?m)^\s*3\. PRs: (.+?)\.[ \t]*\r?$`)

// preflightStrategyChoiceDeclaration matches the preflight requirement line that
// names the canonical values the chained-PR question collects.
var preflightStrategyChoiceDeclaration = regexp.MustCompile(`(?m)^\s*3\. \*\*Chained PR strategy\*\*[^:]*: (.+?)[ \t]*\r?$`)

var backtickSpan = regexp.MustCompile("`([^`]+)`")

// canonicalValueShape matches the shape every delivery strategy value has:
// lowercase, hyphen-separated, nothing else. It exists to skip the prompt
// variables (`delivery_strategy`) and qualified labels (`size:exception`) that
// legitimately share those sentences, while still catching a retired or
// mistyped strategy such as `single-pr-default`.
var canonicalValueShape = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

// deliveryStrategySkillContracts are the phase skills that declare the domain as
// their own input contract. They are the consumers that reject an unknown value,
// which makes them the authority on what the producer is allowed to emit.
var deliveryStrategySkillContracts = []string{
	"skills/sdd-apply/SKILL.md",
	"skills/sdd-tasks/SKILL.md",
}

// canonicalDeliveryStrategyDomain reads the domain out of the shipped phase
// skills and fails when they disagree with each other.
func canonicalDeliveryStrategyDomain(t *testing.T) []string {
	t.Helper()

	var domain []string
	var source string
	for _, path := range deliveryStrategySkillContracts {
		matches := deliveryDomainDeclaration.FindAllStringSubmatch(assets.MustRead(path), -1)
		if len(matches) != 1 {
			t.Fatalf(
				"%s must declare the delivery strategy domain exactly once as a backticked `a | b | ...` list; found %d declarations",
				path,
				len(matches),
			)
		}

		values := splitDeclaredDomain(matches[0][1])
		if domain == nil {
			domain, source = values, path
			continue
		}
		if strings.Join(values, "|") != strings.Join(domain, "|") {
			t.Fatalf(
				"delivery strategy domain disagrees between shipped skills: %s declares %v, %s declares %v",
				source,
				domain,
				path,
				values,
			)
		}
	}

	if len(domain) == 0 {
		t.Fatal("no delivery strategy domain could be derived from the shipped phase skills")
	}
	return domain
}

func splitDeclaredDomain(declaration string) []string {
	values := make([]string, 0, 4)
	for _, value := range strings.Split(declaration, "|") {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

// preflightMappingSources returns every shipped asset that carries the
// label->canonical preflight mapping, discovered by walking the embedded tree so
// a new runtime that grows a preflight is covered without editing this guard.
func preflightMappingSources(t *testing.T) map[string]string {
	t.Helper()

	sources := map[string]string{}
	err := fs.WalkDir(assets.FS, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return walkErr
		}
		content := assets.MustRead(path)
		if strings.Contains(content, "Map answers to canonical values") {
			sources[path] = content
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded assets: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("no shipped asset carries a preflight label->canonical mapping; the guard has lost its subject")
	}

	// The OpenCode installer re-injects its own copy of the mapping into
	// existing configs, so a markdown-only fix would be reverted on the next
	// sync. Check the compiled literal through the same derivation.
	sources["internal/components/sdd/inject.go (ensurePreservedOpenCodeOrchestratorPreflight)"] =
		ensurePreservedOpenCodeOrchestratorPreflight("")

	return sources
}

// mappedCanonicalValue reads the canonical value the mapping block assigns to
// one user-facing label.
func mappedCanonicalValue(mapping, label string) (string, bool) {
	marker := label + " -> `"
	start := strings.Index(mapping, marker)
	if start < 0 {
		return "", false
	}
	rest := mapping[start+len(marker):]
	end := strings.Index(rest, "`")
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// canonicalMappingBlock isolates the mapping from the surrounding prose so a
// label that merely appears in the UI section is never mistaken for a mapping.
func canonicalMappingBlock(t *testing.T, path, content string) string {
	t.Helper()

	start := strings.Index(content, "Map answers to canonical values")
	if start < 0 {
		t.Fatalf("%s lost its canonical mapping block", path)
	}
	block := content[start:]
	if end := strings.Index(block, "Hard gate rules:"); end >= 0 {
		block = block[:end]
	}
	return block
}

func TestSDDPreflightDeliveryStrategyMappingStaysInsideConsumerDomain(t *testing.T) {
	domain := canonicalDeliveryStrategyDomain(t)
	allowed := map[string]bool{}
	for _, value := range domain {
		allowed[value] = true
	}

	for path, content := range preflightMappingSources(t) {
		labelMatch := preflightPRGroupLabels.FindStringSubmatch(content)
		if labelMatch == nil {
			t.Fatalf("%s carries a canonical mapping but no `3. PRs: ...` option list to map from", path)
		}

		mapping := canonicalMappingBlock(t, path, content)
		for _, rawLabel := range strings.Split(labelMatch[1], ",") {
			label := strings.TrimSpace(rawLabel)
			if label == "" {
				continue
			}

			value, ok := mappedCanonicalValue(mapping, label)
			if !ok {
				t.Errorf("%s offers PR option %q but maps it to no canonical value", path, label)
				continue
			}
			if !allowed[value] {
				t.Errorf(
					"%s maps PR option %q to %q, which is outside the delivery strategy domain %v declared by %v; "+
						"the consumer branch list has no arm for it, so the orchestrator would pick a strategy freely",
					path,
					label,
					value,
					domain,
					deliveryStrategySkillContracts,
				)
			}
		}
	}
}

func TestSDDPreflightStrategyChoicesStayInsideConsumerDomain(t *testing.T) {
	domain := canonicalDeliveryStrategyDomain(t)
	allowed := map[string]bool{}
	for _, value := range domain {
		allowed[value] = true
	}

	checked := 0
	for path, content := range preflightMappingSources(t) {
		match := preflightStrategyChoiceDeclaration.FindStringSubmatch(content)
		if match == nil {
			continue
		}
		checked++

		for _, span := range backtickSpan.FindAllStringSubmatch(match[1], -1) {
			for _, value := range splitDeclaredDomain(span[1]) {
				if !canonicalValueShape.MatchString(value) {
					continue
				}
				if !allowed[value] {
					t.Errorf(
						"%s declares chained PR strategy choice %q, which is outside the delivery strategy domain %v",
						path,
						value,
						domain,
					)
				}
			}
		}
	}

	if checked == 0 {
		t.Fatal("no preflight declares its chained PR strategy choices; the guard has lost its subject")
	}
}

// reviewWorkloadGuardSections returns the Review Workload Guard body of every
// shipped orchestrator, discovered by walking the embedded tree.
func reviewWorkloadGuardSections(t *testing.T) map[string]string {
	t.Helper()

	const heading = "### Review Workload Guard"
	sections := map[string]string{}
	err := fs.WalkDir(assets.FS, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return walkErr
		}
		content := assets.MustRead(path)
		start := strings.Index(content, heading)
		if start < 0 {
			return nil
		}
		body := content[start+len(heading):]
		for _, next := range []string{"\n### ", "\n## ", "\n# ", "\n<!-- gentle-ai:"} {
			if end := strings.Index(body, next); end >= 0 {
				body = body[:end]
			}
		}
		sections[path] = body
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded assets: %v", err)
	}
	if len(sections) == 0 {
		t.Fatal("no shipped orchestrator carries a Review Workload Guard; the guard has lost its subject")
	}
	return sections
}

// A branch list that silently drops an arm is the same defect seen from the
// other side: a value the producer can emit that the consumer cannot match. The
// domain is derived, so a typo in any arm shows up here as a missing arm.
func TestSDDReviewWorkloadGuardsCoverTheWholeDeliveryStrategyDomain(t *testing.T) {
	domain := canonicalDeliveryStrategyDomain(t)

	for path, section := range reviewWorkloadGuardSections(t) {
		var missing []string
		for _, value := range domain {
			if !strings.Contains(section, "`"+value+"`") {
				missing = append(missing, value)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			t.Errorf(
				"%s Review Workload Guard has no arm for delivery strategy %v; an unmatched value lets the orchestrator choose freely",
				path,
				missing,
			)
		}
	}
}

// The vocabularies agreeing today does not stop a future typo from reintroducing
// silent fallthrough. Every guard must say what happens to a value it does not
// recognise instead of leaving the branch list open.
func TestSDDReviewWorkloadGuardsRejectUnrecognisedDeliveryStrategy(t *testing.T) {
	for path, section := range reviewWorkloadGuardSections(t) {
		if !strings.Contains(section, "Any other `delivery_strategy` value is invalid") {
			t.Errorf(
				"%s Review Workload Guard does not say what to do with an unrecognised `delivery_strategy`; "+
					"a bare branch list falls through silently",
				path,
			)
		}
	}
}

// A preserved OpenCode prompt that already carries a well-formed preflight
// satisfies every other freshness clause in
// ensurePreservedOpenCodeOrchestratorPreflight, so before this the retired
// mapping survived every sync and a markdown-only fix would have been reverted
// on the user's next install.
func TestInjectOpenCodeMigratesRetiredDeliveryStrategyMapping(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}

	stalePrompt := strings.ReplaceAll(
		ensurePreservedOpenCodeOrchestratorPreflight(""),
		"Ask me -> `ask-on-risk`; Single PR -> `single-pr`; Auto -> `auto-chain`",
		"Ask me -> `ask-always`; Single PR -> `single-pr-default`; Auto -> `auto-forecast`",
	)
	if !strings.Contains(stalePrompt, "Ask me -> `ask-always`") {
		t.Fatal("test seed did not reproduce the retired mapping; the literal shape changed")
	}

	seed := `{
  "agent": {
    "gentle-orchestrator": {
      "mode": "primary",
      "prompt": ` + strconv.Quote("# Custom prompt\n"+stalePrompt) + `
    }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(seed), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	if _, err := Inject(home, opencodeAdapter(), model.SDDModeMulti, InjectOptions{
		PreserveOpenCodeOrchestratorPrompt: true,
	}); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	text := preservedOrchestratorPrompt(t, settingsPath)

	for _, retired := range []string{"ask-always", "single-pr-default", "force-chained", "auto-forecast"} {
		if strings.Contains(text, retired) {
			t.Errorf("opencode.json kept retired delivery strategy value %q after sync", retired)
		}
	}
	if !strings.Contains(text, "Ask me -> `ask-on-risk`") {
		t.Error("opencode.json did not receive the corrected delivery strategy mapping")
	}
	if !strings.Contains(text, "# Custom prompt") {
		t.Error("migration discarded the user's own prompt content")
	}
}

// `Chained` was retired from the preflight PR question. Chaining is not a mode
// an operator can force: `delivery_strategy` is read only after the tasks
// Review Workload Forecast crosses the review-budget line, so below that line
// there is nothing to chain, and above it `Auto` already resolves to
// `auto-chain` without asking again. The label therefore promised control the
// design does not offer, and it collided with the pace question's `Automatic`.
// Only the label is retired; `auto-chain` stays canonical, which the companion
// guard below pins.
func TestSDDPreflightNoLongerOffersRetiredChainedPRLabel(t *testing.T) {
	const retired = "Chained"

	for path, content := range preflightMappingSources(t) {
		labelMatch := preflightPRGroupLabels.FindStringSubmatch(content)
		if labelMatch == nil {
			t.Fatalf("%s carries a canonical mapping but no `3. PRs: ...` option list to check", path)
		}

		for _, rawLabel := range strings.Split(labelMatch[1], ",") {
			if strings.TrimSpace(rawLabel) != retired {
				continue
			}
			t.Errorf(
				"%s still offers the retired PR option %q; chaining is decided by the tasks review-budget forecast, "+
					"not by this preference, so the option promises control the preflight cannot deliver",
				path,
				retired,
			)
		}

		if _, mapped := mappedCanonicalValue(canonicalMappingBlock(t, path, content), retired); mapped {
			t.Errorf("%s still maps the retired PR option %q to a canonical value", path, retired)
		}
	}
}

// Removing a label must not remove a canonical value. `auto-chain` is still in
// the consumer domain and `Auto` is still the label that produces it; a guard
// that only checked for the absence of `Chained` would pass on a preflight that
// dropped auto-chaining altogether.
func TestSDDPreflightAutoStillProducesAutoChain(t *testing.T) {
	for path, content := range preflightMappingSources(t) {
		value, mapped := mappedCanonicalValue(canonicalMappingBlock(t, path, content), "Auto")
		if !mapped {
			t.Errorf("%s no longer maps the PR option \"Auto\" to any canonical value", path)
			continue
		}
		if value != "auto-chain" {
			t.Errorf(
				"%s maps PR option \"Auto\" to %q; retiring the `Chained` label must not retire the `auto-chain` canonical value",
				path,
				value,
			)
		}
	}
}

// The four-option preflight shipped alongside the already-corrected
// `ask-on-risk` mapping, so a prompt that still offers `Chained` satisfies every
// freshness clause that existed before this change — including the one added
// when the retired mapping was fixed. Without a clause that fails on the retired
// option, an installed config would keep the four-option menu forever and the
// asset-only removal would be reverted on the operator's next sync.
func TestInjectOpenCodeMigratesRetiredChainedPRPreflightOption(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}

	// Rebuild the exact prompt the previous release injected by reversing this
	// change on the current literal, so the seed tracks the literal instead of
	// freezing a copy of it that could drift.
	stalePrompt := strings.NewReplacer(
		"3. PRs: Ask me, Single PR, Auto.",
		"3. PRs: Ask me, Single PR, Chained, Auto.",
		"Ask me -> `ask-on-risk`; Single PR -> `single-pr`; Auto -> `auto-chain`",
		"Ask me -> `ask-on-risk`; Single PR -> `single-pr`; Chained -> `auto-chain`; Auto -> `auto-chain`",
		"The preflight offers no separate chained option because `delivery_strategy` is only consulted once the tasks forecast flags review-budget risk: below that line there is nothing to chain, and above it `Auto` already resolves to `auto-chain`.",
		"Chained and Auto both resolve to `auto-chain` because `delivery_strategy` is only consulted once the tasks forecast flags review-budget risk.",
	).Replace(ensurePreservedOpenCodeOrchestratorPreflight(""))

	for _, seeded := range []string{
		"3. PRs: Ask me, Single PR, Chained, Auto.",
		"Chained -> `auto-chain`",
		"Chained and Auto both resolve to `auto-chain`",
	} {
		if !strings.Contains(stalePrompt, seeded) {
			t.Fatalf("test seed did not reproduce the retired four-option preflight fragment %q; the literal shape changed", seeded)
		}
	}

	seed := `{
  "agent": {
    "gentle-orchestrator": {
      "mode": "primary",
      "prompt": ` + strconv.Quote("# Custom prompt\n"+stalePrompt) + `
    }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(seed), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	if _, err := Inject(home, opencodeAdapter(), model.SDDModeMulti, InjectOptions{
		PreserveOpenCodeOrchestratorPrompt: true,
	}); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	text := preservedOrchestratorPrompt(t, settingsPath)

	for _, residue := range []string{
		"3. PRs: Ask me, Single PR, Chained, Auto.",
		"Chained -> `auto-chain`",
		"Chained and Auto both resolve to `auto-chain`",
	} {
		if strings.Contains(text, residue) {
			t.Errorf("opencode.json kept retired preflight fragment %q after sync", residue)
		}
	}
	if !strings.Contains(text, "3. PRs: Ask me, Single PR, Auto.") {
		t.Error("opencode.json did not receive the three-option PR preflight list")
	}
	if !strings.Contains(text, "Auto -> `auto-chain`") {
		t.Error("opencode.json lost the `auto-chain` canonical value; only the `Chained` label was retired")
	}
	if !strings.Contains(text, "# Custom prompt") {
		t.Error("migration discarded the user's own prompt content")
	}
	if count := strings.Count(text, "3. PRs: "); count != 1 {
		t.Errorf("migrated prompt carries %d PR option lists; the retired menu must be replaced, not appended to", count)
	}

	// The freshness clause that fires this migration must also stop firing once
	// it has run, or every later sync would rewrite the operator's prompt again.
	if _, err := Inject(home, opencodeAdapter(), model.SDDModeMulti, InjectOptions{
		PreserveOpenCodeOrchestratorPrompt: true,
	}); err != nil {
		t.Fatalf("second Inject() error = %v", err)
	}
	if resynced := preservedOrchestratorPrompt(t, settingsPath); resynced != text {
		t.Error("re-syncing the migrated prompt changed it again; the migration is not idempotent")
	}
}

// preservedOrchestratorPrompt reads the preserved prompt back through JSON
// rather than matching raw bytes: encoding/json escapes the ">" in "->", so a
// substring match on the file would silently never fire.
func preservedOrchestratorPrompt(t *testing.T, settingsPath string) string {
	t.Helper()

	settingsBytes, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}
	var settings struct {
		Agent map[string]struct {
			Prompt string `json:"prompt"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(settingsBytes, &settings); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}
	prompt := settings.Agent["gentle-orchestrator"].Prompt
	if prompt == "" {
		t.Fatal("opencode.json lost the preserved gentle-orchestrator prompt")
	}
	return prompt
}

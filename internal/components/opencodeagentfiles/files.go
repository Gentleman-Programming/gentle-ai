// Package opencodeagentfiles emits gentle-ai's wholesale-managed OpenCode
// subagents as individual Markdown agent files — the OpenCode V2 file-based
// agent format discovered under the global agents directory — instead of
// settings-document entries, and migrates older installs away from the
// settings copies.
package opencodeagentfiles

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/mutationjournal"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencodeagents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/managedownership"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// OwnershipLedgerFilename identifies the managed-agent ownership record.
const OwnershipLedgerFilename = ".gentle-ai-managed-agent-ownership.json"

// Manifest lists the wholesale-managed subagents this package emits as
// individual agent files. gentle-orchestrator is deliberately absent: its
// prompt is marker-merged routing guidance delivered through the settings
// document, not a whole-file artifact.
var Manifest = []string{
	"gentle-ai-explore", "gentle-ai-verify", "gentle-ai-worker",
	"jd-fix-agent", "jd-judge-a", "jd-judge-b",
	"review-readability", "review-refuter", "review-reliability",
	"review-resilience", "review-risk", "review-validator",
}

// Result reports what an install touched. SettingsChanged marks the settings
// document rewrite (managed entries leaving the `agents` map); Files lists
// every agent file and ledger path the emitter wrote.
type Result struct {
	Changed         bool
	SettingsChanged bool
	Files           []string
	Skipped         []string
}

type ownershipLedger struct {
	Version int               `json:"version"`
	Files   map[string]string `json:"files"`
}

// Render renders one managed agent file exactly as the emitter writes it:
// frontmatter carrying the settings-entry fields (the OpenCode V2 agent-file
// format accepts the same fields as an `agents` configuration entry) and the
// embedded prompt as the document body, which becomes the agent's system
// prompt.
func Render(agent model.AgentID, name string) (string, error) {
	entry, err := opencodeagents.NativeEntryByName(agent, name)
	if err != nil {
		return "", err
	}
	description, _ := entry["description"].(string)
	prompt, _ := entry["system"].(string)
	rules, _ := entry["permissions"].([]map[string]any)
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "description: %s\n", yamlString(description))
	b.WriteString("mode: subagent\n")
	b.WriteString("hidden: true\n")
	b.WriteString("permissions:\n")
	for _, rule := range rules {
		action, _ := rule["action"].(string)
		resource, _ := rule["resource"].(string)
		effect, _ := rule["effect"].(string)
		fmt.Fprintf(&b, "  - action: %s\n    resource: %s\n    effect: %s\n", yamlString(action), yamlString(resource), yamlString(effect))
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimRight(prompt, "\n"))
	b.WriteString("\n")
	return b.String(), nil
}

// yamlString quotes a scalar as a YAML double-quoted string so pattern
// wildcards, flag permutations, and descriptions render safely.
func yamlString(value string) string {
	if value == "" {
		return `""`
	}
	return strconvQuoteYAML(value)
}

// strconvQuoteYAML renders a double-quoted YAML scalar. YAML's double-quoted
// escaping matches Go's for the ASCII payloads gentle-ai writes.
func strconvQuoteYAML(value string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Install emits the managed subagents as individual agent files under the
// OpenCode global agents directory and removes their entries from the
// settings document, migrating older installs to the file-based layout.
// User-authored files are never touched: a file is rewritten only when the
// ownership ledger records its exact bytes or the bytes equal the managed
// render. Anything else, including a user's own agent under a managed name,
// is preserved and reported as skipped.
func Install(homeDir string, adapter agents.Adapter, settingsPath string) (Result, error) {
	result := Result{}
	if adapter.Agent() != model.AgentOpenCode {
		return result, nil
	}
	dir := adapter.SubAgentsDir(homeDir)
	if dir == "" {
		return result, errors.New("empty OpenCode agents directory")
	}
	rendered := make(map[string]string, len(Manifest))
	for _, name := range Manifest {
		content, err := Render(adapter.Agent(), name)
		if err != nil {
			return result, err
		}
		rendered[name] = content
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return result, fmt.Errorf("create OpenCode agents directory: %w", err)
	}
	journal := mutationjournal.New(dir)
	ledgerFile := filepath.Join(dir, OwnershipLedgerFilename)
	// The ledger is captured first; no agent mutation may precede its snapshot.
	if err := journal.Capture(ledgerFile); err != nil {
		return result, fmt.Errorf("capture ownership ledger: %w", err)
	}
	ledger, ledgerExists, err := managedownership.Read[ownershipLedger](ledgerFile, Manifest)
	if err != nil {
		return result, err
	}
	ledgerChanged := false
	for _, name := range Manifest {
		path := filepath.Join(dir, name+".md")
		if err := journal.Validate(path); err != nil {
			return result, err
		}
		if err := journal.Capture(path); err != nil {
			return result, fmt.Errorf("capture agent file %s: %w", name, err)
		}
		data, exists, err := managedFile(path)
		if err != nil {
			return result, err
		}
		expected, known := ledger.Files[name]
		owned := !exists || (known && managedownership.Hash(data) == expected)
		if exists && !owned {
			result.Skipped = append(result.Skipped, path)
			continue
		}
		desired := []byte(rendered[name])
		if !exists || string(data) != string(desired) {
			if _, err := journal.WriteWithMode(path, desired, 0o644); err != nil {
				return result, fmt.Errorf("write agent file %s: %w", name, err)
			}
			result.Changed = true
			result.Files = append(result.Files, path)
		}
		// Hash the bytes at the installed destination rather than the template.
		actual, err := os.ReadFile(path)
		if err != nil {
			return result, fmt.Errorf("read installed agent %s: %w", name, err)
		}
		if string(actual) != string(desired) {
			return result, fmt.Errorf("agent file changed after write: %s", path)
		}
		hash := managedownership.Hash(actual)
		if ledger.Files[name] != hash {
			ledger.Files[name] = hash
			ledgerChanged = true
		}
	}
	if ledgerChanged && len(ledger.Files) == 0 && ledgerExists {
		// Every entry stopped being owned: an empty ledger owns nothing.
		if _, err := journal.Remove(ledgerFile); err != nil {
			return result, fmt.Errorf("remove ownership ledger: %w", err)
		}
		result.Changed = true
		result.Files = append(result.Files, ledgerFile)
	} else if ledgerChanged {
		encoded, err := json.MarshalIndent(ledger, "", "  ")
		if err != nil {
			return result, fmt.Errorf("encode ownership ledger: %w", err)
		}
		encoded = append(encoded, '\n')
		if _, err := journal.WriteWithMode(ledgerFile, encoded, 0o644); err != nil {
			return result, fmt.Errorf("write ownership ledger: %w", err)
		}
		result.Changed = true
		result.Files = append(result.Files, ledgerFile)
	}
	settingsChanged, err := RemoveManagedAgentsFromSettings(settingsPath)
	if err != nil {
		return result, err
	}
	result.SettingsChanged = settingsChanged
	result.Changed = result.Changed || settingsChanged
	return result, nil
}

// RemoveManagedAgentsFromSettings migrates the settings document to the
// file-based layout by removing the managed subagent entries from the native
// `agents` map. A document without any managed entry is left untouched, and
// an unparseable document is skipped (the settings writers fail closed on it
// elsewhere).
func RemoveManagedAgentsFromSettings(settingsPath string) (bool, error) {
	raw, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		return false, nil
	}
	agents, _ := root["agents"].(map[string]any)
	present := false
	for _, name := range Manifest {
		if _, ok := agents[name]; ok {
			present = true
			break
		}
	}
	if !present {
		return false, nil
	}
	updated, _, err := filemerge.RemoveJSONCMembers(raw, []string{"agents"}, Manifest, true)
	if err != nil {
		return false, err
	}
	result, err := filemerge.WriteFileAtomic(settingsPath, updated, filemerge.ExistingFileMode(settingsPath, 0o644))
	return result.Changed, err
}

// OwnedFiles inspects the OpenCode global agents directory and reports which
// managed agent files gentle-ai owns (per the ownership ledger or exact
// managed bytes), which files were preserved as user-authored, and where the
// ownership ledger lives. Uninstall uses this to remove only owned bytes.
func OwnedFiles(homeDir string, adapter agents.Adapter) (owned []string, skipped []string, ledgerFile string, err error) {
	if adapter.Agent() != model.AgentOpenCode {
		return nil, nil, "", nil
	}
	dir := adapter.SubAgentsDir(homeDir)
	ledgerFile = filepath.Join(dir, OwnershipLedgerFilename)
	if info, statErr := os.Lstat(dir); statErr != nil || !info.IsDir() {
		return nil, nil, ledgerFile, nil
	}
	ledger, _, readErr := managedownership.Read[ownershipLedger](ledgerFile, Manifest)
	if readErr != nil {
		return nil, nil, ledgerFile, readErr
	}
	for _, name := range Manifest {
		path := filepath.Join(dir, name+".md")
		data, exists, fileErr := managedFile(path)
		if fileErr != nil || !exists {
			continue
		}
		expected, known := ledger.Files[name]
		if known && managedownership.Hash(data) == expected {
			owned = append(owned, path)
			continue
		}
		rendered, renderErr := Render(adapter.Agent(), name)
		if renderErr == nil && string(data) == rendered {
			owned = append(owned, path)
			continue
		}
		skipped = append(skipped, path)
	}
	if len(owned) == len(Manifest) {
		owned = append(owned, ledgerFile)
	}
	return owned, skipped, ledgerFile, nil
}

func managedFile(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("stat agent file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("agent file is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("read agent file %s: %w", path, err)
	}
	return data, true, nil
}

package cli

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// retiredWorkflowText matches SDD and OpenSpec, both retired in v4.0.0.
var retiredWorkflowText = regexp.MustCompile(`(?i)sdd|openspec`)

// TestCLIHelpShowsNoRetiredWorkflowText covers help and preview text a user
// reads: none of it may offer SDD or OpenSpec as a current feature.
func TestCLIHelpShowsNoRetiredWorkflowText(t *testing.T) {
	outputs := map[string]string{
		"dry-run": RenderDryRun(InstallResult{Selection: model.Selection{SDDMode: model.SDDModeMulti}}),
	}
	for name, run := range map[string]func(*bytes.Buffer) error{
		"telemetry --help":          func(out *bytes.Buffer) error { return RunTelemetry([]string{"--help"}, out) },
		"review store-reset --help": func(out *bytes.Buffer) error { return RunReviewStoreReset([]string{"--help"}, out) },
	} {
		var out bytes.Buffer
		if err := run(&out); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		outputs[name] = out.String()
	}
	for name, output := range outputs {
		if match := retiredWorkflowText.FindString(output); match != "" {
			t.Errorf("%s shows retired workflow text %q:\n%s", name, match, output)
		}
	}
}

// retiredWorkflowLiteralAllowlist lists the string literals in internal/cli and
// internal/tui that may still name SDD, keyed by file and literal. Only
// retirement inventory qualifies: identifiers that recognize legacy keys or
// files, and messages that name a legacy file the user must act on. Every
// entry must still match, so the list can only shrink.
var retiredWorkflowLiteralAllowlist = map[string]map[string]string{
	"internal/cli/run.go": {
		"agent:retire-sdd-assets:":             "retirement step ID",
		"agent:retire-sdd:":                    "retirement step ID",
		"agent:retire-sdd-settings:":           "retirement step ID",
		"retire SDD agents for %q: %w":         "retirement failure naming legacy agents",
		"retire SDD agents in %q settings: %w": "retirement failure naming legacy settings",
		"retire SDD prompts for %q: %w":        "retirement failure naming legacy prompts",
		"gentle-ai/sdd":                        "legacy OpenCode ownership marker",
		"sdd-":                                 "legacy OpenCode agent prefix",
	},
	"internal/cli/sync.go": {
		"sync:agent:retire-sdd-assets:":   "retirement step ID",
		"sync:agent:retire-sdd:":          "retirement step ID",
		"sync:agent:retire-sdd-settings:": "retirement step ID",
	},
	"internal/cli/validate.go": {
		"unsupported sdd-mode %q (valid: single, multi)": "unreachable validation of an install flag field nothing assigns",
	},
	"internal/tui/model.go": {
		"sdd-orchestrator": "legacy OpenCode coordinator key",
		"sdd-":             "legacy model-assignment key prefix",
	},
	"internal/tui/screens/claude_model_picker.go": {
		"sdd-": "legacy Claude model-assignment key prefix",
	},
	"internal/tui/screens/codex_model_picker.go": {
		"sdd-strong": "current Codex preset-matrix key persisted in gentle-ai state; never rendered",
		"sdd-mid":    "current Codex preset-matrix key persisted in gentle-ai state; never rendered",
		"sdd-cheap":  "current Codex preset-matrix key persisted in gentle-ai state; never rendered",
	},
}

// TestSourceStringsHaveNoRetiredWorkflowText is a ratchet over every string
// literal in the CLI and TUI packages, so new user-facing text cannot name a
// retired workflow without an explicit, reviewed allowlist entry.
func TestSourceStringsHaveNoRetiredWorkflowText(t *testing.T) {
	seen := map[string]map[string]bool{}
	scanned := 0
	for _, root := range []string{".", filepath.Join("..", "tui")} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if root == "." && path != "." {
					return filepath.SkipDir // internal/cli/*.go only.
				}
				return nil
			}
			if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			scanned++
			key := filepath.ToSlash(filepath.Join("internal", "cli", path))
			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(literal.Value)
				if err != nil || !retiredWorkflowText.MatchString(value) {
					return true
				}
				if _, ok := retiredWorkflowLiteralAllowlist[key][value]; ok {
					if seen[key] == nil {
						seen[key] = map[string]bool{}
					}
					seen[key][value] = true
					return true
				}
				t.Errorf("%s: string literal names a retired workflow: %q", key, value)
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if scanned == 0 {
		t.Fatal("no source files scanned")
	}
	for file, literals := range retiredWorkflowLiteralAllowlist {
		for literal := range literals {
			if !seen[file][literal] {
				t.Errorf("%s: allowlisted literal %q no longer appears; remove the entry", file, literal)
			}
		}
	}
}

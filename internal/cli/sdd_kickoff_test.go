package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustMkdirAllCLI(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

// newGovernanceWorkspace builds a minimal workspace + active change
// directory under t.TempDir(), the fixture shape every kickoff/gate CLI
// test in this file and in sdd_gate_test.go shares.
func newGovernanceWorkspace(t *testing.T, change string) (root, changeRoot string) {
	t.Helper()
	root = t.TempDir()
	changeRoot = filepath.Join(root, "openspec", "changes", change)
	mustMkdirAllCLI(t, changeRoot)
	return root, changeRoot
}

// writeRepoLikeAxiomYAML mirrors this repository's own axiom.yaml shape
// closely enough for multirole.DetectRoles: roles core and e2e declared, no
// fullstack (H-4), matching internal/kickoff/infer_test.go's
// repoLikeWorkspaceConfig fixture.
func writeRepoLikeAxiomYAML(t *testing.T, root string) {
	t.Helper()
	writeCLIFixture(t, filepath.Join(root, "axiom.yaml"), "workspace:\n"+
		"  name: test-workspace\n"+
		"  topology: monorepo-embedded\n"+
		"roles:\n"+
		"  core:\n"+
		"    name: core\n"+
		"    repositories:\n"+
		"      - path: .\n"+
		"  e2e:\n"+
		"    name: e2e\n"+
		"    repositories:\n"+
		"      - path: .\n")
}

// TestRunSDDKickoffSealWritesSummaryAndKickoffFile is task 9.1's base case:
// a correct seal exits 0 with a summary, and kickoff.yaml actually exists
// afterwards.
func TestRunSDDKickoffSealWritesSummaryAndKickoffFile(t *testing.T) {
	root, changeRoot := newGovernanceWorkspace(t, "inc-99-example")

	var out bytes.Buffer
	err := RunSDDKickoff([]string{
		"seal", "--cwd", root, "--change", "inc-99-example",
		"--execution-style", "checkpointed", "--handoff-policy", "per_checkpoint",
	}, &out)
	if err != nil {
		t.Fatalf("RunSDDKickoff() error = %v", err)
	}
	if !strings.Contains(out.String(), "inc-99-example") || !strings.Contains(out.String(), "checkpointed") {
		t.Fatalf("la salida no resume el sello:\n%s", out.String())
	}
	if _, statErr := os.Stat(filepath.Join(changeRoot, "kickoff.yaml")); statErr != nil {
		t.Fatalf("kickoff.yaml no se escribio: %v", statErr)
	}
}

// TestRunSDDKickoffSealOnAlreadySealedChangeReturnsWinnerWithoutRewriting
// covers D-01/D-02's single-write rule end to end through the CLI: a second
// seal with different flags exits 0, shows the WINNING (first) config, and
// never rewrites kickoff.yaml.
func TestRunSDDKickoffSealOnAlreadySealedChangeReturnsWinnerWithoutRewriting(t *testing.T) {
	root, changeRoot := newGovernanceWorkspace(t, "inc-99-example")
	sealedPath := filepath.Join(changeRoot, "kickoff.yaml")

	var first bytes.Buffer
	if err := RunSDDKickoff([]string{"seal", "--cwd", root, "--change", "inc-99-example", "--execution-style", "checkpointed", "--handoff-policy", "none"}, &first); err != nil {
		t.Fatalf("primer seal error = %v", err)
	}
	before, err := os.ReadFile(sealedPath)
	if err != nil {
		t.Fatalf("leer kickoff.yaml: %v", err)
	}

	var second bytes.Buffer
	if err := RunSDDKickoff([]string{"seal", "--cwd", root, "--change", "inc-99-example", "--execution-style", "continuous", "--handoff-policy", "per_checkpoint"}, &second); err != nil {
		t.Fatalf("segundo seal error = %v", err)
	}
	after, err := os.ReadFile(sealedPath)
	if err != nil {
		t.Fatalf("releer kickoff.yaml: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("el segundo seal reescribio kickoff.yaml:\nantes:\n%s\ndespues:\n%s", before, after)
	}
	if !strings.Contains(second.String(), "checkpointed") {
		t.Fatalf("la salida del segundo seal no muestra la configuracion ganadora (checkpointed):\n%s", second.String())
	}
}

// TestRunSDDKickoffSealInferThreeCases exercises design.md S8.1's three
// retro-seal cases (Phase 3) end to end through `seal --infer`.
func TestRunSDDKickoffSealInferThreeCases(t *testing.T) {
	t.Run("caso a: design.md con roles explicitos", func(t *testing.T) {
		root, changeRoot := newGovernanceWorkspace(t, "inc-99-example")
		writeRepoLikeAxiomYAML(t, root)
		writeCLIFixture(t, filepath.Join(changeRoot, "design.md"),
			"# Diseno\n\n```yaml\nroles:\n  - role: core\n    gate_policy: blocking\n  - role: e2e\n    gate_policy: deferred\n```\n")

		var out bytes.Buffer
		if err := RunSDDKickoff([]string{"seal", "--infer", "--cwd", root, "--change", "inc-99-example"}, &out); err != nil {
			t.Fatalf("RunSDDKickoff() error = %v", err)
		}
		if !strings.Contains(out.String(), "core") || !strings.Contains(out.String(), "e2e") || strings.Contains(out.String(), "fullstack") {
			t.Fatalf("la salida no refleja los roles inferidos del design.md (core, e2e, nunca fullstack):\n%s", out.String())
		}
	})

	t.Run("caso b: sin design.md sella fullstack", func(t *testing.T) {
		root, _ := newGovernanceWorkspace(t, "inc-99-example")

		var out bytes.Buffer
		if err := RunSDDKickoff([]string{"seal", "--infer", "--cwd", root, "--change", "inc-99-example"}, &out); err != nil {
			t.Fatalf("RunSDDKickoff() error = %v", err)
		}
		if !strings.Contains(out.String(), "fullstack") {
			t.Fatalf("la salida no refleja el rol fullstack por defecto:\n%s", out.String())
		}
	})

	t.Run("caso c: rol ausente de axiom.yaml propaga el error", func(t *testing.T) {
		root, changeRoot := newGovernanceWorkspace(t, "inc-99-example")
		writeRepoLikeAxiomYAML(t, root)
		writeCLIFixture(t, filepath.Join(changeRoot, "design.md"),
			"# Diseno\n\n```yaml\nroles:\n  - role: database\n    gate_policy: blocking\n```\n")

		err := RunSDDKickoff([]string{"seal", "--infer", "--cwd", root, "--change", "inc-99-example"}, &bytes.Buffer{})
		if err == nil {
			t.Fatal("RunSDDKickoff() = nil error, se esperaba propagar el error de DetectRoles (rol ausente de axiom.yaml)")
		}
		if _, statErr := os.Stat(filepath.Join(changeRoot, "kickoff.yaml")); !os.IsNotExist(statErr) {
			t.Fatalf("kickoff.yaml se escribio pese al error de deteccion de roles: statErr = %v", statErr)
		}
	})
}

// TestRunSDDKickoffSealOnArchivedRootRefusesNamingBugOrNewIncrement is
// D-14/REQ-21.18: a seal targeting a change root already under
// openspec/changes/archive/ is refused, naming the bug/new-increment path.
func TestRunSDDKickoffSealOnArchivedRootRefusesNamingBugOrNewIncrement(t *testing.T) {
	root := t.TempDir()
	archivedRoot := filepath.Join(root, "openspec", "changes", "archive", "inc-01-old")
	mustMkdirAllCLI(t, archivedRoot)

	err := RunSDDKickoff([]string{"seal", "--cwd", root, "--change", "inc-01-old", "--execution-style", "continuous", "--handoff-policy", "none"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("RunSDDKickoff() = nil error, se esperaba rechazo por raiz archivada (D-14)")
	}
	if !strings.Contains(err.Error(), "REQ-21.18") {
		t.Fatalf("error = %v, se esperaba que nombrase REQ-21.18 (bug o nuevo incremento)", err)
	}
	if _, statErr := os.Stat(filepath.Join(archivedRoot, "kickoff.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("kickoff.yaml se escribio pese al rechazo por raiz archivada: statErr = %v", statErr)
	}
}

// TestRunSDDKickoffUnknownSubcommand is T-8: an unrecognised kickoff
// subverb is refused.
func TestRunSDDKickoffUnknownSubcommand(t *testing.T) {
	err := RunSDDKickoff([]string{"bogus"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("RunSDDKickoff() = nil error, se esperaba rechazo de subcomando desconocido")
	}
}

// TestRunSDDKickoffNoSubcommand covers the empty-args edge of T-8.
func TestRunSDDKickoffNoSubcommand(t *testing.T) {
	err := RunSDDKickoff(nil, &bytes.Buffer{})
	if err == nil {
		t.Fatal("RunSDDKickoff() = nil error, se esperaba exigir un subcomando")
	}
}

// TestRunSDDKickoffHelp is T-8: --help and -h both exit 0 and mention both
// subcommands.
func TestRunSDDKickoffHelp(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		t.Run(flag, func(t *testing.T) {
			var out bytes.Buffer
			if err := RunSDDKickoff([]string{flag}, &out); err != nil {
				t.Fatalf("RunSDDKickoff(%q) error = %v", flag, err)
			}
			if !strings.Contains(out.String(), "seal") || !strings.Contains(out.String(), "show") {
				t.Fatalf("la ayuda no menciona los subcomandos seal/show:\n%s", out.String())
			}
		})
	}
}

// TestRunSDDKickoffShowOnUnsealedChangeStatesExplicitly is task 9.1's
// "show sobre un cambio sin sellar" scenario: an explicit, unambiguous "no
// kickoff" statement, never a silent empty success.
func TestRunSDDKickoffShowOnUnsealedChangeStatesExplicitly(t *testing.T) {
	root, _ := newGovernanceWorkspace(t, "inc-99-example")

	var out bytes.Buffer
	if err := RunSDDKickoff([]string{"show", "--cwd", root, "--change", "inc-99-example"}, &out); err != nil {
		t.Fatalf("RunSDDKickoff() error = %v", err)
	}
	if !strings.Contains(strings.ToLower(out.String()), "sin kickoff sellado") {
		t.Fatalf("la salida no declara explicitamente la ausencia de kickoff:\n%s", out.String())
	}
}

// TestRunSDDKickoffShowOnSealedChangeDisplaysIt confirms the mirror case:
// a sealed change's show output actually surfaces its configuration.
func TestRunSDDKickoffShowOnSealedChangeDisplaysIt(t *testing.T) {
	root, _ := newGovernanceWorkspace(t, "inc-99-example")
	if err := RunSDDKickoff([]string{"seal", "--cwd", root, "--change", "inc-99-example", "--execution-style", "checkpointed", "--handoff-policy", "none"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("seal error = %v", err)
	}

	var out bytes.Buffer
	if err := RunSDDKickoff([]string{"show", "--cwd", root, "--change", "inc-99-example"}, &out); err != nil {
		t.Fatalf("RunSDDKickoff() error = %v", err)
	}
	if !strings.Contains(out.String(), "checkpointed") {
		t.Fatalf("la salida de show no refleja la configuracion sellada:\n%s", out.String())
	}
}

// TestRunSDDKickoffSealOnNonexistentChangeNamesResolvedPath covers
// design.md S5.7's "cambio inexistente" row: these verbs write inside a
// change that already exists and never create one (task 8.2).
func TestRunSDDKickoffSealOnNonexistentChangeNamesResolvedPath(t *testing.T) {
	root := t.TempDir()
	mustMkdirAllCLI(t, filepath.Join(root, "openspec", "changes"))

	err := RunSDDKickoff([]string{"seal", "--cwd", root, "--change", "does-not-exist", "--execution-style", "continuous", "--handoff-policy", "none"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("RunSDDKickoff() = nil error, se esperaba rechazo: el cambio no existe")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Fatalf("error = %v, se esperaba que nombrase la ruta resuelta", err)
	}
}

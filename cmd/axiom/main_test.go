package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/cli"
)

func TestAppVersionInitialization(t *testing.T) {
	cli.AppVersion = Version
	if cli.AppVersion != Version {
		t.Fatalf("se esperaba que cli.AppVersion fuera %q, pero se obtuvo %q", Version, cli.AppVersion)
	}
}

func TestRunSDDHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"flag --help", []string{"--help"}},
		{"flag -h", []string{"-h"}},
		{"sin argumentos", []string{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runSDD(tc.args, &stdout, &stderr)

			out := stdout.String()
			if len(tc.args) == 0 {
				if exitCode != 1 {
					t.Fatalf("se esperaba código 1 sin argumentos, se obtuvo %d", exitCode)
				}
			} else {
				if exitCode != 0 {
					t.Fatalf("se esperaba código 0 con %v, se obtuvo %d", tc.args, exitCode)
				}
			}

			if !strings.Contains(out, "Uso: axiom sdd <subcomando>") {
				t.Fatalf("la salida no contiene el uso esperado:\n%s", out)
			}
			expectedSubcmds := []string{"status", "continue", "attempt", "verify-validate", "archive-compose", "task-result", "preflight-hook"}
			for _, sub := range expectedSubcmds {
				if !strings.Contains(out, sub) {
					t.Fatalf("la ayuda de sdd no documenta el subcomando %q:\n%s", sub, out)
				}
			}
		})
	}
}

func TestRunSDDUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := runSDD([]string{"subcomando-inexistente"}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("se esperaba código 1 para subcomando desconocido, se obtuvo %d", exitCode)
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, "no reconocido para sdd") {
		t.Fatalf("stderr no contiene el mensaje de error esperado:\n%s", errOut)
	}
}

func TestRunSDDStatusJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := runSDD([]string{"status", "inc-13-sdd-commands-axiom-cli-integration", "--json"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("runSDD status --json falló con código %d:\nstderr: %s\nstdout: %s", exitCode, stderr.String(), stdout.String())
	}

	var data map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		t.Fatalf("la salida de status --json no es JSON válido: %v\nSalida recibida:\n%s", err, stdout.String())
	}

	change, ok := data["changeName"].(string)
	if !ok || change != "inc-13-sdd-commands-axiom-cli-integration" {
		t.Fatalf("se esperaba changeName 'inc-13-sdd-commands-axiom-cli-integration' en JSON, se obtuvo %v", data["changeName"])
	}
}

func TestRunSDDStatusNonexistentChange(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := runSDD([]string{"status", "cambio-que-no-existe-123456789"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("se esperaba código 0 informando cambio no encontrado, se obtuvo %d", exitCode)
	}
	out := stdout.String()
	if !strings.Contains(out, "Active OpenSpec change not found") {
		t.Fatalf("se esperaba mensaje 'Active OpenSpec change not found', se obtuvo:\n%s", out)
	}
}

func TestRunSDDStatusInvalidFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := runSDD([]string{"status", "--flag-invalida-xyz"}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("se esperaba código 1 para bandera inválida, se obtuvo %d", exitCode)
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, "unknown sdd-status argument") {
		t.Fatalf("stderr no contiene error de argumento desconocido:\n%s", errOut)
	}
}

func TestRunReviewHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"flag --help", []string{"--help"}},
		{"flag -h", []string{"-h"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runReview(tc.args, &stdout, &stderr)
			if exitCode != 0 {
				t.Fatalf("se esperaba código 0 con %v, se obtuvo %d", tc.args, exitCode)
			}
			out := stdout.String()
			if !strings.Contains(out, "Uso: axiom review <subcomando>") {
				t.Fatalf("la salida no contiene el uso esperado:\n%s", out)
			}
			expectedSubcmds := []string{"mode", "start", "resume", "step", "bundle-export", "bundle-import", "validate"}
			for _, sub := range expectedSubcmds {
				if !strings.Contains(out, sub) {
					t.Fatalf("la ayuda de review no documenta el subcomando %q:\n%s", sub, out)
				}
			}
		})
	}
}

func TestRunReviewModeStatus(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := runReview([]string{"mode", "status"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("se esperaba código 0 al consultar review mode status, se obtuvo %d:\nstderr: %s", exitCode, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "receipt-driven development:") && !strings.Contains(out, "disabled") && !strings.Contains(out, "off") {
		t.Fatalf("salida inesperada al consultar review mode status:\n%s", out)
	}
}

func TestCLIIntegrationSubprocessAndFlatAliases(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("error al resolver raíz del repositorio: %v", err)
	}
	binName := "axiom.exe"
	binPath := filepath.Join(repoRoot, binName)
	if _, err := os.Stat(binPath); err != nil {
		t.Skipf("binario %s no encontrado en %s, saltando prueba de integración de subproceso", binName, binPath)
	}

	t.Run("axiom sdd status vs axiom sdd-status equivalencia", func(t *testing.T) {
		cmd1 := exec.Command(binPath, "sdd", "status", "inc-13-sdd-commands-axiom-cli-integration", "--json")
		cmd1.Dir = repoRoot
		out1, err1 := cmd1.Output()
		if err1 != nil {
			t.Fatalf("axiom sdd status falló: %v", err1)
		}

		cmd2 := exec.Command(binPath, "sdd-status", "inc-13-sdd-commands-axiom-cli-integration", "--json")
		cmd2.Dir = repoRoot
		out2, err2 := cmd2.Output()
		if err2 != nil {
			t.Fatalf("axiom sdd-status falló: %v", err2)
		}

		var json1, json2 map[string]interface{}
		if err := json.Unmarshal(out1, &json1); err != nil {
			t.Fatalf("salida de sdd status no es JSON válido: %v", err)
		}
		if err := json.Unmarshal(out2, &json2); err != nil {
			t.Fatalf("salida de sdd-status no es JSON válido: %v", err)
		}

		if json1["change"] != json2["change"] {
			t.Fatalf("las salidas difieren: sdd status (%v) != sdd-status (%v)", json1["change"], json2["change"])
		}
	})

	t.Run("axiom sdd continue vs axiom sdd-continue", func(t *testing.T) {
		cmd1 := exec.Command(binPath, "sdd", "continue", "inc-13-sdd-commands-axiom-cli-integration")
		cmd1.Dir = repoRoot
		out1, err1 := cmd1.CombinedOutput()
		if err1 != nil {
			t.Fatalf("axiom sdd continue falló: %v\nSalida: %s", err1, string(out1))
		}

		cmd2 := exec.Command(binPath, "sdd-continue", "inc-13-sdd-commands-axiom-cli-integration")
		cmd2.Dir = repoRoot
		out2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			t.Fatalf("axiom sdd-continue falló: %v\nSalida: %s", err2, string(out2))
		}

		if len(out1) == 0 || len(out2) == 0 {
			t.Fatalf("las salidas de continue no deben estar vacías")
		}
	})

	t.Run("axiom review mode status", func(t *testing.T) {
		cmd := exec.Command(binPath, "review", "mode", "status")
		cmd.Dir = repoRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("axiom review mode status falló: %v\nSalida: %s", err, string(out))
		}
		if !strings.Contains(string(out), "receipt-driven development:") && !strings.Contains(string(out), "off") {
			t.Fatalf("salida no contiene 'receipt-driven development:': %s", string(out))
		}
	})

	t.Run("axiom --help incluye comandos sdd y review", func(t *testing.T) {
		cmd := exec.Command(binPath, "--help")
		cmd.Dir = repoRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("axiom --help falló: %v\nSalida: %s", err, string(out))
		}
		outStr := string(out)
		if !strings.Contains(outStr, "sdd status") || !strings.Contains(outStr, "sdd continue") || !strings.Contains(outStr, "review") {
			t.Fatalf("axiom --help no documenta sdd o review:\n%s", outStr)
		}
	})
}

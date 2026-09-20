package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/cli"
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
			expectedSubcmds := []string{"status", "continue", "attempt", "archive-compose", "task-result", "preflight-hook"}
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
			exitCode := runReview(tc.args, &stdout, &stderr, false)
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
	exitCode := runReview([]string{"mode", "status"}, &stdout, &stderr, false)
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
		expectedCmds := []string{"tui", "install", "sync", "upgrade", "doctor", "backup", "restore", "uninstall"}
		for _, cmdName := range expectedCmds {
			if !strings.Contains(outStr, cmdName) {
				t.Fatalf("axiom --help no documenta el comando de ecosistema %q:\n%s", cmdName, outStr)
			}
		}
	})

	t.Run("axiom backup lista respaldos", func(t *testing.T) {
		cmd := exec.Command(binPath, "backup")
		cmd.Dir = repoRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("axiom backup falló: %v\nSalida: %s", err, string(out))
		}
		outStr := string(out)
		if !strings.Contains(outStr, "Respaldos registrados") && !strings.Contains(outStr, "No hay respaldos") {
			t.Fatalf("salida inesperada para axiom backup:\n%s", outStr)
		}
	})

	t.Run("axiom install --help", func(t *testing.T) {
		cmd := exec.Command(binPath, "install", "--help")
		cmd.Dir = repoRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("axiom install --help falló: %v\nSalida: %s", err, string(out))
		}
		if !strings.Contains(string(out), "install [flags]") {
			t.Fatalf("salida no contiene 'install [flags]':\n%s", string(out))
		}
	})

	t.Run("axiom sync --help", func(t *testing.T) {
		cmd := exec.Command(binPath, "sync", "--help")
		cmd.Dir = repoRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("axiom sync --help falló: %v\nSalida: %s", err, string(out))
		}
		if !strings.Contains(string(out), "sync [flags]") {
			t.Fatalf("salida no contiene 'sync [flags]':\n%s", string(out))
		}
	})
}

func TestRunBackup_Output(t *testing.T) {
	var buf bytes.Buffer
	runBackup(nil, &buf)
	out := buf.String()
	if !strings.Contains(out, "Respaldos registrados") && !strings.Contains(out, "No hay respaldos") {
		t.Fatalf("runBackup salida inesperada: %s", out)
	}
}

func TestRunChangeHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"flag --help", []string{"--help"}},
		{"flag -h", []string{"-h"}},
		{"create --help", []string{"create", "--help"}},
		{"sin argumentos", []string{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runChange(tc.args, &stdout, &stderr)
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

			if !strings.Contains(out, "axiom change") || !strings.Contains(out, "create") {
				t.Fatalf("la salida no contiene la ayuda esperada:\n%s", out)
			}
		})
	}
}

func TestRunChangeCreate_Success(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "axiom-cli-change-*")
	if err != nil {
		t.Fatalf("error creando tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	var stdout, stderr bytes.Buffer
	args := []string{
		"create", "inc-cli-feature",
		"--intent", "Integración completa de subcomando CLI con arnés SDD",
		"--type", "feature",
		"--cwd", tempDir,
	}
	exitCode := runChange(args, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("runChange falló con código %d:\nstderr: %s\nstdout: %s", exitCode, stderr.String(), stdout.String())
	}

	outStr := stdout.String()
	if !strings.Contains(outStr, "creado correctamente") {
		t.Errorf("salida no contiene mensaje de éxito: %s", outStr)
	}

	// Comprobar proposal.md generado en disco
	proposalPath := filepath.Join(tempDir, "openspec", "changes", "inc-cli-feature", "proposal.md")
	data, err := os.ReadFile(proposalPath)
	if err != nil {
		t.Fatalf("no se creó proposal.md en %s: %v", proposalPath, err)
	}
	content := string(data)
	if !strings.Contains(content, "# Propuesta: Inc Cli Feature") ||
		!strings.Contains(content, "Integración completa de subcomando CLI") {
		t.Errorf("proposal.md no contiene el formato esperado:\n%s", content)
	}
}

func TestRunChangeCreate_ValidationAndCollision(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "axiom-cli-validation-*")
	if err != nil {
		t.Fatalf("error creando tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 1. Nombre inválido
	var stdout1, stderr1 bytes.Buffer
	code1 := runChange([]string{"create", "Nombre Invalido!", "--cwd", tempDir}, &stdout1, &stderr1)
	if code1 != 1 {
		t.Errorf("esperado código 1 para nombre inválido, obtenido %d", code1)
	}
	if !strings.Contains(stderr1.String(), "kebab-case") {
		t.Errorf("stderr no contiene advertencia de kebab-case: %s", stderr1.String())
	}

	// 2. Creación inicial
	var stdout2, stderr2 bytes.Buffer
	code2 := runChange([]string{"create", "feature-colision", "--intent", "Primera creación", "--cwd", tempDir}, &stdout2, &stderr2)
	if code2 != 0 {
		t.Fatalf("primera creación falló: %s", stderr2.String())
	}

	// 3. Colisión por duplicado
	var stdout3, stderr3 bytes.Buffer
	code3 := runChange([]string{"create", "feature-colision", "--intent", "Segunda creación duplicada", "--cwd", tempDir}, &stdout3, &stderr3)
	if code3 != 1 {
		t.Errorf("esperado código 1 para cambio colisionante, obtenido %d", code3)
	}
	if !strings.Contains(stderr3.String(), "ya existe") {
		t.Errorf("stderr no contiene advertencia de cambio existente: %s", stderr3.String())
	}
}

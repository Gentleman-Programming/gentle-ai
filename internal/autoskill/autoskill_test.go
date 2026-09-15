package autoskill

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientFetchIndexAndSHA256(t *testing.T) {
	skillContent := []byte("# React Best Practices\n\nDirectrices de React...")
	hasher := sha256.New()
	hasher.Write(skillContent)
	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	mockIndex := RegistryIndex{
		Version:     1,
		GeneratedAt: "2026-09-14T20:00:00Z",
		Skills: map[string]RegistrySkillEntry{
			"react-best-practices": {
				Source:    "vercel-labs/agent-skills",
				SkillPath: "react-best-practices",
				Files:     []string{"SKILL.md"},
				SHA256: map[string]string{
					"SKILL.md": expectedHash,
				},
			},
		},
	}
	indexBytes, _ := json.Marshal(mockIndex)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(indexBytes)
		case "/react-best-practices/SKILL.md":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write(skillContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(ClientOptions{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	})

	ctx := context.Background()

	// 1. Fetch index
	idx, err := client.FetchIndex(ctx)
	if err != nil {
		t.Fatalf("FetchIndex() falló: %v", err)
	}
	if len(idx.Skills) != 1 {
		t.Errorf("Esperada 1 skill en index, obtenidas %d", len(idx.Skills))
	}

	// 2. Fetch and verify skill
	entry := idx.Skills["react-best-practices"]
	files, err := client.FetchAndVerifySkill(ctx, "react-best-practices", entry)
	if err != nil {
		t.Fatalf("FetchAndVerifySkill() falló: %v", err)
	}

	if string(files["SKILL.md"]) != string(skillContent) {
		t.Errorf("Contenido descargado no coincide con el esperado")
	}
}

func TestClientSHA256MismatchRejection(t *testing.T) {
	alteredContent := []byte("# Contenido alterado maliciosamente")

	mockIndex := RegistryIndex{
		Version:     1,
		GeneratedAt: "2026-09-14T20:00:00Z",
		Skills: map[string]RegistrySkillEntry{
			"compromised-skill": {
				Source:    "unknown",
				SkillPath: "compromised-skill",
				Files:     []string{"SKILL.md"},
				SHA256: map[string]string{
					"SKILL.md": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", // Hash diferente
				},
			},
		},
	}
	indexBytes, _ := json.Marshal(mockIndex)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.json":
			_, _ = w.Write(indexBytes)
		case "/compromised-skill/SKILL.md":
			_, _ = w.Write(alteredContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(ClientOptions{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	})

	ctx := context.Background()
	entry := mockIndex.Skills["compromised-skill"]
	_, err := client.FetchAndVerifySkill(ctx, "compromised-skill", entry)
	if err == nil {
		t.Fatal("Esperado fallo de verificación SHA-256 ante hash alterado, pero la descarga fue aceptada")
	}
	if !strings.Contains(err.Error(), "discrepancia de integridad criptográfica SHA-256") {
		t.Errorf("Mensaje de error inesperado: %v", err)
	}
}

func TestDetector(t *testing.T) {
	tmpDir := t.TempDir()

	// Crear package.json con dependencia react
	pkgJSON := `{
		"name": "sample-ui",
		"dependencies": {
			"react": "^18.2.0",
			"react-dom": "^18.2.0"
		}
	}`
	_ = os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(pkgJSON), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module sample.com/app\n\ngo 1.25\n"), 0644)

	detector := NewDetector(SKILLS_MAP)
	detected, err := detector.Detect(tmpDir, "")
	if err != nil {
		t.Fatalf("Detect() retornó error: %v", err)
	}

	foundReact := false
	foundGo := false
	for _, d := range detected {
		if d.Tech.ID == "react" {
			foundReact = true
		}
		if d.Tech.ID == "go" {
			foundGo = true
		}
	}

	if !foundReact {
		t.Error("No se detectó React a partir de package.json")
	}
	if !foundGo {
		t.Error("No se detectó Go a partir de go.mod")
	}
}

func TestMiner(t *testing.T) {
	tmpDir := t.TempDir()

	// Crear archivo Go con pruebas tabulares y envoltorio de errores
	testFile := `package foo_test

import (
	"fmt"
	"testing"
)

func TestTableDriven(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "test 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = fmt.Errorf("error envuelto: %w", nil)
		})
	}
}
`
	_ = os.WriteFile(filepath.Join(tmpDir, "sample_test.go"), []byte(testFile), 0644)

	miner := NewMiner()
	proposals, err := miner.MinePatterns(tmpDir)
	if err != nil {
		t.Fatalf("MinePatterns() falló: %v", err)
	}

	foundTableTests := false
	for _, p := range proposals {
		if p.Name == "axiom-go-table-tests" {
			foundTableTests = true
			if !strings.Contains(p.SkillMD, "Table-Driven Tests") {
				t.Errorf("Contenido de SkillMD minado inesperado: %s", p.SkillMD)
			}
		}
	}

	if !foundTableTests {
		t.Error("Miner no descubrió el patrón axiom-go-table-tests")
	}
}

func TestManagerLifecycle(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Configurar mock server
	skillContent := []byte("# Docker Best Practices\n\nUso de contenedores...")
	hasher := sha256.New()
	hasher.Write(skillContent)
	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	mockIndex := RegistryIndex{
		Version:     1,
		GeneratedAt: "2026-09-14T20:00:00Z",
		Skills: map[string]RegistrySkillEntry{
			"docker-best-practices": {
				Source:    "midudev/autoskills",
				SkillPath: "docker-best-practices",
				Files:     []string{"SKILL.md"},
				SHA256: map[string]string{
					"SKILL.md": expectedHash,
				},
			},
		},
	}
	indexBytes, _ := json.Marshal(mockIndex)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.json":
			_, _ = w.Write(indexBytes)
		case "/docker-best-practices/SKILL.md":
			_, _ = w.Write(skillContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Crear Dockerfile para detección
	_ = os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte("FROM alpine:latest"), 0644)

	client := NewClient(ClientOptions{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	})
	detector := NewDetector(SKILLS_MAP)
	miner := NewMiner()
	manager := NewManager(tmpDir, client, detector, miner)

	ctx := context.Background()

	// 2. Scan
	report, err := manager.Scan(ctx, "", false)
	if err != nil {
		t.Fatalf("manager.Scan() falló: %v", err)
	}
	if len(report.SkillsProposed) == 0 {
		t.Fatal("Esperadas propuestas en ScanReport, ninguna recibida")
	}

	// 3. ListInbox
	inbox, err := manager.ListInbox()
	if err != nil {
		t.Fatalf("manager.ListInbox() falló: %v", err)
	}
	if len(inbox) == 0 {
		t.Fatal("El buzón debería contener propuestas pendientes")
	}

	// 4. Approve
	approvedSkill := inbox[0].Metadata.Name
	err = manager.Approve(approvedSkill)
	if err != nil {
		t.Fatalf("manager.Approve(%s) falló: %v", approvedSkill, err)
	}

	// Verificar que está en skills/ y ya no en inbox/
	targetSkillFile := filepath.Join(tmpDir, "skills", approvedSkill, "SKILL.md")
	if _, err := os.Stat(targetSkillFile); os.IsNotExist(err) {
		t.Errorf("El archivo promocionado %s no existe en skills/", targetSkillFile)
	}

	inboxAfterApprove, _ := manager.ListInbox()
	for _, item := range inboxAfterApprove {
		if item.Metadata.Name == approvedSkill {
			t.Errorf("La skill aprobada %s todavía reside en el buzón", approvedSkill)
		}
	}

	// 5. Reject de otra skill si existe, o crear una propuesta ad-hoc para probar Reject
	testRejectSkill := "skill-de-prueba"
	inboxTestFolder := filepath.Join(tmpDir, ".axiom", "skills", "inbox", testRejectSkill)
	_ = os.MkdirAll(inboxTestFolder, 0755)
	_ = os.WriteFile(filepath.Join(inboxTestFolder, "SKILL.md"), []byte("# Temporal"), 0644)

	err = manager.Reject(testRejectSkill)
	if err != nil {
		t.Fatalf("manager.Reject(%s) falló: %v", testRejectSkill, err)
	}

	if _, err := os.Stat(inboxTestFolder); !os.IsNotExist(err) {
		t.Errorf("La carpeta rechazada %s aún existe en el buzón", inboxTestFolder)
	}
}

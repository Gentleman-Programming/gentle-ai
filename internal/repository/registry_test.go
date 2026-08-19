package repository

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseRegistry(t *testing.T) {
	tempDir := t.TempDir()
	registryPath := filepath.Join(tempDir, "repository-registry.md")
	content := `
# Repository Registry

| Repository (gitlab_path) | repo-slug | Owner | Type | Purpose | Profile |
|---|---|---|---|---|---|
| ` + "`" + `gp-apps-cross/Pagos` + "`" + ` | ` + "`" + `gp-apps-cross-pagos` + "`" + ` | gp-apps-cross | backend (.NET, Clean Architecture) | Payments backend | ` + "`" + `skills/repo-profiles/payments-api/SKILL.md` + "`" + ` |
| ` + "`" + `SmartClic/MSPagos` + "`" + ` | ` + "`" + `smartclic-mspagos` + "`" + ` | SmartClic | backend (legacy) | Legacy payments backend | none |
`
	if err := os.WriteFile(registryPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	repos, err := ParseRegistry(registryPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}

	pagos, ok := repos["gp-apps-cross-pagos"]
	if !ok {
		t.Fatal("expected to find gp-apps-cross-pagos")
	}

	if pagos.GitlabPath != "gp-apps-cross/Pagos" {
		t.Errorf("expected gitlabPath gp-apps-cross/Pagos, got %s", pagos.GitlabPath)
	}
	if pagos.Owner != "gp-apps-cross" {
		t.Errorf("expected owner gp-apps-cross, got %s", pagos.Owner)
	}
	if pagos.Profile != "skills/repo-profiles/payments-api/SKILL.md" {
		t.Errorf("expected profile skills/repo-profiles/payments-api/SKILL.md, got %s", pagos.Profile)
	}
}

func TestParseRegistryNotFound(t *testing.T) {
	repos, err := ParseRegistry("does-not-exist.md")
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if len(repos) != 0 {
		t.Fatalf("expected 0 repos, got %d", len(repos))
	}
}

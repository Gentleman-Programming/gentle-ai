package registry_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/components/registry"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestNewRegistryClient(t *testing.T) {
	c := registry.NewRegistryClient("http://example.com", t.TempDir())
	if c == nil {
		t.Fatal("NewRegistryClient returned nil")
	}
}

func TestRegistryClientList(t *testing.T) {
	entries := []registry.RegistryEntry{
		{ID: "dev-01", Name: "Developer", Author: "team-a", Version: "1.0.0", Quality: 90},
		{ID: "sec-01", Name: "Security", Author: "team-b", Version: "1.0.0", Quality: 85},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
	}))
	defer srv.Close()

	c := registry.NewRegistryClient(srv.URL, t.TempDir())
	got, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("List returned %d entries, want 2", len(got))
	}
	if got[0].ID != "dev-01" {
		t.Errorf("first entry ID = %q, want %q", got[0].ID, "dev-01")
	}
}

func TestRegistryClientListHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := registry.NewRegistryClient(srv.URL, t.TempDir())
	_, err := c.List(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP 503")
	}
}

func TestRegistryClientInstall(t *testing.T) {
	profile := model.RoleProfile{
		ID:          "sec-01",
		Name:        "Security",
		Description: "Security profile",
		Role:        model.RoleCybersecurity,
		Persona: model.PersonaOverride{
			Base: model.PersonaGentleman,
			Tone: "adversarial",
		},
		Skills: []model.SkillRef{
			{ID: "pentest-methodology", Priority: "primary"},
		},
		MCPConfig: []model.MCPServerRef{
			{Name: "nuclei-mcp", Category: "dast", Priority: "required"},
		},
	}

	// Serve the registry index
	entries := []registry.RegistryEntry{
		{ID: "sec-01", Name: "Security", URL: "/profiles/sec-01.json"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/profiles.json":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(entries)
		case "/profiles/sec-01.json":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(profile)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := registry.NewRegistryClient(srv.URL, t.TempDir())
	got, err := c.Install(context.Background(), "sec-01")
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if got.ID != "sec-01" {
		t.Errorf("Install() ID = %q, want %q", got.ID, "sec-01")
	}
	if got.Role != model.RoleCybersecurity {
		t.Errorf("Install() Role = %q, want %q", got.Role, model.RoleCybersecurity)
	}
}

func TestRegistryClientInstallNotFound(t *testing.T) {
	entries := []registry.RegistryEntry{
		{ID: "dev-01", Name: "Developer", URL: "/profiles/dev-01.json"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
	}))
	defer srv.Close()

	c := registry.NewRegistryClient(srv.URL, t.TempDir())
	_, err := c.Install(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent profile ID")
	}
}

func TestRegistryClientSearch(t *testing.T) {
	entries := []registry.RegistryEntry{
		{ID: "dev-01", Name: "Developer", Tags: []string{"development", "go"}, Quality: 90},
		{ID: "sec-01", Name: "Security", Tags: []string{"security", "pentest"}, Quality: 85},
		{ID: "sec-02", Name: "Sec Advanced", Tags: []string{"security", "forensics"}, Quality: 88},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
	}))
	defer srv.Close()

	c := registry.NewRegistryClient(srv.URL, t.TempDir())

	// Search by tag
	got, err := c.Search(context.Background(), "", []string{"security"})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("Search by tag returned %d entries, want 2", len(got))
	}

	// Search by role (all entries match since we don't filter by role in mock)
	got, err = c.Search(context.Background(), model.RoleDeveloper, nil)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("Search by role returned %d entries, want 3", len(got))
	}
}

func TestRegistryClientSearchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := registry.NewRegistryClient(srv.URL, t.TempDir())
	_, err := c.Search(context.Background(), model.RoleDeveloper, nil)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestRegistryEntryYAMLTags(t *testing.T) {
	// Verify that RegistryEntry can be used with JSON (used in HTTP responses)
	entry := registry.RegistryEntry{
		ID:          "test-01",
		Name:        "Test Profile",
		Description: "A test profile",
		Author:      "tester",
		Version:     "1.0.0",
		Tags:        []string{"test", "example"},
		Quality:     95.0,
		URL:         "http://example.com/profile.json",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var restored registry.RegistryEntry
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if restored.ID != entry.ID {
		t.Errorf("ID = %q, want %q", restored.ID, entry.ID)
	}
	if len(restored.Tags) != 2 {
		t.Errorf("Tags len = %d, want 2", len(restored.Tags))
	}
}

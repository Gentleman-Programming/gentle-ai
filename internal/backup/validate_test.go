package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRestoredFile_JSONValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"key":"value"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	warn := ValidateRestoredFile(path)
	if warn != "" {
		t.Errorf("ValidateRestoredFile() = %q, want empty", warn)
	}
}

func TestValidateRestoredFile_JSONInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(path, []byte(`{"key":`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	warn := ValidateRestoredFile(path)
	if warn == "" {
		t.Fatal("ValidateRestoredFile() expected warning for invalid JSON")
	}
	if !strings.Contains(warn, "invalid JSON") {
		t.Errorf("warning = %q, want 'invalid JSON'", warn)
	}
}

func TestValidateRestoredFile_JSONEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	warn := ValidateRestoredFile(path)
	if warn == "" {
		t.Fatal("ValidateRestoredFile() expected warning for empty JSON")
	}
	if !strings.Contains(warn, "empty") {
		t.Errorf("warning = %q, want 'empty'", warn)
	}
}

func TestValidateRestoredFile_TOMLValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[section]\nkey = \"value\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	warn := ValidateRestoredFile(path)
	if warn != "" {
		t.Errorf("ValidateRestoredFile() = %q, want empty", warn)
	}
}

func TestValidateRestoredFile_TOMLBogus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.toml")
	if err := os.WriteFile(path, []byte("this is not toml at all"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	warn := ValidateRestoredFile(path)
	if warn == "" {
		t.Fatal("ValidateRestoredFile() expected warning for bogus TOML")
	}
	if !strings.Contains(warn, "lacks expected structure") {
		t.Errorf("warning = %q, want structure warning", warn)
	}
}

func TestValidateRestoredFile_YAMLValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("key: value\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	warn := ValidateRestoredFile(path)
	if warn != "" {
		t.Errorf("ValidateRestoredFile() = %q, want empty", warn)
	}
}

func TestValidateRestoredFile_YAMLListValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "list.yaml")
	if err := os.WriteFile(path, []byte("- item1\n- item2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	warn := ValidateRestoredFile(path)
	if warn != "" {
		t.Errorf("ValidateRestoredFile() = %q, want empty", warn)
	}
}

func TestValidateRestoredFile_YAMLBogus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.yaml")
	if err := os.WriteFile(path, []byte("this is not yaml"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	warn := ValidateRestoredFile(path)
	if warn == "" {
		t.Fatal("ValidateRestoredFile() expected warning for bogus YAML")
	}
	if !strings.Contains(warn, "lacks expected structure") {
		t.Errorf("warning = %q, want structure warning", warn)
	}
}

func TestValidateRestoredFile_UnknownExtensionSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(path, []byte("# Hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	warn := ValidateRestoredFile(path)
	if warn != "" {
		t.Errorf("ValidateRestoredFile() = %q, want empty for unknown extension", warn)
	}
}

func TestValidateRestoredFile_NonExistentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	warn := ValidateRestoredFile(path)
	if warn == "" {
		t.Fatal("ValidateRestoredFile() expected warning for missing file")
	}
	if !strings.Contains(warn, "cannot read") {
		t.Errorf("warning = %q, want 'cannot read'", warn)
	}
}

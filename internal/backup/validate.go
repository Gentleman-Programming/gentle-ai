package backup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateRestoredFile performs best-effort validation of a restored file
// based on its extension. It reads the file from disk. For callers that
// already have the content in memory (e.g., restoreEntry), prefer
// ValidateRestoredContent to avoid redundant I/O.
func ValidateRestoredFile(path string) string {
	return validateByExtension(path, nil)
}

// ValidateRestoredContent performs the same validation as ValidateRestoredFile
// but uses the provided in-memory content instead of reading from disk.
//
// Path is used only to determine the file extension for format selection.
// Content must be the full file content that was written.
func ValidateRestoredContent(path string, content []byte) string {
	return validateByExtension(path, content)
}

// validateByExtension dispatches to the appropriate format validator based
// on the file extension. If content is nil, the file is read from disk.
func validateByExtension(path string, content []byte) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return validateJSON(path, content)
	case ".toml":
		return validateTOML(path, content)
	case ".yaml", ".yml":
		return validateYAML(path, content)
	default:
		return ""
	}
}

// validateJSON validates JSON content. If content is nil, reads from disk.
func validateJSON(path string, content []byte) string {
	if content == nil {
		if err := readFileContent(path, &content); err != "" {
			return err
		}
	}
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return fmt.Sprintf("restored JSON file %q is empty", path)
	}
	if !json.Valid(trimmed) {
		return fmt.Sprintf("restored JSON file %q contains invalid JSON", path)
	}
	return ""
}

// validateTOML validates TOML content with a lightweight structural check.
// If content is nil, reads from disk.
func validateTOML(path string, content []byte) string {
	if content == nil {
		if err := readFileContent(path, &content); err != "" {
			return err
		}
	}
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return fmt.Sprintf("restored TOML file %q is empty", path)
	}
	if !bytes.ContainsAny(trimmed, "=[") {
		return fmt.Sprintf("restored TOML file %q lacks expected structure (no '=' or '[')", path)
	}
	return ""
}

// validateYAML validates YAML content with a lightweight structural check.
// If content is nil, reads from disk.
func validateYAML(path string, content []byte) string {
	if content == nil {
		if err := readFileContent(path, &content); err != "" {
			return err
		}
	}
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return fmt.Sprintf("restored YAML file %q is empty", path)
	}
	hasStructure := bytes.Contains(trimmed, []byte(":")) || bytes.HasPrefix(trimmed, []byte("-"))
	if !hasStructure {
		return fmt.Sprintf("restored YAML file %q lacks expected structure (no ':' or list items)", path)
	}
	return ""
}

// readFileContent reads the file at path into *out. Returns an empty string
// on success or a formatted error string on failure.
func readFileContent(path string, out *[]byte) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("cannot read %q for validation: %v", path, err)
	}
	*out = data
	return ""
}

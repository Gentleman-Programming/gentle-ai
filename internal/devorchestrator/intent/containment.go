package intent

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrIdentifierContainment is returned when a caller-supplied change or
// source identifier, or a workspace-relative artifact path, would resolve
// outside the intended change tree once turned into a filesystem path (T1).
// Containment is checked independently of, and before, the existing
// changeowner ownership check (refusal precedence #1).
var ErrIdentifierContainment = errors.New("intent: identifier escapes the intended change tree")

// ValidateIdentifier rejects a bare change/source identifier that could
// escape the openspec/changes/<id> tree once joined into a path: absolute
// paths, path separators, traversal segments, NUL bytes, and empty values.
// It MUST run before the identifier is used in any filepath.Join and before
// any filesystem side effect (MkdirAll/WriteFile/AssertCanWrite).
func ValidateIdentifier(id string) error {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return fmt.Errorf("%w: identifier is empty", ErrIdentifierContainment)
	}
	if strings.ContainsRune(trimmed, 0) {
		return fmt.Errorf("%w: %q contains a NUL byte", ErrIdentifierContainment, trimmed)
	}
	if filepath.IsAbs(trimmed) || strings.ContainsAny(trimmed, `/\`) {
		return fmt.Errorf("%w: %q contains a path separator or is an absolute path", ErrIdentifierContainment, trimmed)
	}
	if strings.Contains(trimmed, "..") {
		return fmt.Errorf("%w: %q contains a traversal segment", ErrIdentifierContainment, trimmed)
	}
	return nil
}

// ValidateContainedPath rejects a workspace-relative path (e.g. the
// primaryArtifact/sourceArtifact GenerateContextForAgent receives) whose
// resolved location would escape root once joined and cleaned. Unlike
// ValidateIdentifier, relPath is expected to contain path separators (it
// names a file inside the change tree); only escape from root is refused.
func ValidateContainedPath(root, relPath string) error {
	if strings.ContainsRune(relPath, 0) {
		return fmt.Errorf("%w: %q contains a NUL byte", ErrIdentifierContainment, relPath)
	}
	if filepath.IsAbs(relPath) {
		return fmt.Errorf("%w: %q is an absolute path", ErrIdentifierContainment, relPath)
	}
	cleanRoot := filepath.Clean(root)
	rel, err := filepath.Rel(cleanRoot, filepath.Join(cleanRoot, relPath))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %q resolves outside the workspace root", ErrIdentifierContainment, relPath)
	}
	return nil
}

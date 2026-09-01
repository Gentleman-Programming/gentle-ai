package sddstatus

import (
	"errors"
	"fmt"
	"go/build/constraint"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const retiredLegacyCompactReceiptTag = "legacy_compact_receipt"

func TestLegacyCompactReceiptBuildConstraintIsAbsentRepositoryWide(t *testing.T) {
	repoRoot := legacyCompactReceiptRepositoryRoot(t)

	matches, err := legacyCompactReceiptTaggedGoFiles(repoRoot)
	if err != nil {
		t.Fatalf("scan repository Go build constraints: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("retired %s build constraint remains in: %v", retiredLegacyCompactReceiptTag, matches)
	}
}

func TestLegacyCompactReceiptScannerDetectsTaggedFile(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "standard-space directives",
			source: "//go:build legacy_compact_receipt\n// +build legacy_compact_receipt\n\npackage nested\n",
		},
		{
			name:   "tab-separated directives",
			source: "//go:build\tlegacy_compact_receipt\n// +build\tlegacy_compact_receipt\n\npackage nested\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			if err := os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.invalid/d6\n\ngo 1.25\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			fixturePath := filepath.Join(repoRoot, "nested", "tagged.go")
			if err := os.MkdirAll(filepath.Dir(fixturePath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(fixturePath, []byte(tt.source), 0o644); err != nil {
				t.Fatal(err)
			}

			matches, err := legacyCompactReceiptTaggedGoFiles(repoRoot)
			if err != nil {
				t.Fatalf("scan synthetic repository: %v", err)
			}
			if len(matches) != 1 || matches[0] != "nested/tagged.go" {
				t.Fatalf("tagged Go files = %v, want [nested/tagged.go]", matches)
			}
		})
	}
}

func legacyCompactReceiptRepositoryRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get test working directory: %v", err)
	}
	for {
		info, err := os.Stat(filepath.Join(dir, "go.mod"))
		switch {
		case err == nil && !info.IsDir():
			return dir
		case err == nil:
			t.Fatalf("go.mod under %q is not a file", dir)
		case !errors.Is(err, fs.ErrNotExist):
			t.Fatalf("stat go.mod under %q: %v", dir, err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("locate repository root from test working directory: go.mod not found")
		}
		dir = parent
	}
}

func legacyCompactReceiptTaggedGoFiles(repoRoot string) ([]string, error) {
	var matches []string
	err := filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}

		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		usesRetiredTag, err := sourceUsesLegacyCompactReceiptTag(string(source))
		if err != nil {
			return fmt.Errorf("parse build constraint in %q: %w", path, err)
		}
		if !usesRetiredTag {
			return nil
		}
		relativePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		matches = append(matches, filepath.ToSlash(relativePath))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

func sourceUsesLegacyCompactReceiptTag(source string) (bool, error) {
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "package ") {
			return false, nil
		}
		if !constraint.IsGoBuild(trimmed) && !constraint.IsPlusBuild(trimmed) {
			continue
		}
		expression, err := constraint.Parse(trimmed)
		if err != nil {
			return false, err
		}
		if buildConstraintContainsTag(expression, retiredLegacyCompactReceiptTag) {
			return true, nil
		}
	}
	return false, nil
}

func buildConstraintContainsTag(expression constraint.Expr, tag string) bool {
	switch expression := expression.(type) {
	case *constraint.TagExpr:
		return expression.Tag == tag
	case *constraint.NotExpr:
		return buildConstraintContainsTag(expression.X, tag)
	case *constraint.AndExpr:
		return buildConstraintContainsTag(expression.X, tag) || buildConstraintContainsTag(expression.Y, tag)
	case *constraint.OrExpr:
		return buildConstraintContainsTag(expression.X, tag) || buildConstraintContainsTag(expression.Y, tag)
	default:
		return false
	}
}

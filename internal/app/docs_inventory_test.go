package app_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// componentPackages returns the directories under internal/components that
// contain Go files, i.e. the component packages.
func componentPackages(t *testing.T) map[string]bool {
	t.Helper()
	root := filepath.Join("..", "components")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read %s: %v", root, err)
	}
	packages := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		goFiles, err := filepath.Glob(filepath.Join(root, entry.Name(), "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		if len(goFiles) > 0 {
			packages[entry.Name()] = true
		}
	}
	return packages
}

// architectureComponentTree returns the packages listed under `  components/`
// in the docs/architecture.md tree: the leading `name/` fields of each deeper
// indented line, skipping blank lines and stopping at the next nonblank
// sibling of components/. Names are taken
// whole, whatever their spelling, so an invented entry is reported instead of
// skipped; a line that does not start with a `name/` field fails the test.
func architectureComponentTree(t *testing.T, content string) []string {
	t.Helper()
	var names []string
	inTree := false
	for _, line := range strings.Split(content, "\n") {
		if !inTree {
			inTree = strings.HasPrefix(line, "  components/")
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, "    ") {
			break
		}
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.HasSuffix(fields[0], "/") {
			t.Errorf("docs/architecture.md components tree: unrecognized entry %q", strings.TrimSpace(line))
			continue
		}
		for _, field := range fields {
			if !strings.HasSuffix(field, "/") {
				break
			}
			names = append(names, strings.TrimSuffix(field, "/"))
		}
	}
	if !inTree {
		t.Fatal("docs/architecture.md: `  components/` tree entry not found")
	}
	return names
}

func diffPackages(listed []string, actual map[string]bool, checkMissing bool) (missing, invented []string) {
	seen := map[string]bool{}
	for _, name := range listed {
		seen[name] = true
		if !actual[name] {
			invented = append(invented, name)
		}
	}
	if checkMissing {
		for name := range actual {
			if !seen[name] {
				missing = append(missing, name)
			}
		}
	}
	sort.Strings(missing)
	sort.Strings(invented)
	return missing, invented
}

func TestComponentTreesMatchPackages(t *testing.T) {
	actual := componentPackages(t)

	read := func(name string) string {
		content, err := os.ReadFile(filepath.Join("..", "..", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(content)
	}

	// docs/architecture.md carries the full package tree: compare 1:1.
	missing, invented := diffPackages(architectureComponentTree(t, read("docs/architecture.md")), actual, true)
	if len(missing) > 0 || len(invented) > 0 {
		t.Errorf("docs/architecture.md components tree drifted from internal/components\nmissing: %v\ninvented: %v", missing, invented)
	}

	// docs/codebase/repository-map.md is an ownership table that names only
	// selected component packages, so every package it names must exist.
	referenced := repositoryMapReferences(read("docs/codebase/repository-map.md"))
	if len(referenced) == 0 {
		t.Fatal("docs/codebase/repository-map.md: no internal/components/<package>/ references found; update the parser if the format changed")
	}
	if _, invented := diffPackages(referenced, actual, false); len(invented) > 0 {
		t.Errorf("docs/codebase/repository-map.md names packages missing from internal/components\ninvented: %v", invented)
	}
}

func TestComponentParsersReportInventedNamesOfAnyShape(t *testing.T) {
	actual := map[string]bool{"communitytool": true, "engram": true}

	tree := "  components/              Per-component logic\n    engram/  ghost-pkg/\n    Ghost.Pkg/             Invented\n\n    ghost/                 After a blank line\n  skillregistry/\n"
	_, invented := diffPackages(architectureComponentTree(t, tree), actual, true)
	if got := strings.Join(invented, ","); got != "Ghost.Pkg,ghost,ghost-pkg" {
		t.Fatalf("architecture tree invented = %q, want %q", got, "Ghost.Pkg,ghost,ghost-pkg")
	}

	mapDoc := "| `internal/components/communitytool/` | ok |\n| `internal/components/ghost-pkg/` | x |\n" +
		"| `internal/components/ghost` | x |\nSee internal/components/engram/README.md and internal/components/Ghost.\n| `internal/components/` | root |\n"
	refs := repositoryMapReferences(mapDoc)
	if got := strings.Join(refs, ","); got != "communitytool,ghost-pkg,ghost,engram,Ghost" {
		t.Fatalf("repositoryMapReferences = %q, want %q", got, "communitytool,ghost-pkg,ghost,engram,Ghost")
	}
	_, invented = diffPackages(refs, actual, false)
	if got := strings.Join(invented, ","); got != "Ghost,ghost,ghost-pkg" {
		t.Fatalf("repository map invented = %q, want %q", got, "Ghost,ghost,ghost-pkg")
	}
}

func TestArchitectureComponentTreeParsesOnlyComponents(t *testing.T) {
	content := "internal/\n  catalog/                 Registry (agents, skills/)\n  components/              Per-component logic\n" +
		"    engram/  skills/\n    filemerge/             Marker-based install/inject merging\n    ghost_pkg/             Invented\n  skillregistry/           Refresh\n    nested/\n"
	listed := architectureComponentTree(t, content)
	if got := strings.Join(listed, ","); got != "engram,skills,filemerge,ghost_pkg" {
		t.Fatalf("architectureComponentTree = %q, want %q", got, "engram,skills,filemerge,ghost_pkg")
	}
	missing, invented := diffPackages(listed, map[string]bool{"engram": true, "skills": true, "uninstall": true}, true)
	if strings.Join(missing, ",") != "uninstall" || strings.Join(invented, ",") != "filemerge,ghost_pkg" {
		t.Fatalf("diffPackages missing=%v invented=%v, want [uninstall] [filemerge ghost_pkg]", missing, invented)
	}
}

// repositoryMapReferences returns every package name that follows
// internal/components/ in content, whatever its spelling, so an invented
// name is reported instead of skipped.
func repositoryMapReferences(content string) []string {
	var refs []string
	for _, match := range componentRef.FindAllStringSubmatch(content, -1) {
		if name := strings.TrimRight(match[1], "."); name != "" {
			refs = append(refs, name)
		}
	}
	return refs
}

// componentRef captures the full path segment after internal/components/, up
// to whitespace, a slash or Markdown/prose punctuation.
var componentRef = regexp.MustCompile("internal/components/([^\\s/`'\"()\\[\\]|,;:*<>]+)")

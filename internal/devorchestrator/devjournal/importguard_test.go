package devjournal

import (
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// TestPackageImportsExcludeSDDStatus parses every Go source file in this
// package directory and asserts internal/sddstatus is absent from every
// import set. This makes "phase is unwritable" a build-time, AST-level
// guarantee (design D2, binding decision Q5) that a future edit cannot
// silently regress by reintroducing the import.
func TestPackageImportsExcludeSDDStatus(t *testing.T) {
	const forbiddenImport = "github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("ParseDir(.): %v", err)
	}

	filesSeen := 0
	for pkgName, pkg := range pkgs {
		for filename, file := range pkg.Files {
			filesSeen++
			for _, imp := range file.Imports {
				path, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					t.Fatalf("unquote import path %q in %s: %v", imp.Path.Value, filename, err)
				}
				if path == forbiddenImport || strings.HasPrefix(path, forbiddenImport+"/") {
					t.Fatalf("package %q file %s imports %q; devjournal must never import internal/sddstatus", pkgName, filename, path)
				}
			}
		}
	}
	if filesSeen == 0 {
		t.Fatalf("guard is vacuous: parser.ParseDir(\".\") found zero files in the devjournal package directory")
	}
}

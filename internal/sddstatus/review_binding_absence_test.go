package sddstatus

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestReviewBindingCodecDeclarationsRemoved(t *testing.T) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "review_binding.go", nil, 0)
	if err != nil {
		t.Fatalf("parse review_binding.go: %v", err)
	}

	declared := map[string]bool{}
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			declared[declaration.Name.Name] = true
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					declared[spec.Name.Name] = true
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						declared[name.Name] = true
					}
				}
			}
		}
	}

	for _, name := range []string{
		"ReviewBinding", "parseBinding", "bindingBytes", "bindingDigest", "bindingHash", "bindingPath", "reviewBindingViolation",
		"reviewBindingSchema", "reviewBindingChange", "reviewBindingLineage", "reviewBindingHash",
		"validReviewBindingChange", "validReviewBindingLineage",
	} {
		if declared[name] {
			t.Errorf("review_binding.go still declares removed provider codec symbol %q", name)
		}
	}

	for _, name := range []string{"resolveBindingChangeRoot", "bindingChangeRoots", "canonicalBindingPath", "pathWithinBindingRoot"} {
		if !declared[name] {
			t.Errorf("review_binding.go no longer declares retained authority-free path helper %q", name)
		}
	}
}

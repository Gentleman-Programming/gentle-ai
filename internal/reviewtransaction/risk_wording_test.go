package reviewtransaction

import (
	"strings"
	"testing"
)

const wordingProofBase = `package greet

import "fmt"

// Greet returns the greeting shown to a visitor.
func Greet(name string) string {
	return fmt.Sprintf("hello, %s", name) // keep the comma
}
`

func TestGoCommentOnlyChangeAcceptsOnlyCommentRewrites(t *testing.T) {
	for _, fixture := range []struct {
		name      string
		base      string
		candidate string
		want      bool
	}{
		{name: "line comment reworded", base: wordingProofBase,
			candidate: strings.Replace(wordingProofBase, "shown to a visitor", "a visitor sees", 1), want: true},
		{name: "trailing comment becomes block comment", base: wordingProofBase,
			candidate: strings.Replace(wordingProofBase, "// keep the comma", "/* the comma\n\tstays */", 1), want: true},
		{name: "comment added between tokens", base: wordingProofBase,
			candidate: strings.Replace(wordingProofBase, "func Greet(name string)", "func Greet(name /* visitor */ string)", 1), want: true},
		{name: "identical bytes", base: wordingProofBase, candidate: wordingProofBase, want: true},
		{name: "string literal", base: wordingProofBase,
			candidate: strings.Replace(wordingProofBase, `"hello, %s"`, `"hi, %s"`, 1)},
		{name: "code commented out", base: wordingProofBase,
			candidate: strings.Replace(wordingProofBase, "\treturn fmt", "\t// return fmt", 1)},
		{name: "build constraint", base: "//go:build linux\n\n" + wordingProofBase,
			candidate: "//go:build darwin\n\n" + wordingProofBase},
		{name: "build constraint added", base: wordingProofBase,
			candidate: "//go:build linux\n\n" + wordingProofBase},
		{name: "embed directive", base: wordingProofBase + "\n//go:embed a.txt\nvar a string\n",
			candidate: wordingProofBase + "\n//go:embed b.txt\nvar a string\n"},
		{name: "legacy build tag", base: "// +build linux\n\n" + wordingProofBase,
			candidate: "// +build darwin\n\n" + wordingProofBase},
		{name: "line directive", base: wordingProofBase,
			candidate: "//line greet.tmpl:1\n" + wordingProofBase},
		{name: "example output comment", base: "package greet\n\nfunc ExampleGreet() {\n\t// Output: hello\n}\n",
			candidate: "package greet\n\nfunc ExampleGreet() {\n\t// Output: hi\n}\n"},
		{name: "lowercase output comment", base: "package greet\n\nfunc ExampleGreet() {\n\t//output: hello\n}\n",
			candidate: "package greet\n\nfunc ExampleGreet() {\n\t//output: hi\n}\n"},
		{name: "multi-line example output", base: "package greet\n\nfunc ExampleGreet() {\n\t// Output:\n\t// hello\n}\n",
			candidate: "package greet\n\nfunc ExampleGreet() {\n\t// Output:\n\t// hi\n}\n"},
		{name: "cgo preamble", base: "package greet\n\n// int one() { return 1; }\nimport \"C\"\n",
			candidate: "package greet\n\n// int one() { return 2; }\nimport \"C\"\n"},
		{name: "cgo export", base: "package greet\n\n//export Greet\nfunc Greet() {}\n",
			candidate: "package greet\n\n//export Hello\nfunc Greet() {}\n"},
		{name: "unterminated comment", base: wordingProofBase,
			candidate: wordingProofBase + "/* never closed"},
		{name: "invalid utf8", base: wordingProofBase,
			candidate: strings.Replace(wordingProofBase, "visitor", "visit\xffor", 1)},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if got := goCommentOnlyChange([]byte(fixture.base), []byte(fixture.candidate)); got != fixture.want {
				t.Fatalf("goCommentOnlyChange = %v, want %v", got, fixture.want)
			}
		})
	}
}

func TestWordingOnlyGoCandidateStatRequiresAnUnchangedRegularGoFile(t *testing.T) {
	for _, fixture := range []struct {
		name string
		stat DiffStat
		want bool
	}{
		{name: "regular go edit", stat: DiffStat{Path: "internal/greet/greet.go", Additions: 1, Deletions: 1, OldMode: "100644", NewMode: "100644"}, want: true},
		{name: "test file edit", stat: DiffStat{Path: "internal/greet/greet_test.go", Additions: 1, Deletions: 1, OldMode: "100644", NewMode: "100644"}, want: true},
		{name: "added file", stat: DiffStat{Path: "internal/greet/greet.go", Additions: 3, OldMode: "000000", NewMode: "100644"}},
		{name: "deleted file", stat: DiffStat{Path: "internal/greet/greet.go", Deletions: 3, OldMode: "100644", NewMode: "000000"}},
		{name: "executable", stat: DiffStat{Path: "internal/greet/greet.go", Additions: 1, Deletions: 1, OldMode: "100755", NewMode: "100755"}},
		{name: "mode change", stat: DiffStat{Path: "internal/greet/greet.go", Additions: 1, Deletions: 1, OldMode: "100644", NewMode: "100755"}},
		{name: "symlink", stat: DiffStat{Path: "internal/greet/greet.go", Additions: 1, Deletions: 1, OldMode: "120000", NewMode: "120000"}},
		{name: "binary", stat: DiffStat{Path: "internal/greet/greet.go", Binary: true, OldMode: "100644", NewMode: "100644"}},
		{name: "shell", stat: DiffStat{Path: "scripts/greet.sh", Additions: 1, Deletions: 1, OldMode: "100644", NewMode: "100644"}},
		{name: "go.mod", stat: DiffStat{Path: "go.mod", Additions: 1, Deletions: 1, OldMode: "100644", NewMode: "100644"}},
		{name: "no line change", stat: DiffStat{Path: "internal/greet/greet.go", OldMode: "100644", NewMode: "100644"}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if got := isWordingOnlyGoCandidateStat(fixture.stat); got != fixture.want {
				t.Fatalf("isWordingOnlyGoCandidateStat(%#v) = %v, want %v", fixture.stat, got, fixture.want)
			}
		})
	}
}

package providercontractbundle

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestReviewExecutionContractDecoupling(t *testing.T) {
	source, err := os.ReadFile("review_execution.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "reviewassets.ReviewExecutionContractFor(agent)") {
		t.Fatal("bundle must consume reviewassets owner rather than render a separate contract")
	}

	old, err := reviewassets.ReviewExecutionContractFor(model.AgentPi)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := canonicalOrchestrationEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || string(entries[0].content) != strings.TrimSpace(old)+"\n" {
		t.Fatal("bundle contract bytes changed")
	}
	file, err := parser.ParseFile(token.NewFileSet(), "bundle.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range file.Imports {
		if strings.Contains(imp.Path.Value, "/components/sdd") {
			t.Fatal("bundle imports SDD")
		}
	}
}

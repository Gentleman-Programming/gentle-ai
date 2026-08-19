package router

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/context"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/trace"
)

func TestFormatPromptSignature(t *testing.T) {
	pkg := &context.Package{
		ExecutionID: "exec-1",
		Agent:       "frontend-implementer",
		Trace: trace.Node{
			ID:         "feature-123",
			Implements: []string{"spec-001"},
		},
		Scope: context.Scope{
			Repositories: []string{"repo-a"},
			Architecture: "spring-rest-service",
		},
		RepoProfile:         "## Profile for repo-a\nProfile contents here.",
		ArchitectureProfile: "## Profile for spring\nArchitecture contents here.",
		Permissions: context.Permissions{
			Code: "read",
			Git:  "read",
		},
		ExpectedOutput: context.Output{
			Type: "PROPOSAL",
			ID:   "proposal-123",
		},
	}

	baseInstruction := "Por favor, ejecuta la fase de exploración."

	out, err := FormatPromptSignature(baseInstruction, pkg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !strings.Contains(out, baseInstruction) {
		t.Errorf("Expected output to contain base instruction")
	}
	if !strings.Contains(out, "<context_package>") {
		t.Errorf("Expected output to contain context_package tag")
	}
	if !strings.Contains(out, "execution_id: exec-1") {
		t.Errorf("Expected output to contain execution_id")
	}
	if !strings.Contains(out, "agent: frontend-implementer") {
		t.Errorf("Expected output to contain agent name")
	}
	if !strings.Contains(out, "- spec-001") {
		t.Errorf("Expected output to contain trace implements")
	}
	if !strings.Contains(out, "code: read") {
		t.Errorf("Expected output to contain code permissions")
	}
	if !strings.Contains(out, "type: PROPOSAL") {
		t.Errorf("Expected output to contain expected output type")
	}
	if !strings.Contains(out, "<repo_profiles>") {
		t.Errorf("Expected output to contain repo_profiles tag")
	}
	if !strings.Contains(out, "<architecture_profile>") {
		t.Errorf("Expected output to contain architecture_profile tag")
	}
	if !strings.Contains(out, "architecture: spring-rest-service") {
		t.Errorf("Expected output to contain architecture in scope")
	}
	if !strings.Contains(out, "Profile contents here.") {
		t.Errorf("Expected output to contain repo profile contents")
	}
}

func TestFormatPromptSignature_NilPackage(t *testing.T) {
	_, err := FormatPromptSignature("instruction", nil)
	if err == nil {
		t.Errorf("Expected error for nil package")
	}
}

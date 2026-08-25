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

// TestFormatPromptSignature_RendersSkillsWhenPresent covers H-10: resolved
// skills must appear in the rendered agent prompt.
func TestFormatPromptSignature_RendersSkillsWhenPresent(t *testing.T) {
	pkg := &context.Package{
		ExecutionID: "exec-2",
		Agent:       "backend-implementer",
		Skills: []string{
			"skills/backend-implementer/SKILL.md",
			"skills/database-specialist/SKILL.md",
		},
	}

	out, err := FormatPromptSignature("Do work.", pkg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !strings.Contains(out, "<skills>") {
		t.Errorf("Expected output to contain <skills> tag when skills are resolved, got: %s", out)
	}
	for _, s := range pkg.Skills {
		if !strings.Contains(out, s) {
			t.Errorf("Expected output to contain resolved skill %q, got: %s", s, out)
		}
	}
}

// TestFormatPromptSignature_OmitsSkillsBlockWhenEmpty covers H-10's negative
// case: when no skills resolve, the skills block must be entirely absent,
// not rendered as an empty section.
func TestFormatPromptSignature_OmitsSkillsBlockWhenEmpty(t *testing.T) {
	pkg := &context.Package{
		ExecutionID: "exec-3",
		Agent:       "backend-implementer",
	}

	out, err := FormatPromptSignature("Do work.", pkg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if strings.Contains(out, "<skills>") {
		t.Errorf("Expected no <skills> block when no skills resolved, got: %s", out)
	}
}

// TestFormatPromptSignature_RendersDBImpactWhenPresent covers H-05's prompt
// side (design decision D6): when a DB impact was evaluated for the primary
// artifact, it must appear in the rendered prompt as "db_impact: <value>" so
// agents -- especially frontend-implementer -- are told about it explicitly.
func TestFormatPromptSignature_RendersDBImpactWhenPresent(t *testing.T) {
	pkg := &context.Package{
		ExecutionID: "exec-4",
		Agent:       "frontend-implementer",
		DBImpact:    "high-risk",
	}

	out, err := FormatPromptSignature("Do work.", pkg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !strings.Contains(out, "db_impact: high-risk") {
		t.Errorf("Expected output to contain 'db_impact: high-risk', got: %s", out)
	}
}

// TestFormatPromptSignature_OmitsDBImpactWhenEmpty covers the negative case:
// when no DB impact was evaluated, no db_impact line should be rendered.
func TestFormatPromptSignature_OmitsDBImpactWhenEmpty(t *testing.T) {
	pkg := &context.Package{
		ExecutionID: "exec-5",
		Agent:       "backend-implementer",
	}

	out, err := FormatPromptSignature("Do work.", pkg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if strings.Contains(out, "db_impact") {
		t.Errorf("Expected no db_impact line when DBImpact is empty, got: %s", out)
	}
}

// TestFormatPromptSignature_RendersDesignRefWhenPresent covers design
// decision D-D: when a design reference was recognized for the primary
// artifact, it must appear in the rendered prompt as "design_ref: <value>",
// mirroring how db_impact is rendered (see
// TestFormatPromptSignature_RendersDBImpactWhenPresent above).
func TestFormatPromptSignature_RendersDesignRefWhenPresent(t *testing.T) {
	pkg := &context.Package{
		ExecutionID: "exec-6",
		Agent:       "frontend-implementer",
		DesignRef:   "https://www.figma.com/design/ABC12345XY",
	}

	out, err := FormatPromptSignature("Do work.", pkg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !strings.Contains(out, "design_ref: https://www.figma.com/design/ABC12345XY") {
		t.Errorf("Expected output to contain 'design_ref: https://www.figma.com/design/ABC12345XY', got: %s", out)
	}
}

// TestFormatPromptSignature_OmitsDesignRefWhenEmpty covers the negative
// case: when no design reference was recognized, no substring "design_ref"
// may appear anywhere in the rendered prompt.
func TestFormatPromptSignature_OmitsDesignRefWhenEmpty(t *testing.T) {
	pkg := &context.Package{
		ExecutionID: "exec-7",
		Agent:       "backend-implementer",
	}

	out, err := FormatPromptSignature("Do work.", pkg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if strings.Contains(out, "design_ref") {
		t.Errorf("Expected no design_ref line when DesignRef is empty, got: %s", out)
	}
}

// TestFormatPromptSignature_DesignRefCannotBreakOutOfContextPackage covers
// the spec's only Applicable Threat Matrix row on the rendering side:
// db_impact rendering is unaffected by DesignRef being set (non-interference
// invariant), and the rendered design_ref line contains no embedded newline
// -- since Canonical() only ever emits a charset-bounded URL, this proves
// the rendering side never receives a payload capable of breaking out of
// <context_package>.
func TestFormatPromptSignature_DesignRefCannotBreakOutOfContextPackage(t *testing.T) {
	pkg := &context.Package{
		ExecutionID: "exec-8",
		Agent:       "frontend-implementer",
		DBImpact:    "high-risk",
		DesignRef:   "https://www.figma.com/design/ABC12345XY?node-id=1-2",
	}

	out, err := FormatPromptSignature("Do work.", pkg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !strings.Contains(out, "db_impact: high-risk") {
		t.Errorf("Expected db_impact rendering unaffected by DesignRef, got: %s", out)
	}
	if !strings.Contains(out, "design_ref: https://www.figma.com/design/ABC12345XY?node-id=1-2") {
		t.Errorf("Expected design_ref line to render fully on one line, got: %s", out)
	}

	designRefLineStart := strings.Index(out, "design_ref: ")
	restAfterLabel := out[designRefLineStart+len("design_ref: "):]
	firstNewline := strings.IndexByte(restAfterLabel, '\n')
	renderedValue := restAfterLabel[:firstNewline]
	if strings.ContainsAny(renderedValue, "\n\r\"'<>") {
		t.Errorf("Expected rendered design_ref value to contain no newline/quote/angle-bracket, got: %q", renderedValue)
	}
}

func TestFormatPromptSignature_NilPackage(t *testing.T) {
	_, err := FormatPromptSignature("instruction", nil)
	if err == nil {
		t.Errorf("Expected error for nil package")
	}
}

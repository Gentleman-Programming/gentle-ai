package router

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/context"
)

// AgentRouter format template for injecting Context Package into prompt
const promptTemplate = `{{ .BaseInstruction }}

<context_package>
execution_id: {{ .Pkg.ExecutionID }}
agent: {{ .Pkg.Agent }}
{{- if .Pkg.ExpectedOutput.Type }}
expected_output:
  type: {{ .Pkg.ExpectedOutput.Type }}
  id: {{ .Pkg.ExpectedOutput.ID }}
{{- end }}
trace:
  id: {{ .Pkg.Trace.ID }}
{{- if .Pkg.Trace.Implements }}
  implements:
{{- range .Pkg.Trace.Implements }}
    - {{ . }}
{{- end }}
{{- end }}
{{- if .Pkg.Trace.OriginatesFrom }}
  originates_from:
{{- range .Pkg.Trace.OriginatesFrom }}
    - {{ . }}
{{- end }}
{{- end }}
scope:
{{- if .Pkg.Scope.Architecture }}
  architecture: {{ .Pkg.Scope.Architecture }}
{{- end }}
  repositories:
{{- range .Pkg.Scope.Repositories }}
    - {{ . }}
{{- end }}
permissions:
  code: {{ .Pkg.Permissions.Code }}
  git: {{ .Pkg.Permissions.Git }}
</context_package>

{{- if .Pkg.RepoProfile }}
<repo_profiles>
{{ .Pkg.RepoProfile }}
</repo_profiles>
{{- end }}

{{- if .Pkg.ArchitectureProfile }}
<architecture_profile>
{{ .Pkg.ArchitectureProfile }}
</architecture_profile>
{{- end }}

{{- if .Pkg.Skills }}
<skills>
{{- range .Pkg.Skills }}
  - {{ . }}
{{- end }}
</skills>
{{- end }}
`

// FormatPromptSignature takes a base instruction and a structured Context Package,
// and produces the final plain-text prompt signature for the agent.
func FormatPromptSignature(baseInstruction string, pkg *context.Package) (string, error) {
	if pkg == nil {
		return "", fmt.Errorf("context package is nil")
	}

	tmpl, err := template.New("prompt").Parse(promptTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse prompt template: %w", err)
	}

	data := struct {
		BaseInstruction string
		Pkg             *context.Package
	}{
		BaseInstruction: baseInstruction,
		Pkg:             pkg,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute prompt template: %w", err)
	}

	return buf.String(), nil
}

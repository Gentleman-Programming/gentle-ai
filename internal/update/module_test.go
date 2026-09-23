package update

import "testing"

// TestGoInstallResolvable verifies that GoInstallResolvable reads ONLY the
// declared module path and import path (D-01) and never derives the module
// from Owner/Repo (REQ-22.2).
func TestGoInstallResolvable(t *testing.T) {
	const (
		upstreamModule = "github.com/gentleman-programming/gentle-ai/v3"
		upstreamImport = "github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai"
		forkImport     = "github.com/IGutierrezZ/axiom/cmd/axiom"
		// partialModule is the upstream module path WITHOUT the /v3 major
		// version suffix: a partial prefix that must not make the /v3 import
		// path resolvable (tasks.md 1.2, design.md section 6).
		partialModule = "github.com/gentleman-programming/gentle-ai"
	)

	tests := []struct {
		name         string
		goModulePath string
		goImportPath string
		want         bool
	}{
		{
			name:         "upstream pair intact is resolvable",
			goModulePath: upstreamModule,
			goImportPath: upstreamImport,
			want:         true,
		},
		{
			name:         "exact equality is resolvable",
			goModulePath: "github.com/example/tool",
			goImportPath: "github.com/example/tool",
			want:         true,
		},
		{
			name:         "v1 module resolves its own cmd package",
			goModulePath: partialModule,
			goImportPath: "github.com/gentleman-programming/gentle-ai/cmd/tool",
			want:         true,
		},
		{
			name:         "fork import path against upstream module is not resolvable",
			goModulePath: upstreamModule,
			goImportPath: forkImport,
			want:         false,
		},
		{
			name:         "empty module path is not resolvable",
			goModulePath: "",
			goImportPath: upstreamImport,
			want:         false,
		},
		{
			name:         "empty import path is not resolvable",
			goModulePath: upstreamModule,
			goImportPath: "",
			want:         false,
		},
		{
			name:         "both empty is not resolvable",
			goModulePath: "",
			goImportPath: "",
			want:         false,
		},
		{
			name:         "partial module prefix without v3 is not resolvable",
			goModulePath: partialModule,
			goImportPath: upstreamImport,
			want:         false,
		},
		{
			name:         "module path that is only a raw string prefix is not resolvable",
			goModulePath: partialModule,
			goImportPath: "github.com/gentleman-programming/gentle-ai-extra/cmd/tool",
			want:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := ToolInfo{
				// Owner/Repo are deliberately unrelated to the paths under test:
				// GoInstallResolvable must never derive the module from them (D-01).
				Owner:        "Somebody",
				Repo:         "somewhere",
				GoModulePath: tt.goModulePath,
				GoImportPath: tt.goImportPath,
			}
			if got := tool.GoInstallResolvable(); got != tt.want {
				t.Errorf("GoInstallResolvable() = %v, want %v (GoModulePath=%q GoImportPath=%q)",
					got, tt.want, tt.goModulePath, tt.goImportPath)
			}
		})
	}
}

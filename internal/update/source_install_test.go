package update

import (
	"strings"
	"testing"
)

// forkSourceTool mirrors the self-tool entry after cmd/axiom init() rewrites it
// (REQ-22.3): fork coordinates with the upstream module still declared in go.mod.
var forkSourceTool = ToolInfo{
	Name:         "axiom",
	Owner:        "IGutierrezZ",
	Repo:         "axiom",
	GoImportPath: "github.com/IGutierrezZ/axiom/cmd/axiom",
	GoModulePath: "github.com/gentleman-programming/gentle-ai/v3",
}

// upstreamSourceTool mirrors the shipped self-tool entry before the fork rename.
var upstreamSourceTool = ToolInfo{
	Name:         "gentle-ai",
	Owner:        "Gentleman-Programming",
	Repo:         "gentle-ai",
	GoImportPath: "github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai",
	GoModulePath: "github.com/gentleman-programming/gentle-ai/v3",
}

func TestSourceInstallCommand(t *testing.T) {
	tests := []struct {
		name    string
		tool    ToolInfo
		version string
		want    string
	}{
		{
			name:    "resolvable exact release emits go install at tag",
			tool:    upstreamSourceTool,
			version: "2.2.0",
			want:    "go install github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai@v2.2.0",
		},
		{
			name:    "resolvable already-prefixed version emits go install at tag",
			tool:    upstreamSourceTool,
			version: "v2.2.0",
			want:    "go install github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai@v2.2.0",
		},
		{
			name:    "resolvable beta main target emits go install at main",
			tool:    upstreamSourceTool,
			version: "main@972997650b51",
			want:    "go install github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai@main",
		},
		{
			name:    "resolvable empty version emits go install at latest",
			tool:    upstreamSourceTool,
			version: "",
			want:    "go install github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai@latest",
		},
		{
			name:    "fork module discrepancy emits clone and build at tag",
			tool:    forkSourceTool,
			version: "2.2.0",
			want:    "git clone --branch v2.2.0 https://github.com/IGutierrezZ/axiom && cd axiom && go build -o axiom ./cmd/axiom",
		},
		{
			name:    "fork beta main target emits clone and build at main",
			tool:    forkSourceTool,
			version: "main@972997650b51",
			want:    "git clone --branch main https://github.com/IGutierrezZ/axiom && cd axiom && go build -o axiom ./cmd/axiom",
		},
		{
			name:    "fork empty version emits clone and build of default branch",
			tool:    forkSourceTool,
			version: "",
			want:    "git clone https://github.com/IGutierrezZ/axiom && cd axiom && go build -o axiom ./cmd/axiom",
		},
		{
			name: "empty declared module emits clone and build",
			tool: ToolInfo{
				Name:         "axiom",
				Owner:        "IGutierrezZ",
				Repo:         "axiom",
				GoImportPath: "github.com/IGutierrezZ/axiom/cmd/axiom",
			},
			version: "2.2.0",
			want:    "git clone --branch v2.2.0 https://github.com/IGutierrezZ/axiom && cd axiom && go build -o axiom ./cmd/axiom",
		},
		{
			name: "empty import path emits clone and build",
			tool: ToolInfo{
				Name:         "axiom",
				Owner:        "IGutierrezZ",
				Repo:         "axiom",
				GoModulePath: "github.com/gentleman-programming/gentle-ai/v3",
			},
			version: "",
			want:    "git clone https://github.com/IGutierrezZ/axiom && cd axiom && go build -o axiom ./cmd/axiom",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SourceInstallCommand(tc.tool, tc.version)
			if got != tc.want {
				t.Fatalf("SourceInstallCommand(%q, %q) = %q, want %q", tc.tool.Name, tc.version, got, tc.want)
			}
			if tc.tool.GoInstallResolvable() {
				if !strings.Contains(got, "go install ") {
					t.Fatalf("resolvable instruction must contain a go install command: %q", got)
				}
				return
			}
			if strings.Contains(got, "go install") {
				t.Fatalf("non-resolvable instruction must contain zero go install commands: %q", got)
			}
			if !strings.Contains(got, "https://github.com/"+tc.tool.Owner+"/"+tc.tool.Repo) {
				t.Fatalf("clone instruction must name the tool repository: %q", got)
			}
			if !strings.Contains(got, "./cmd/"+tc.tool.Name) {
				t.Fatalf("build instruction must name ./cmd/<Name>: %q", got)
			}
		})
	}
}

// TestSourceInstallCommandForkNeverNamesUpstream covers REQ-22.2 scenarios
// "La pista de actualizacion nombra el fork, no upstream" and "Discrepancia de
// modulo detectada, sin `go install` abortado a mitad".
func TestSourceInstallCommandForkNeverNamesUpstream(t *testing.T) {
	if forkSourceTool.GoInstallResolvable() {
		t.Fatal("fork fixture must model a module discrepancy (GoInstallResolvable == false)")
	}

	for _, version := range []string{"2.2.0", "v2.2.0", "main@972997650b51", "main@", ""} {
		got := SourceInstallCommand(forkSourceTool, version)
		if got == "" {
			t.Fatalf("SourceInstallCommand(fork, %q) must not be empty", version)
		}
		for _, forbidden := range []string{"gentleman-programming/gentle-ai", "cmd/gentle-ai"} {
			if strings.Contains(got, forbidden) {
				t.Fatalf("SourceInstallCommand(fork, %q) = %q, must not contain %q", version, got, forbidden)
			}
		}
		if strings.Contains(got, "go install") {
			t.Fatalf("SourceInstallCommand(fork, %q) = %q, must emit zero go install commands", version, got)
		}
		if !strings.Contains(got, "IGutierrezZ/axiom") {
			t.Fatalf("SourceInstallCommand(fork, %q) = %q, must name the fork identity", version, got)
		}
	}
}

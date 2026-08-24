package agent

import "testing"

// TestDeriveArtifactType covers design decision D5's alias table: two
// canonical filenames are NOT a literal filename-minus-extension match
// (tasks.md -> task, explore.md -> exploration), so a naive
// strings.TrimSuffix(base, ".md") would silently produce the wrong string
// and every AllowedArtifactTypes comparison against it would always fail.
func TestDeriveArtifactType(t *testing.T) {
	tests := []struct {
		name         string
		artifactPath string
		want         string
	}{
		{"proposal filename derives proposal", "proposal.md", "proposal"},
		{"spec filename derives spec", "spec.md", "spec"},
		{"design filename derives design", "design.md", "design"},
		{"tasks filename derives singular task (not literal match)", "tasks.md", "task"},
		{"explore filename derives exploration (not literal match)", "explore.md", "exploration"},
		{"blueprint filename derives blueprint", "blueprint.md", "blueprint"},
		{"workspace-relative path uses only the base name", "openspec/changes/feature/spec.md", "spec"},
		{"non-canonical filename is unclassified, not rejected", "notes.md", ""},
		{"empty path is unclassified", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeriveArtifactType(tt.artifactPath); got != tt.want {
				t.Errorf("DeriveArtifactType(%q) = %q, want %q", tt.artifactPath, got, tt.want)
			}
		})
	}
}

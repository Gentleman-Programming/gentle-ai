package update

import "testing"

// TestModulePathForVersion pins the helper's contract for issue #4687:
// derive the Go module /vN suffix from the version being installed, not
// from the running binary's own major. The seam runningGoMajor is pinned to
// 3 (the running binary's major in this repository) so every unparseable
// version falls back to /v3.
func TestModulePathForVersion(t *testing.T) {
	origRunning := runningGoMajor
	t.Cleanup(func() { runningGoMajor = origRunning })
	runningGoMajor = func() int { return 3 }

	const base = "github.com/gentleman-programming/gentle-ai/cmd/gentle-ai"
	const repo = "gentle-ai"

	tests := []struct {
		name    string
		base    string
		repo    string
		version string
		want    string
	}{
		{
			name:    "v3.4.0 inserts /v3 before /cmd subpath",
			base:    base,
			repo:    repo,
			version: "v3.4.0",
			want:    "github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai",
		},
		{
			name:    "v2.0.0 inserts /v2 before /cmd subpath",
			base:    base,
			repo:    repo,
			version: "v2.0.0",
			want:    "github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai",
		},
		{
			name:    "v4.0.0-rc.1 inserts /v4 before /cmd subpath",
			base:    base,
			repo:    repo,
			version: "v4.0.0-rc.1",
			want:    "github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai",
		},
		{
			name:    "10.0.0 multi-digit inserts /v10 before /cmd subpath",
			base:    base,
			repo:    repo,
			version: "10.0.0",
			want:    "github.com/gentleman-programming/gentle-ai/v10/cmd/gentle-ai",
		},
		{
			name:    "v1.9.0 stays unsuffixed (major 1)",
			base:    base,
			repo:    repo,
			version: "v1.9.0",
			want:    base,
		},
		{
			name:    "v0 stays unsuffixed (major 0)",
			base:    base,
			repo:    repo,
			version: "v0",
			want:    base,
		},
		{
			name:    "unparseable 'latest' falls back to running major /v3",
			base:    base,
			repo:    repo,
			version: "latest",
			want:    "github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai",
		},
		{
			name:    "unparseable 'main@abc' falls back to running major /v3",
			base:    base,
			repo:    repo,
			version: "main@abc",
			want:    "github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai",
		},
		{
			name:    "empty version falls back to running major /v3",
			base:    base,
			repo:    repo,
			version: "",
			want:    "github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai",
		},
		{
			name:    "base without subpath appends /vN",
			base:    "github.com/gentleman-programming/gentle-ai",
			repo:    repo,
			version: "v3.0.1",
			want:    "github.com/gentleman-programming/gentle-ai/v3",
		},
		{
			name:    "leading v stripped when parsing",
			base:    base,
			repo:    repo,
			version: "V3.0.0",
			want:    "github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai",
		},
		{
			name:    "repo substring inside longer name is not matched as a segment",
			base:    "github.com/Gentleman-Programming/gentleman-guardian-angel/cmd/gga",
			repo:    "guardian-angel",
			version: "v2.0.0",
			want:    "github.com/Gentleman-Programming/gentleman-guardian-angel/cmd/gga/v2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ModulePathForVersion(tc.base, tc.repo, tc.version)
			if got != tc.want {
				t.Errorf("ModulePathForVersion(%q, %q, %q) = %q, want %q", tc.base, tc.repo, tc.version, got, tc.want)
			}
		})
	}
}

// TestParseMajorVersion covers the version-parsing edge cases independently
// of the suffix-insertion logic. Returns -1 for unparseable input.
func TestParseMajorVersion(t *testing.T) {
	tests := []struct {
		version string
		want    int
	}{
		{"3.4.0", 3},
		{"v3.4.0", 3},
		{"V3.4.0", 3},
		{"v4.0.0-rc.1", 4},
		{"10.0.0", 10},
		{"v1.9.0", 1},
		{"v0", 0},
		{"latest", -1},
		{"main@abc", -1},
		{"", -1},
		{"v", -1},
		{"vabc", -1},
		{"  v3.0.0  ", 3},
	}
	for _, tc := range tests {
		got := parseMajorVersion(tc.version)
		if got != tc.want {
			t.Errorf("parseMajorVersion(%q) = %d, want %d", tc.version, got, tc.want)
		}
	}
}

// TestParseMajorFromModulePath covers the running-binary's module-path
// parser. The repo's own path is .../gentle-ai/v3 → 3; an unsuffixed path
// returns 0.
func TestParseMajorFromModulePath(t *testing.T) {
	tests := []struct {
		path string
		want int
	}{
		{"github.com/gentleman-programming/gentle-ai/v3", 3},
		{"github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai", 3},
		{"github.com/gentleman-programming/gentle-ai/v10/cmd/x", 10},
		{"github.com/gentleman-programming/gentle-ai", 0},
		{"github.com/gentleman-programming/gentle-ai/cmd/x", 0},
		{"", 0},
		{"/v3", 3},
		{"/v", 0},
		{"/vN", 0},
	}
	for _, tc := range tests {
		got := parseMajorFromModulePath(tc.path)
		if got != tc.want {
			t.Errorf("parseMajorFromModulePath(%q) = %d, want %d", tc.path, got, tc.want)
		}
	}
}

package update

import "testing"

// TestGentleAISourceInstallCommandSameMajorStaysV3 pins the existing
// same-major stable behavior: when the running binary is v3 (the default
// seam), "latest" and v3.*.* compose a /v3 path. An unresolved beta commit
// cannot produce a safe source command without its validated module path.
func TestGentleAISourceInstallCommandSameMajorStaysV3(t *testing.T) {
	origRunning := runningGoMajor
	t.Cleanup(func() { runningGoMajor = origRunning })
	runningGoMajor = func() int { return 3 }

	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "empty version uses @latest with running major /v3", version: "", want: "go install github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai@latest"},
		{name: "unresolved beta commit has no safe source command", version: "main@972997650b51", want: ""},
		{name: "v3.4.0 uses @v3.4.0 with /v3", version: "v3.4.0", want: "go install github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai@v3.4.0"},
		{name: "3.4.0 (no v prefix) uses @v3.4.0 with /v3", version: "3.4.0", want: "go install github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai@v3.4.0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GentleAISourceInstallCommand(tc.version)
			if got != tc.want {
				t.Errorf("GentleAISourceInstallCommand(%q) = %q, want %q", tc.version, got, tc.want)
			}
		})
	}
}

// TestGentleAISourceInstallCommandCrossMajorDerivesFromVersion is the
// acceptance scenario for issue #4687: the /vN suffix tracks the version
// being installed, so a v4 release composes /v4 even though the running
// binary is v3. This is what makes a cross-major upgrade resolvable.
func TestGentleAISourceInstallCommandCrossMajorDerivesFromVersion(t *testing.T) {
	origRunning := runningGoMajor
	t.Cleanup(func() { runningGoMajor = origRunning })
	runningGoMajor = func() int { return 3 }

	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "v4.0.0 composes /v4 even when running major is /v3", version: "v4.0.0", want: "go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@v4.0.0"},
		{name: "v2.0.0 composes /v2 even when running major is /v3", version: "v2.0.0", want: "go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@v2.0.0"},
		{name: "v1.9.0 stays unsuffixed (major 1 has no /vN)", version: "v1.9.0", want: "go install github.com/gentleman-programming/gentle-ai/cmd/gentle-ai@v1.9.0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GentleAISourceInstallCommand(tc.version)
			if got != tc.want {
				t.Errorf("GentleAISourceInstallCommand(%q) = %q, want %q", tc.version, got, tc.want)
			}
		})
	}
}

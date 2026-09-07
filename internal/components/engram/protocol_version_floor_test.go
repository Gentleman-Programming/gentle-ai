package engram

import "testing"

// TestEngramVersionMeetsFloor pins JD-014's tightened version-parse policy:
// accepted token shapes are "X.Y.Z", "vX.Y.Z", "engram X.Y.Z", and
// "X.Y.Z-rc.N" (pre-release RC versions). Pre-release versions are now
// accepted and compared against the floor. Embedded semver inside arbitrary
// text ("foo 1.5.0 bar") does NOT meet the floor — it falls back to the safe
// default (full), because arbitrary text is not a trustworthy "engram
// version" output shape.
func TestEngramVersionMeetsFloor(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    bool
	}{
		// Pre-existing cases (adapted for floor 2.0.0).
		{"below floor", "1.3.9", false},
		{"unknown/unparseable version", "not-a-version", false},
		{"empty version", "", false},
		{"below old floor", "1.4.0", false},
		{"below new floor, bare X.Y.Z", "1.18.0", false},
		{"engram-prefixed below new floor", "engram 1.18.0", false},

		// New cases (JD-014).
		{"v-prefixed below old floor", "v1.4.0", false},
		{"v-prefixed below new floor", "v1.18.0", false},
		{"engram-prefixed below old floor", "engram 1.4.0", false},
		{"engram v-prefixed below new floor", "engram v1.18.0", false},
		{"pre-release at floor version does not meet floor", "1.4.0-beta", false},
		{"release-candidate at floor version does not meet floor", "1.4.0-rc1", false},
		{"release-candidate at floor version with dot does not meet floor", "1.4.0-rc.1", false},
		{"release-candidate at new floor version meets floor", "2.0.0-rc.8", true},
		{"release-candidate at new floor version with no dot meets floor", "2.0.0-rc8", true},
		{"release-candidate above new floor version meets floor", "2.1.0-rc.1", true},
		{"v-prefixed release-candidate at new floor meets floor", "v2.0.0-rc.8", true},
		{"engram-prefixed release-candidate at new floor meets floor", "engram 2.0.0-rc.8", true},
		{"embedded semver in arbitrary text does not meet floor", "foo 1.5.0 bar", false},
		{"trailing garbage after version does not meet floor", "1.18.0 (build 42)", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := engramVersionMeetsFloor(tt.version); got != tt.want {
				t.Fatalf("engramVersionMeetsFloor(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

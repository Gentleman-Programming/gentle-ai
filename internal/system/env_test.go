package system

import "testing"

func TestLookupEnv(t *testing.T) {
	const axiomKey = "AXIOM_TEST_VAR"
	const gentleKey = "GENTLE_AI_TEST_VAR"

	t.Run("axiom priority when both set", func(t *testing.T) {
		t.Setenv(axiomKey, "val_axiom")
		t.Setenv(gentleKey, "val_gentle")

		got, ok := LookupEnv(axiomKey, gentleKey)
		if !ok || got != "val_axiom" {
			t.Errorf("LookupEnv() = (%q, %v), want (%q, true)", got, ok, "val_axiom")
		}
	})

	t.Run("gentle fallback when axiom unset", func(t *testing.T) {
		t.Setenv(axiomKey, "")
		// Unset axiomKey explicitly in subtest
		t.Setenv(gentleKey, "val_gentle")

		// In Go testing, to truly unset:
		// We can test by unsetting or using a unique key
		const unsetAxiom = "AXIOM_UNSET_KEY"
		const setGentle = "GENTLE_AI_SET_KEY"
		t.Setenv(setGentle, "val_gentle")

		got, ok := LookupEnv(unsetAxiom, setGentle)
		if !ok || got != "val_gentle" {
			t.Errorf("LookupEnv() = (%q, %v), want (%q, true)", got, ok, "val_gentle")
		}
	})

	t.Run("returns false when both unset", func(t *testing.T) {
		const unsetAxiom = "AXIOM_UNSET_BOTH"
		const unsetGentle = "GENTLE_AI_UNSET_BOTH"

		got, ok := LookupEnv(unsetAxiom, unsetGentle)
		if ok || got != "" {
			t.Errorf("LookupEnv() = (%q, %v), want (%q, false)", got, ok, "")
		}
	})
}

func TestGetenv(t *testing.T) {
	const axiomKey = "AXIOM_GETENV_TEST"
	const gentleKey = "GENTLE_AI_GETENV_TEST"

	t.Run("axiom takes precedence", func(t *testing.T) {
		t.Setenv(axiomKey, "1")
		t.Setenv(gentleKey, "0")

		if got := Getenv(axiomKey, gentleKey); got != "1" {
			t.Errorf("Getenv() = %q, want %q", got, "1")
		}
	})

	t.Run("fallback to gentle when axiom empty", func(t *testing.T) {
		t.Setenv(axiomKey, "")
		t.Setenv(gentleKey, "0")

		if got := Getenv(axiomKey, gentleKey); got != "0" {
			t.Errorf("Getenv() = %q, want %q", got, "0")
		}
	})

	t.Run("returns empty when neither set", func(t *testing.T) {
		const k1 = "AXIOM_NONE_SET"
		const k2 = "GENTLE_AI_NONE_SET"

		if got := Getenv(k1, k2); got != "" {
			t.Errorf("Getenv() = %q, want %q", got, "")
		}
	})

	t.Run("automatic fallback for single AXIOM_ key", func(t *testing.T) {
		const unsetAxiom = "AXIOM_AUTO_FALLBACK_UNSET"
		const setGentle = "GENTLE_AI_AUTO_FALLBACK_UNSET"
		t.Setenv(setGentle, "fallback_value")

		if got := Getenv(unsetAxiom); got != "fallback_value" {
			t.Errorf("Getenv(%s) = %q, want %q", unsetAxiom, got, "fallback_value")
		}

		got, ok := LookupEnv(unsetAxiom)
		if !ok || got != "fallback_value" {
			t.Errorf("LookupEnv(%s) = (%q, %v), want (%q, true)", unsetAxiom, got, ok, "fallback_value")
		}
	})

	t.Run("canonical variables resolution", func(t *testing.T) {
		testCases := []struct {
			name        string
			axiomKey    string
			gentleKey   string
			axiomVal    string
			gentleVal   string
			wantBoth    string
			wantGentle  string
		}{
			{
				name:       "channel",
				axiomKey:   EnvChannelAxiom,
				gentleKey:  EnvChannelGentle,
				axiomVal:   "beta",
				gentleVal:  "stable",
				wantBoth:   "beta",
				wantGentle: "stable",
			},
			{
				name:       "opencode background subagents",
				axiomKey:   EnvOpenCodeBackgroundSubagentsAxiom,
				gentleKey:  EnvOpenCodeBackgroundSubagentsGentle,
				axiomVal:   "on",
				gentleVal:  "off",
				wantBoth:   "on",
				wantGentle: "off",
			},
			{
				name:       "state dir",
				axiomKey:   EnvStateDirAxiom,
				gentleKey:  EnvStateDirGentle,
				axiomVal:   "/opt/axiom",
				gentleVal:  "/opt/gentle-ai",
				wantBoth:   "/opt/axiom",
				wantGentle: "/opt/gentle-ai",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Setenv(tc.axiomKey, tc.axiomVal)
				t.Setenv(tc.gentleKey, tc.gentleVal)
				if got := Getenv(tc.axiomKey); got != tc.wantBoth {
					t.Errorf("Getenv(%s) = %q, want %q", tc.axiomKey, got, tc.wantBoth)
				}
				if got := Getenv(tc.axiomKey, tc.gentleKey); got != tc.wantBoth {
					t.Errorf("Getenv(%s, %s) = %q, want %q", tc.axiomKey, tc.gentleKey, got, tc.wantBoth)
				}

				t.Setenv(tc.axiomKey, "")
				if got := Getenv(tc.axiomKey); got != tc.wantGentle {
					t.Errorf("Getenv(%s) fallback = %q, want %q", tc.axiomKey, got, tc.wantGentle)
				}
				if got := Getenv(tc.axiomKey, tc.gentleKey); got != tc.wantGentle {
					t.Errorf("Getenv(%s, %s) fallback = %q, want %q", tc.axiomKey, tc.gentleKey, got, tc.wantGentle)
				}
			})
		}
	})
}


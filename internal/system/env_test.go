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
}

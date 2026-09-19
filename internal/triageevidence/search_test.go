package triageevidence

import "testing"

func TestKeywordsDeterministicAndBounded(t *testing.T) {
	title := "TUI Reset Review Store is offered outside a Git repository"
	area := "CLI (commands, flags)"

	got := Keywords(title, area, 5, 4)
	if len(got) > 5 {
		t.Fatalf("Keywords returned %d terms, cap is 5: %v", len(got), got)
	}
	want := []string{"reset", "review", "store", "offered", "outside"}
	if !equalStrings(got, want) {
		t.Errorf("Keywords = %v, want %v", got, want)
	}
	if first := Keywords(title, area, 5, 4); !equalStrings(first, got) {
		t.Error("Keywords not deterministic across calls")
	}
}

func TestKeywordsFiltersNoise(t *testing.T) {
	title := "`R3-001` 3.4.0 broken the-of error fix bug" // versions, ids, stopwords
	got := Keywords(title, "", 10, 4)
	if len(got) != 1 || got[0] != "broken" {
		t.Errorf("Keywords = %v, want [broken]", got)
	}
}

func TestKeywordsEmpty(t *testing.T) {
	if got := Keywords("", "", 5, 4); len(got) != 0 {
		t.Errorf("Keywords(empty) = %v, want none", got)
	}
	if got := Keywords("a title", "", 0, 4); got != nil {
		t.Errorf("Keywords with max 0 = %v, want nil", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

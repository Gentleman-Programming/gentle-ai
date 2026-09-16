package qastage_test

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/qastage"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

func TestVocabularyV1(t *testing.T) {
	vocab := qastage.VocabularyV1()
	if vocab.ID != qastage.VocabularyID {
		t.Errorf("expected ID %q, got %q", qastage.VocabularyID, vocab.ID)
	}
	if err := vocab.Validate(); err != nil {
		t.Fatalf("VocabularyV1 validation failed: %v", err)
	}
	expectedStages := []string{"explore", "spec", "apply", "verify", "docs"}
	if len(vocab.Stages) != len(expectedStages) {
		t.Fatalf("expected %d stages, got %d", len(expectedStages), len(vocab.Stages))
	}
	for i, expected := range expectedStages {
		if vocab.Stages[i].Label != expected {
			t.Errorf("stage %d: expected %q, got %q", i, expected, vocab.Stages[i].Label)
		}
	}
}

func TestLedgerChangeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"whitespace", "   ", ""},
		{"valid", "foo-bar", "qa--foo-bar"},
		{"with spaces", "foo bar", "qa--foo bar"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := qastage.LedgerChangeName(tc.input)
			if result != tc.expected {
				t.Errorf("LedgerChangeName(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
			if result != "" {
				if err := sddstatus.ValidateRuntimeText(result, 128); err != nil {
					t.Errorf("expected LedgerChangeName output %q to pass ValidateRuntimeText: %v", result, err)
				}
			}
		})
	}
}

package model

import "testing"

func TestClaudeModelAlias_String(t *testing.T) {
	tests := []struct {
		alias ClaudeModelAlias
		want  string
	}{
		{ClaudeModelOpus, "opus"},
		{ClaudeModelSonnet, "sonnet"},
		{ClaudeModelHaiku, "haiku"},
		{"custom-alias", "custom-alias"},
	}

	for _, tt := range tests {
		t.Run(string(tt.alias), func(t *testing.T) {
			if got := tt.alias.String(); got != tt.want {
				t.Errorf("ClaudeModelAlias.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClaudeModelAlias_Valid(t *testing.T) {
	tests := []struct {
		alias ClaudeModelAlias
		want  bool
	}{
		{ClaudeModelOpus, true},
		{ClaudeModelSonnet, true},
		{ClaudeModelHaiku, true},
		{"opus", true},
		{"sonnet", true},
		{"haiku", true},
		{"", false},
		{"invalid", false},
		{"gpt-4o", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.alias), func(t *testing.T) {
			if got := tt.alias.Valid(); got != tt.want {
				t.Errorf("ClaudeModelAlias.Valid() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestPresets(t *testing.T) {
	t.Run("Balanced", func(t *testing.T) {
		preset := ClaudeModelPresetBalanced()
		if preset["orchestrator"] != ClaudeModelOpus {
			t.Errorf("Balanced preset orchestrator = %q, want %q", preset["orchestrator"], ClaudeModelOpus)
		}
		if preset["sdd-apply"] != ClaudeModelSonnet {
			t.Errorf("Balanced preset sdd-apply = %q, want %q", preset["sdd-apply"], ClaudeModelSonnet)
		}
		if preset["sdd-archive"] != ClaudeModelHaiku {
			t.Errorf("Balanced preset sdd-archive = %q, want %q", preset["sdd-archive"], ClaudeModelHaiku)
		}
	})

	t.Run("Performance", func(t *testing.T) {
		preset := ClaudeModelPresetPerformance()
		if preset["sdd-verify"] != ClaudeModelOpus {
			t.Errorf("Performance preset sdd-verify = %q, want %q", preset["sdd-verify"], ClaudeModelOpus)
		}
	})

	t.Run("Economy", func(t *testing.T) {
		preset := ClaudeModelPresetEconomy()
		if preset["orchestrator"] != ClaudeModelSonnet {
			t.Errorf("Economy preset orchestrator = %q, want %q", preset["orchestrator"], ClaudeModelSonnet)
		}
	})

	t.Run("Diversity", func(t *testing.T) {
		preset := ClaudeModelPresetDiversity()
		if preset["jd-judge-a"] != ClaudeModelOpus {
			t.Errorf("Diversity preset jd-judge-a = %q, want %q", preset["jd-judge-a"], ClaudeModelOpus)
		}
		if preset["jd-judge-b"] != ClaudeModelHaiku {
			t.Errorf("Diversity preset jd-judge-b = %q, want %q", preset["jd-judge-b"], ClaudeModelHaiku)
		}
		if preset["jd-fix-agent"] != ClaudeModelSonnet {
			t.Errorf("Diversity preset jd-fix-agent = %q, want %q", preset["jd-fix-agent"], ClaudeModelSonnet)
		}
	})
}

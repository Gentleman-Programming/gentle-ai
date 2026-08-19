package db

import "testing"

func TestEvaluateImpact(t *testing.T) {
	router := New()

	tests := []struct {
		name     string
		content  string
		expected Impact
	}{
		{
			name: "Impact Simple",
			content: `---
db_impact: simple
---
# Tasks
- Add column`,
			expected: ImpactSimple,
		},
		{
			name: "Impact High Risk",
			content: `---
db_impact: high-risk
---
# Tasks`,
			expected: ImpactHighRisk,
		},
		{
			name: "Impact None",
			content: `---
db_impact: none
---
# Tasks`,
			expected: ImpactNone,
		},
		{
			name: "No Frontmatter",
			content: `# Tasks
Just doing some UI work`,
			expected: ImpactNone,
		},
		{
			name: "Missing DB Impact",
			content: `---
id: 123
---
# Tasks`,
			expected: ImpactNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := router.EvaluateImpact(tt.content)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

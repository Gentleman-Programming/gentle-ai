package filemerge

import (
	"reflect"
	"testing"
)

func TestScanSections(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantSections  []Section
		wantAnomalies []Anomaly
	}{
		{
			name:  "single well-formed block",
			input: "before\n<!-- gentle-ai:persona -->\nHello\nWorld\n<!-- /gentle-ai:persona -->\nafter\n",
			wantSections: []Section{
				{ID: "persona", Content: "Hello\nWorld", StartLine: 2, EndLine: 5, CharCount: 11, LineCount: 2},
			},
		},
		{
			name: "multiple distinct blocks in one file",
			input: "<!-- gentle-ai:persona -->\nP body\n<!-- /gentle-ai:persona -->\n\n" +
				"<!-- gentle-ai:engram-protocol -->\nE body\n<!-- /gentle-ai:engram-protocol -->\n\n" +
				"<!-- gentle-ai:strict-tdd-mode -->\nT body\n<!-- /gentle-ai:strict-tdd-mode -->\n",
			wantSections: []Section{
				{ID: "persona", Content: "P body", StartLine: 1, EndLine: 3, CharCount: 6, LineCount: 1},
				{ID: "engram-protocol", Content: "E body", StartLine: 5, EndLine: 7, CharCount: 6, LineCount: 1},
				{ID: "strict-tdd-mode", Content: "T body", StartLine: 9, EndLine: 11, CharCount: 6, LineCount: 1},
			},
		},
		{
			name:  "empty-content block",
			input: "<!-- gentle-ai:empty -->\n<!-- /gentle-ai:empty -->\n",
			wantSections: []Section{
				{ID: "empty", Content: "", StartLine: 1, EndLine: 2, CharCount: 0, LineCount: 0},
			},
		},
		{
			name:  "unclosed opener at EOF",
			input: "<!-- gentle-ai:orphan -->\nsome content\n",
			wantAnomalies: []Anomaly{
				{Kind: AnomalyOrphanOpen, ID: "orphan", Line: 1},
			},
		},
		{
			name:  "stray closer with no opener",
			input: "before\n<!-- /gentle-ai:stray -->\nafter\n",
			wantAnomalies: []Anomaly{
				{Kind: AnomalyOrphanClose, ID: "stray", Line: 2},
			},
		},
		{
			name:  "closer id mismatch with opener id",
			input: "<!-- gentle-ai:a -->\ncontent\n<!-- /gentle-ai:b -->\nafter\n",
			wantAnomalies: []Anomaly{
				{Kind: AnomalyMismatch, ID: "b", Line: 3},
			},
		},
		{
			name: "mismatch discards stale opener then resumes cleanly on next block",
			input: "<!-- gentle-ai:a -->\nstale\n<!-- /gentle-ai:b -->\n" +
				"<!-- gentle-ai:c -->\nclean\n<!-- /gentle-ai:c -->\n",
			wantSections: []Section{
				{ID: "c", Content: "clean", StartLine: 4, EndLine: 6, CharCount: 5, LineCount: 1},
			},
			wantAnomalies: []Anomaly{
				{Kind: AnomalyMismatch, ID: "b", Line: 3},
			},
		},
		{
			name:  "unknown novel id measured like any other",
			input: "<!-- gentle-ai:brand-new-block -->\nnovel content\n<!-- /gentle-ai:brand-new-block -->\n",
			wantSections: []Section{
				{ID: "brand-new-block", Content: "novel content", StartLine: 1, EndLine: 3, CharCount: 13, LineCount: 1},
			},
		},
		{
			name:  "markers with leading indentation and trailing CR",
			input: "  <!-- gentle-ai:indented -->\r\ncontent line\r\n  <!-- /gentle-ai:indented -->\r\n",
			wantSections: []Section{
				{ID: "indented", Content: "content line", StartLine: 1, EndLine: 3, CharCount: 12, LineCount: 1},
			},
		},
		{
			name:  "file with zero markers",
			input: "just some text\nno markers at all\n",
		},
		{
			name:  "consecutive opener while a block is already active",
			input: "<!-- gentle-ai:a -->\n<!-- gentle-ai:b -->\ninner content\n<!-- /gentle-ai:b -->\n",
			wantSections: []Section{
				{ID: "b", Content: "inner content", StartLine: 2, EndLine: 4, CharCount: 13, LineCount: 1},
			},
			wantAnomalies: []Anomaly{
				{Kind: AnomalyOrphanOpen, ID: "a", Line: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScanSections(tt.input)
			if !reflect.DeepEqual(got.Sections, tt.wantSections) {
				t.Fatalf("Sections mismatch:\ngot:  %+v\nwant: %+v", got.Sections, tt.wantSections)
			}
			if !reflect.DeepEqual(got.Anomalies, tt.wantAnomalies) {
				t.Fatalf("Anomalies mismatch:\ngot:  %+v\nwant: %+v", got.Anomalies, tt.wantAnomalies)
			}
		})
	}
}

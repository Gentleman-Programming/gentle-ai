package sddstatus

import (
	"errors"
	"testing"
)

func TestStageVocabularyValidate(t *testing.T) {
	for _, test := range []struct {
		name    string
		vocab   StageVocabulary
		wantErr string
	}{
		{
			name: "valid vocabulary",
			vocab: StageVocabulary{
				ID: "test-vocab-v1",
				Stages: []Stage{
					{Label: "explore"},
					{Label: "spec"},
				},
			},
			wantErr: "",
		},
		{
			name: "empty ID",
			vocab: StageVocabulary{
				ID: "",
				Stages: []Stage{
					{Label: "explore"},
					{Label: "spec"},
				},
			},
			wantErr: "vocabulary ID cannot be empty",
		},
		{
			name: "less than 2 stages",
			vocab: StageVocabulary{
				ID: "test-vocab-v1",
				Stages: []Stage{
					{Label: "explore"},
				},
			},
			wantErr: "vocabulary must define at least two stages",
		},
		{
			name: "duplicate labels",
			vocab: StageVocabulary{
				ID: "test-vocab-v1",
				Stages: []Stage{
					{Label: "explore"},
					{Label: "spec"},
					{Label: "explore"},
				},
			},
			wantErr: "duplicate stage label: explore",
		},
		{
			name: "empty stage label",
			vocab: StageVocabulary{
				ID: "test-vocab-v1",
				Stages: []Stage{
					{Label: "explore"},
					{Label: ""},
				},
			},
			wantErr: "stage label cannot be empty",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.vocab.Validate()
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil || err.Error() != test.wantErr {
					t.Fatalf("got error %v, want %q", err, test.wantErr)
				}
			}
		})
	}
}

func TestStageVocabularyPositionAndAt(t *testing.T) {
	vocab := StageVocabulary{
		ID: "test-vocab-v1",
		Stages: []Stage{
			{Label: "explore"},
			{Label: "spec"},
			{Label: "apply"},
		},
	}

	for _, test := range []struct {
		name      string
		label     string
		wantPos   int
		wantErr   error
	}{
		{"first stage", "explore", 0, nil},
		{"middle stage", "spec", 1, nil},
		{"last stage", "apply", 2, nil},
		{"unknown stage", "deploy", -1, ErrRuntimeStageUnknown},
	} {
		t.Run("Position/"+test.name, func(t *testing.T) {
			pos, err := vocab.Position(test.label)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Position(%q) error = %v, want %v", test.label, err, test.wantErr)
			}
			if pos != test.wantPos {
				t.Fatalf("Position(%q) = %d, want %d", test.label, pos, test.wantPos)
			}
		})
	}

	for _, test := range []struct {
		name      string
		position  int
		wantStage string
		wantErr   error
	}{
		{"first pos", 0, "explore", nil},
		{"last pos", 2, "apply", nil},
		{"negative pos", -1, "", ErrRuntimeStageUnknown},
		{"out of bounds pos", 3, "", ErrRuntimeStageUnknown},
	} {
		t.Run("At", func(t *testing.T) {
			stage, err := vocab.At(test.position)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("At(%d) error = %v, want %v", test.position, err, test.wantErr)
			}
			if stage.Label != test.wantStage {
				t.Fatalf("At(%d) = %q, want %q", test.position, stage.Label, test.wantStage)
			}
		})
	}
}

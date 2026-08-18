package changeowner

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantEngine Engine
		wantFound  bool
		wantErr    error
	}{
		{
			name:       "no marker at all",
			content:    "type: greenfield\ngates: required\n",
			wantEngine: "",
			wantFound:  false,
			wantErr:    nil,
		},
		{
			name:       "recognized gentle-orchestrator marker",
			content:    "type: greenfield\nengine: gentle-orchestrator\n",
			wantEngine: EngineGentle,
			wantFound:  true,
			wantErr:    nil,
		},
		{
			name:       "recognized dev-orchestrator marker",
			content:    "engine: dev-orchestrator\n",
			wantEngine: EngineDev,
			wantFound:  true,
			wantErr:    nil,
		},
		{
			name:       "unrecognized value refuses rather than guesses",
			content:    "engine: some-future-engine\n",
			wantEngine: "",
			wantFound:  true,
			wantErr:    ErrUnknownEngine,
		},
		{
			name:       "marker is case-insensitive and tolerates leading whitespace",
			content:    "  ENGINE:   dev-orchestrator  \n",
			wantEngine: EngineDev,
			wantFound:  true,
			wantErr:    nil,
		},
		{
			name:       "empty content",
			content:    "",
			wantEngine: "",
			wantFound:  false,
			wantErr:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, found, err := Parse(tt.content)
			if !errors.Is(err, tt.wantErr) && err != tt.wantErr {
				t.Fatalf("Parse() err = %v, want %v", err, tt.wantErr)
			}
			if found != tt.wantFound {
				t.Fatalf("Parse() found = %v, want %v", found, tt.wantFound)
			}
			if engine != tt.wantEngine {
				t.Fatalf("Parse() engine = %q, want %q", engine, tt.wantEngine)
			}
		})
	}
}

func TestResolveDefault(t *testing.T) {
	changeRoot := t.TempDir()

	engine, err := Resolve(changeRoot)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if engine != EngineGentle {
		t.Fatalf("Resolve() = %q, want %q (default)", engine, EngineGentle)
	}
}

func TestResolveDefaultWhenArtifactsHaveNoMarker(t *testing.T) {
	changeRoot := t.TempDir()
	writeFile(t, changeRoot, "proposal.md", "# Proposal\n\nSome content with no engine marker.\n")

	engine, err := Resolve(changeRoot)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if engine != EngineGentle {
		t.Fatalf("Resolve() = %q, want %q (default)", engine, EngineGentle)
	}
}

func TestResolveUnknownValueRefuses(t *testing.T) {
	changeRoot := t.TempDir()
	writeFile(t, changeRoot, "proposal.md", "engine: some-future-engine\n")

	_, err := Resolve(changeRoot)
	if !errors.Is(err, ErrUnknownEngine) {
		t.Fatalf("Resolve() error = %v, want ErrUnknownEngine", err)
	}
}

func TestResolveExploreOverProposalPrecedence(t *testing.T) {
	changeRoot := t.TempDir()
	writeFile(t, changeRoot, "explore.md", "engine: dev-orchestrator\n")
	writeFile(t, changeRoot, "proposal.md", "engine: gentle-orchestrator\n")

	engine, err := Resolve(changeRoot)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if engine != EngineDev {
		t.Fatalf("Resolve() = %q, want %q (explore.md must win over proposal.md)", engine, EngineDev)
	}
}

func TestResolveFallsBackToProposalWhenExploreHasNoMarker(t *testing.T) {
	changeRoot := t.TempDir()
	writeFile(t, changeRoot, "explore.md", "type: greenfield\n")
	writeFile(t, changeRoot, "proposal.md", "engine: dev-orchestrator\n")

	engine, err := Resolve(changeRoot)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if engine != EngineDev {
		t.Fatalf("Resolve() = %q, want %q (proposal.md marker used when explore.md has none)", engine, EngineDev)
	}
}

func TestAssertCanWriteNonExistentChangeRootIsWritableByAnyone(t *testing.T) {
	changeRoot := filepath.Join(t.TempDir(), "does-not-exist-yet")

	if err := AssertCanWrite(changeRoot, EngineDev); err != nil {
		t.Fatalf("AssertCanWrite() error = %v, want nil for non-existent changeRoot", err)
	}
	if err := AssertCanWrite(changeRoot, EngineGentle); err != nil {
		t.Fatalf("AssertCanWrite() error = %v, want nil for non-existent changeRoot", err)
	}
}

func TestAssertCanWriteSameEngineProceeds(t *testing.T) {
	changeRoot := t.TempDir()
	writeFile(t, changeRoot, "proposal.md", "engine: dev-orchestrator\n")

	if err := AssertCanWrite(changeRoot, EngineDev); err != nil {
		t.Fatalf("AssertCanWrite() error = %v, want nil for same-engine write", err)
	}
}

func TestAssertCanWriteForeignEngineRefuses(t *testing.T) {
	changeRoot := t.TempDir()
	writeFile(t, changeRoot, "proposal.md", "engine: gentle-orchestrator\n")

	err := AssertCanWrite(changeRoot, EngineDev)
	if !errors.Is(err, ErrForeignEngine) {
		t.Fatalf("AssertCanWrite() error = %v, want ErrForeignEngine", err)
	}
}

func TestAssertCanWriteUnmarkedDefaultsToGentleOwnership(t *testing.T) {
	changeRoot := t.TempDir()
	writeFile(t, changeRoot, "proposal.md", "# no marker here\n")

	if err := AssertCanWrite(changeRoot, EngineGentle); err != nil {
		t.Fatalf("AssertCanWrite() error = %v, want nil for gentle write to unmarked change", err)
	}
	if err := AssertCanWrite(changeRoot, EngineDev); !errors.Is(err, ErrForeignEngine) {
		t.Fatalf("AssertCanWrite() error = %v, want ErrForeignEngine for dev write to unmarked (gentle-owned by default) change", err)
	}
}

func TestStampIdempotency(t *testing.T) {
	tests := []struct {
		name        string
		frontmatter string
		want        Engine
	}{
		{
			name:        "no existing marker",
			frontmatter: "type: greenfield\ngates: required\n",
			want:        EngineDev,
		},
		{
			name:        "already stamped with same value",
			frontmatter: "type: greenfield\nengine: dev-orchestrator\n",
			want:        EngineDev,
		},
		{
			name:        "stamped with a different value gets corrected",
			frontmatter: "type: greenfield\nengine: gentle-orchestrator\n",
			want:        EngineDev,
		},
		{
			name:        "empty frontmatter",
			frontmatter: "",
			want:        EngineGentle,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := Stamp(tt.frontmatter, tt.want)
			second := Stamp(first, tt.want)
			if first != second {
				t.Fatalf("Stamp() not idempotent:\nfirst  = %q\nsecond = %q", first, second)
			}

			engine, found, err := Parse(first)
			if err != nil {
				t.Fatalf("Parse(stamped) error = %v", err)
			}
			if !found {
				t.Fatalf("Parse(stamped) found = false, want true")
			}
			if engine != tt.want {
				t.Fatalf("Parse(stamped) engine = %q, want %q", engine, tt.want)
			}
		})
	}
}

func TestStampDoesNotDuplicateMarkerLine(t *testing.T) {
	stamped := Stamp("type: greenfield\n", EngineDev)
	restamped := Stamp(stamped, EngineDev)

	count := 0
	for _, line := range splitLines(restamped) {
		if unknownEnginePattern.MatchString(line) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("Stamp() produced %d engine: lines, want exactly 1:\n%s", count, restamped)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", name, err)
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, r := range s {
		if r == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

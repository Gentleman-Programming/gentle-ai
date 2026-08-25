package screens

import (
	"encoding/json"
	"errors"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/pi"
	"strings"
	"testing"
)

func TestPiModelInspectionStateRowsScrollAndSafeError(t *testing.T) {
	state := NewPiModelInspectionState()
	if state.Status != PiModelInspectionLoading || state.Target != pi.ModelRoutingTargetProject {
		t.Fatal("state must start loading at project")
	}
	var inspection pi.ModelRoutingInspection
	payload := `{"targets":{"project":{"provenance":{"source":"project","status":"valid"},"assignments":{"a":{"model":"m","thinking":"high"},"b":{"inheritModel":true,"thinking":"low"},"c":{"model":"p","inheritThinking":true},"d":{}}},"global":{"provenance":{"source":"global","status":"valid"},"assignments":{"a":{"model":"g","thinking":"low"}}}},"agents":[{"name":"a","configurable":true},{"name":"b","configurable":true},{"name":"c","configurable":true},{"name":"d","configurable":true},{"name":"hidden","configurable":false}]}`
	if err := json.Unmarshal([]byte(payload), &inspection); err != nil {
		t.Fatal(err)
	}
	state.SetResult(inspection, nil)
	rows := state.Rows()
	if len(rows) != 4 || rows[0].Model != "m" || rows[1].Model != "inherited" || rows[1].Thinking != "low" || rows[2].Thinking != "inherited" || rows[3].Model != "unset" {
		t.Fatalf("rows = %#v", rows)
	}
	state.Cursor = 3
	state.Move(0, 15)
	if state.Scroll != 1 {
		t.Fatalf("bottom scroll = %d, want 1", state.Scroll)
	}
	if out := RenderPiModelInspection(state, 24); !strings.Contains(out, "a  model: m") || !strings.Contains(out, "b  model: inherited") || !strings.Contains(out, "c  model: p") || !strings.Contains(out, "d  model: unset") {
		t.Fatalf("resized render = %q", out)
	}
	state.SelectTarget(pi.ModelRoutingTargetGlobal)
	if state.Rows()[0].Model != "g" || state.Cursor != 0 || state.Scroll != 0 {
		t.Fatalf("global target = %#v", state)
	}
	if out := RenderPiModelInspection(state, 24); !strings.Contains(out, "source: global") || strings.Contains(out, "hidden") {
		t.Fatal("render lost provenance or filtered agent")
	}
	state.SetResult(pi.ModelRoutingInspection{}, errors.New("bad\x1b\nerror\t"))
	out := RenderPiModelInspection(state, 24)
	if strings.Contains(out, "\x1b") || strings.Contains(out, "\t") || !strings.Contains(out, "Details:") {
		t.Fatalf("unsafe error render: %q", out)
	}
}

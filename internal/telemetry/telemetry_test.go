package telemetry

import (
	"os"
	"strings"
	"testing"
)

func TestNoticeRuntimeDisclosure(t *testing.T) {
	const want = "Gentle AI sends anonymous usage metrics (version, OS, agents, counters) and may send anonymous runtime usage from supported Pi/OpenCode/Codex integrations (public model, effort, agent class, available token usage, timing, error categories); runtime usage is never stored locally; run gentle-ai telemetry disable to opt out."
	if NoticeLine != want {
		t.Fatalf("enrollment notice = %q, want %q", NoticeLine, want)
	}
	doc, err := os.ReadFile("../../docs/telemetry.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), "```text\n"+want+"\n```") {
		t.Fatal("documented enrollment notice must match NoticeLine")
	}
}

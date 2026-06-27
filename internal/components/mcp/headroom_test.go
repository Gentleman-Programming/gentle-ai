package mcp

import (
	"encoding/json"
	"testing"
)

func TestDefaultHeadroomServerJSON(t *testing.T) {
	content := DefaultHeadroomServerJSON()
	if len(content) == 0 {
		t.Fatal("DefaultHeadroomServerJSON() returned empty")
	}

	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("DefaultHeadroomServerJSON() invalid JSON: %v", err)
	}

	cmd, ok := parsed["command"].(string)
	if !ok || cmd != "headroom" {
		t.Fatalf("command = %#v, want %q", parsed["command"], "headroom")
	}

	args, ok := parsed["args"].([]any)
	if !ok || len(args) == 0 {
		t.Fatalf("args missing or empty")
	}
	if args[0] != "mcp" || args[1] != "serve" {
		t.Fatalf("args = %v, want [mcp serve]", args)
	}
}

func TestDefaultHeadroomOverlayJSON(t *testing.T) {
	content := DefaultHeadroomOverlayJSON()
	if len(content) == 0 {
		t.Fatal("DefaultHeadroomOverlayJSON() returned empty")
	}

	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("DefaultHeadroomOverlayJSON() invalid JSON: %v", err)
	}

	mcpServers, ok := parsed["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers key missing or not object")
	}

	headroom, ok := mcpServers["headroom"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers.headroom missing or not object")
	}

	if headroom["command"] != "headroom" {
		t.Fatalf("command = %#v, want %q", headroom["command"], "headroom")
	}
}

func TestOpenCodeHeadroomOverlayJSON(t *testing.T) {
	content := OpenCodeHeadroomOverlayJSON()
	if len(content) == 0 {
		t.Fatal("OpenCodeHeadroomOverlayJSON() returned empty")
	}

	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("OpenCodeHeadroomOverlayJSON() invalid JSON: %v", err)
	}

	mcp, ok := parsed["mcp"].(map[string]any)
	if !ok {
		t.Fatal("mcp key missing")
	}

	headroom, ok := mcp["headroom"].(map[string]any)
	if !ok {
		t.Fatal("mcp.headroom missing")
	}

	replace, ok := headroom["__replace__"].(map[string]any)
	if !ok {
		t.Fatal("mcp.headroom.__replace__ missing")
	}

	if replace["type"] != "local" {
		t.Fatalf("type = %#v, want %q", replace["type"], "local")
	}
	if replace["command"] != "headroom" {
		t.Fatalf("command = %#v, want %q", replace["command"], "headroom")
	}
	if replace["enabled"] != true {
		t.Fatalf("enabled = %#v, want true", replace["enabled"])
	}
}

func TestOpenClawHeadroomOverlayJSON(t *testing.T) {
	content := OpenClawHeadroomOverlayJSON()
	if len(content) == 0 {
		t.Fatal("OpenClawHeadroomOverlayJSON() returned empty")
	}

	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("OpenClawHeadroomOverlayJSON() invalid JSON: %v", err)
	}
	if _, ok := parsed["mcp"]; !ok {
		t.Fatal("mcp key missing")
	}
}

func TestVSCodeHeadroomOverlayJSON(t *testing.T) {
	content := VSCodeHeadroomOverlayJSON()
	if len(content) == 0 {
		t.Fatal("VSCodeHeadroomOverlayJSON() returned empty")
	}

	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("VSCodeHeadroomOverlayJSON() invalid JSON: %v", err)
	}

	servers, ok := parsed["servers"].(map[string]any)
	if !ok {
		t.Fatal("servers key missing")
	}

	headroom, ok := servers["headroom"].(map[string]any)
	if !ok {
		t.Fatal("servers.headroom missing")
	}
	if headroom["command"] != "headroom" {
		t.Fatalf("servers.headroom.command = %#v, want %q", headroom["command"], "headroom")
	}
}

func TestAntigravityHeadroomOverlayJSON(t *testing.T) {
	content := AntigravityHeadroomOverlayJSON()
	if len(content) == 0 {
		t.Fatal("AntigravityHeadroomOverlayJSON() returned empty")
	}

	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("AntigravityHeadroomOverlayJSON() invalid JSON: %v", err)
	}

	mcpServers, ok := parsed["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers key missing")
	}

	headroom, ok := mcpServers["headroom"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers.headroom missing")
	}
	if _, ok := headroom["__replace__"]; !ok {
		t.Fatal("mcpServers.headroom.__replace__ missing")
	}
}

func TestKimiHeadroomOverlayJSON(t *testing.T) {
	content := KimiHeadroomOverlayJSON()
	if len(content) == 0 {
		t.Fatal("KimiHeadroomOverlayJSON() returned empty")
	}

	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("KimiHeadroomOverlayJSON() invalid JSON: %v", err)
	}

	mcpServers, ok := parsed["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers key missing")
	}

	headroom, ok := mcpServers["headroom"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers.headroom missing")
	}
	if _, ok := headroom["__replace__"]; !ok {
		t.Fatal("mcpServers.headroom.__replace__ missing")
	}
}

func TestHeadroomServerJSONGetReturnsCopy(t *testing.T) {
	orig := DefaultHeadroomServerJSON()
	second := DefaultHeadroomServerJSON()

	if len(orig) == 0 {
		t.Fatal("original is empty")
	}

	// Modify the first copy — verify second copy is unmodified.
	orig[0] = ' '
	if string(second[0]) != "{" {
		t.Fatal("DefaultHeadroomServerJSON() did not return a copy; mutation affected second call")
	}
}

func TestHeadroomOverlayJSONGetReturnsCopy(t *testing.T) {
	orig := DefaultHeadroomOverlayJSON()
	second := DefaultHeadroomOverlayJSON()

	if len(orig) == 0 {
		t.Fatal("original is empty")
	}

	orig[0] = ' '
	if string(second[0]) != "{" {
		t.Fatal("DefaultHeadroomOverlayJSON() did not return a copy")
	}
}

package mcp

import (
	"bytes"
	"testing"
)

func TestDefaultContext7ServerJSONIncludesCommand(t *testing.T) {
	content := DefaultContext7ServerJSON()
	if !bytes.Contains(content, []byte("@upstash/context7-mcp")) {
		t.Fatalf("DefaultContext7ServerJSON() does not include context7 package")
	}
}

func TestDefaultContext7OverlayJSONIncludesServerName(t *testing.T) {
	content := DefaultContext7OverlayJSON()
	if !bytes.Contains(content, []byte(`"context7"`)) {
		t.Fatalf("DefaultContext7OverlayJSON() does not include context7 server")
	}
}

func TestVSCodeContext7OverlayUsesServersKey(t *testing.T) {
	content := VSCodeContext7OverlayJSON()
	if !bytes.Contains(content, []byte(`"servers"`)) {
		t.Fatalf("VSCodeContext7OverlayJSON() should include servers key")
	}
	if bytes.Contains(content, []byte(`"mcpServers"`)) {
		t.Fatalf("VSCodeContext7OverlayJSON() should not include mcpServers key")
	}
}

func TestCopilotCLIMCPConfigJSONUsesMCPServersKey(t *testing.T) {
	content := CopilotCLIMCPConfigJSON()
	if !bytes.Contains(content, []byte(`"mcpServers"`)) {
		t.Fatalf("CopilotCLIMCPConfigJSON() should include mcpServers key")
	}
	if !bytes.Contains(content, []byte(`"context7"`)) {
		t.Fatalf("CopilotCLIMCPConfigJSON() should include context7 server name")
	}
}

func TestCopilotCLIMCPConfigJSONUsesStdioTransport(t *testing.T) {
	content := CopilotCLIMCPConfigJSON()
	if !bytes.Contains(content, []byte(`"command": "npx"`)) {
		t.Fatalf("CopilotCLIMCPConfigJSON() should use npx command for stdio transport")
	}
	if !bytes.Contains(content, []byte(`@upstash/context7-mcp@`)) {
		t.Fatalf("CopilotCLIMCPConfigJSON() should include context7 package with version")
	}
}

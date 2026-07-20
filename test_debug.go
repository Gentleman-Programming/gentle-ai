package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	
	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/components/communitytool"
)

func main() {
	fmt.Printf("GOOS: %s\n", runtime.GOOS)
	
	home, _ := os.MkdirTemp("", "test")
	defer os.RemoveAll(home)
	
	xdg := filepath.Join(home, "custom-config")
	os.MkdirAll(filepath.Join(xdg, "opencode"), 0755)
	os.WriteFile(filepath.Join(xdg, "opencode", "opencode.json"), []byte(`{}`), 0644)
	
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "custom-config"))
	
	// Test NeedsOpenCodeCodeGraphReconcile
	// We need to call the function directly
	// Since it's not exported, we'll test the lower level
	
	reg, _ := agents.NewDefaultRegistry()
	adapter, _ := reg.Get("opencode")
	
	home := os.Getenv("HOME")
	home = filepath.Dir(os.Getenv("HOME")) + "/test_temp"
	os.MkdirAll(filepath.Join(home, "custom-config", "opencode"), 0755)
	os.WriteFile(filepath.Join(home, "custom-config", "opencode", "opencode.json"), []byte(`{}`), 0644)
	
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "custom-config"))
	
	adapter, _ = agents.NewDefaultRegistry()
	_ = adapter
}

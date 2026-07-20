package communitytool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	
	"github.com/gentleman-programming/gentle-ai/internal/agents"
)

func DebugTest() {
	fmt.Printf("GOOS: %s\n", runtime.GOOS)
	
	home, _ := os.MkdirTemp("", "test")
	defer os.RemoveAll(home)
	
	xdg := filepath.Join(home, "custom-config")
	os.MkdirAll(filepath.Join(xdg, "opencode"), 0755)
	os.WriteFile(filepath.Join(xdg, "opencode", "opencode.json"), []byte(`{}`), 0644)
	
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "custom-config"))
	
	// Test NeedsOpenCodeCodeGraphReconcile
	fmt.Printf("runtime.GOOS = %s\n", runtime.GOOS)
	fmt.Printf("isAndroid() = %v\n", isAndroid())
	
	reg, _ := agents.NewDefaultRegistry()
	adapter, ok := reg.Get("opencode")
	if !ok {
		fmt.Println("OpenCode adapter not found")
		return
	}
	
	installed, _, configPath, configFound, err := adapter.Detect(context.Background(), home)
	fmt.Printf("OpenCode installed: %v, configPath: %s, configFound: %v, err: %v\n", 
		installed, configPath, configFound, err)
	
	// Check if OpenCode is detected in DiscoverInstalled
	installedAgents := agents.DiscoverInstalled(reg, home)
	for _, agent := range installedAgents {
		fmt.Printf("Installed agent: %s, configDir: %s\n", agent.ID, agent.ConfigDir)
	}
}

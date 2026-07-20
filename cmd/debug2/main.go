package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	
	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/components/communitytool"
)

func main() {
	home, _ := os.MkdirTemp("", "test")
	defer os.RemoveAll(home)
	
	xdg := filepath.Join(home, "custom-config")
	os.MkdirAll(filepath.Join(xdg, "opencode"), 0755)
	os.WriteFile(filepath.Join(xdg, "opencode", "opencode.json"), []byte(`{}`), 0644)
	
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "custom-config"))
	
	fmt.Printf("GOOS: %s\n", runtime.GOOS)
	fmt.Printf("HOME: %s\n", home)
	fmt.Printf("XDG_CONFIG_HOME: %s\n", os.Getenv("XDG_CONFIG_HOME"))
	
	reg, _ := agents.NewDefaultRegistry()
	installed := agents.DiscoverInstalled(reg, home)
	fmt.Printf("Discovered agents: %d\n", len(installed))
	for _, a := range installed {
		fmt.Printf("  Agent: %s, ConfigDir: %s\n", a.ID, a.ConfigDir)
	}
	
	reg2, _ := agents.NewDefaultRegistry()
	adapter, ok := reg2.Get(agents.AgentOpenCode)
	if ok {
		fmt.Printf("OpenCode adapter found\n")
		installed, _, configPath, configFound, err := adapter.Detect(context.Background(), home)
		fmt.Printf("OpenCode: installed=%v, configPath=%s, configFound=%v, err=%v\n", 
			installed, configPath, configFound, err)
		
		dir := adapter.GlobalConfigDir(home)
		fmt.Printf("GlobalConfigDir: %s\n", dir)
		
		info, err := os.Stat(dir)
		if err != nil {
			fmt.Printf("Stat error: %v\n", err)
		} else {
			fmt.Printf("IsDir: %v\n", info.IsDir())
		}
	}
	
	result := communitytool.NeedsOpenCodeCodeGraphReconcile(home)
	fmt.Printf("NeedsOpenCodeCodeGraphReconcile: %v\n", result)
}

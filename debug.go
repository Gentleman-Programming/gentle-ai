package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	
	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/agents/opencode"
)

func main() {
	home, _ := os.MkdirTemp("", "test")
	defer os.RemoveAll(home)
	
	xdg := filepath.Join(home, "custom-config")
	os.MkdirAll(filepath.Join(xdg, "opencode"), 0755)
	os.WriteFile(filepath.Join(xdg, "opencode", "opencode.json"), []byte(`{}`), 0644)
	
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", xdg)
	
	_ , err := agents.NewDefaultRegistry()
	if err != nil {
		fmt.Printf("Registry error: %v\n", err)
		return
	}
	
	adapter := opencode.NewAdapter()
	
	installed, binaryPath, configPath, configFound, err := adapter.Detect(context.Background(), home)
	fmt.Printf("installed: %v, binaryPath: %s, configPath: %s, configFound: %v, err: %v\n", 
		installed, binaryPath, configPath, configFound, err)
	
	dir := adapter.GlobalConfigDir(home)
	fmt.Printf("GlobalConfigDir: %s\n", dir)
	
	info, err := os.Stat(dir)
	if err != nil {
		fmt.Printf("Stat error: %v\n", err)
	} else {
		fmt.Printf("IsDir: %v\n", info.IsDir())
	}
}

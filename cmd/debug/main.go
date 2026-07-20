package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	
	"github.com/gentleman-programming/gentle-ai/internal/agents/opencode"
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
	
	fmt.Printf("GOOS: %s\n", runtime.GOOS)
	fmt.Printf("isAndroid: %v\n", runtime.GOOS == "android")
	
	// Use opencode adapter directly
	adapter := opencode.NewAdapter()
	
	installed, _, configPath, configFound, err := adapter.Detect(context.Background(), home)
	fmt.Printf("installed: %v, configPath: %s, configFound: %v, err: %v\n", 
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

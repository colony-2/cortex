package main

import (
	"encoding/json"
	"fmt"
	"os"
	
	"github.com/divisive-ai/vibethis/server/container/internal/devcontainer"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: test-mount <devcontainer.json>")
		os.Exit(1)
	}
	
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	
	fmt.Println("Raw JSON mounts field:")
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)
	if mounts, ok := raw["mounts"]; ok {
		mountsJSON, _ := json.MarshalIndent(mounts, "", "  ")
		fmt.Printf("%s\n\n", mountsJSON)
	}
	
	var dc devcontainer.DevContainer
	if err := json.Unmarshal(data, &dc); err != nil {
		fmt.Printf("Failed to unmarshal: %v\n", err)
		return
	}
	
	fmt.Printf("Mounts parsed: %d\n", len(dc.Mounts))
	for i, mount := range dc.Mounts {
		fmt.Printf("Mount %d:\n", i)
		switch m := mount.(type) {
		case string:
			fmt.Printf("  String mount: %s\n", m)
		case map[string]interface{}:
			fmt.Printf("  Object mount:\n")
			if t, ok := m["type"]; ok {
				fmt.Printf("    Type: %v\n", t)
			}
			if s, ok := m["source"]; ok {
				fmt.Printf("    Source: %v\n", s)
			}
			if t, ok := m["target"]; ok {
				fmt.Printf("    Target: %v\n", t)
			}
			if r, ok := m["readonly"]; ok {
				fmt.Printf("    ReadOnly: %v\n", r)
			}
		default:
			fmt.Printf("  Unknown mount type: %T\n", mount)
		}
	}
}
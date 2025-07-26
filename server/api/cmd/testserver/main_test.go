package main

import (
	"os"
	"testing"
)

func TestExecute(t *testing.T) {
	// Test that the version command works
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	
	os.Args = []string{"testserver", "--version"}
	err := Execute()
	if err != nil {
		t.Errorf("Execute() with --version failed: %v", err)
	}
}
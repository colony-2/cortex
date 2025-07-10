package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRootCommand(t *testing.T) {
	assert.NotNil(t, rootCmd)
	assert.Equal(t, "ono", rootCmd.Use)
	assert.NotEmpty(t, rootCmd.Short)
	
	hasStartCmd := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "start" {
			hasStartCmd = true
			break
		}
	}
	assert.True(t, hasStartCmd, "Root command should have 'start' subcommand")
}
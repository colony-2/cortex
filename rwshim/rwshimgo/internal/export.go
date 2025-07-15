package internal

import "os/exec"

// GetCmd returns the underlying cmd for testing
func (p *ProcessImpl) GetCmd() *exec.Cmd {
	return p.cmd
}

// GetShimPath returns the shim path for testing
func (p *ProcessImpl) GetShimPath() string {
	return p.shimPath
}
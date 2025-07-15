package rwshim

import "os/exec"

// Export internal fields for testing

// GetCmd returns the underlying exec.Cmd for testing purposes
func (p *Process) GetCmd() *exec.Cmd {
	return p.impl.GetCmd()
}

// GetShimPath returns the shim library path for testing purposes
func (p *Process) GetShimPath() string {
	return p.impl.GetShimPath()
}
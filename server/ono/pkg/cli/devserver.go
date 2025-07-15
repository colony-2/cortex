package cli

import (
	"github.com/vibethis/embeddedtemporal"
)

type DevServerOptions = embeddedtemporal.Options

type DevServer = embeddedtemporal.Server

func NewDevServer(opts DevServerOptions) (*DevServer, error) {
	return embeddedtemporal.NewServer(opts)
}

// Helper functions that remain in ono

func isPortAvailable(host string, port int) bool {
	return embeddedtemporal.IsPortAvailable(host, port)
}

func findFreePort() int {
	return embeddedtemporal.FindFreePort()
}

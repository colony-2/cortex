package embeddedtemporal

import (
	"fmt"
	"net"
	"time"
)

// IsPortAvailable checks if a port is available for binding
func IsPortAvailable(host string, port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// FindFreePort finds a free port for service communication
func FindFreePort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		// Fallback to a high port if we can't find a free one
		return 7000 + int(time.Now().UnixNano()%1000)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}
package lib

import (
	"net"
	"strings"
)

// determineNetwork returns the network type and actual address based on the input
func DetermineNetwork(addr string) (network, address string) {
	if strings.HasPrefix(addr, "unix:") {
		return "unix", strings.TrimPrefix(addr, "unix:")
	}
	return "tcp", addr
}

func CreateListener(addr string) (net.Listener, error) {
	network, address := DetermineNetwork(addr)

	// If it's a unix socket, try to clean up any existing socket file
	if network == "unix" {
		cleanupUnixSocket(address)
	}

	return net.Listen(network, address)
}

func dialTarget(addr string) (net.Conn, error) {
	network, address := DetermineNetwork(addr)
	return net.Dial(network, address)
}

func cleanupUnixSocket(path string) error {
	return nil // Placeholder for actual cleanup if needed
}

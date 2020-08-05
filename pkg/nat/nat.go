package nat

import (
	"fmt"
	"net"
	"runtime"

	"github.com/rs/zerolog"
)

type NAT interface {
	Setup() error
	GetNATDestination(conn *net.TCPConn) (string, *net.TCPConn, error)
	Shutdown() error
	RedirectSubnets(subnets []string, excludes []string) error
	Cleanup() error
}

var StateNotFoundError = fmt.Errorf("nat state is not found")

func New(logger zerolog.Logger, proxyPort int) (NAT, error) {
	switch runtime.GOOS {
	case "darwin":
		return NewPF(logger, proxyPort), nil
	case "linux":
		return NewIptables(logger, proxyPort), nil
	}

	return nil, fmt.Errorf("%s is not supported", runtime.GOOS)
}

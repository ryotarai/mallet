package nat

import (
	"fmt"
	"github.com/rs/zerolog"
	"net"
	"runtime"
)

type NAT interface {
	Setup(proxyPort int, subnets []string) error
	GetNATDestination(conn *net.TCPConn) (string, *net.TCPConn, error)
	Shutdown() error
}

var StateNotFoundError = fmt.Errorf("nat state is not found")

func New(logger zerolog.Logger) (NAT, error) {
	switch runtime.GOOS {
	case "darwin":
		return NewPF(logger), nil
	case "linux":
		return NewIptables(logger), nil
	}

	return nil, fmt.Errorf("%s is not supported", runtime.GOOS)
}

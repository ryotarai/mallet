package nat

import (
	"fmt"
	"github.com/rs/zerolog"
	"github.com/ryotarai/tagane/pkg/priv"
	"net"
	"runtime"
)

type NAT interface {
	Setup() error
	GetNATDestination(conn *net.TCPConn) (string, *net.TCPConn, error)
	Shutdown() error
	RedirectSubnets(subnets []string) error
	Cleanup() error
}

var StateNotFoundError = fmt.Errorf("nat state is not found")

func New(logger zerolog.Logger, privClient *priv.Client, proxyPort int) (NAT, error) {
	switch runtime.GOOS {
	case "darwin":
		return NewPF(logger, privClient, proxyPort), nil
	case "linux":
		return NewIptables(logger, privClient, proxyPort), nil
	}

	return nil, fmt.Errorf("%s is not supported", runtime.GOOS)
}

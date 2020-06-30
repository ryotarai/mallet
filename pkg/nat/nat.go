package nat

import (
	"fmt"
	"github.com/rs/zerolog"
	"github.com/ryotarai/tagane/pkg/priv"
	"net"
	"runtime"
)

type NAT interface {
	Setup(proxyPort int, subnets []string) error
	GetNATDestination(conn *net.TCPConn) (string, *net.TCPConn, error)
	Shutdown() error
	Cleanup() error
}

var StateNotFoundError = fmt.Errorf("nat state is not found")

func New(logger zerolog.Logger, privClient *priv.Client) (NAT, error) {
	switch runtime.GOOS {
	case "darwin":
		return NewPF(logger, privClient), nil
	case "linux":
		return NewIptables(logger, privClient), nil
	}

	return nil, fmt.Errorf("%s is not supported", runtime.GOOS)
}

package nat

import (
	"fmt"
	"github.com/rs/zerolog"
	"github.com/ryotarai/tagane/pkg/priv"
	"net"
	"os"
	"strconv"
	"syscall"
)

const (
	SO_ORIGINAL_DST = 80
)

type Iptables struct {
	logger     zerolog.Logger
	privClient *priv.Client
	proxyPort  int
}

func NewIptables(logger zerolog.Logger, privClient *priv.Client, proxyPort int) *Iptables {
	return &Iptables{
		logger:     logger,
		privClient: privClient,
		proxyPort:  proxyPort,
	}
}

func (p *Iptables) Setup() error {
	chain := p.chainName()

	if err := p.iptables([]string{"-t", "nat", "-N", chain}); err != nil {
		return fmt.Errorf("failed to create a chain: %w", err)
	}

	if err := p.iptables([]string{"-t", "nat", "-F", chain}); err != nil {
		return fmt.Errorf("failed to flush a chain: %w", err)
	}

	if err := p.iptables([]string{"-t", "nat", "-I", "OUTPUT", "1", "-j", chain}); err != nil {
		return fmt.Errorf("failed to insert a jump rule to OUTPUT chain: %w", err)
	}

	if err := p.iptables([]string{"-t", "nat", "-I", "PREROUTING", "1", "-j", chain}); err != nil {
		return fmt.Errorf("failed to insert a jump rule to OUTPUT chain: %w", err)
	}

	if err := p.iptables([]string{"-t", "nat", "-A", chain, "-j", "RETURN", "-m", "addrtype", "--dst-type", "LOCAL"}); err != nil {
		return fmt.Errorf("failed to add a rule to return dst==local: %w", err)
	}

	return nil
}

func (p *Iptables) RedirectSubnets(subnets []string) error {
	chain := p.chainName()

	for _, subnet := range subnets {
		if err := p.iptables([]string{"-t", "nat", "-A", chain, "-j", "REDIRECT", "--dest", subnet, "-p", "tcp", "--to-ports", strconv.Itoa(p.proxyPort)}); err != nil {
			return fmt.Errorf("failed to redirect rule for %s: %w", subnet, err)
		}
	}

	return nil
}

func (p *Iptables) Shutdown() error {
	chain := p.chainName()

	if err := p.iptables([]string{"-t", "nat", "-D", "OUTPUT", "-j", chain}); err != nil {
		return fmt.Errorf("failed to delete a jump rule to OUTPUT chain: %w", err)
	}

	if err := p.iptables([]string{"-t", "nat", "-D", "PREROUTING", "-j", chain}); err != nil {
		return fmt.Errorf("failed to delete a jump rule to OUTPUT chain: %w", err)
	}

	if err := p.iptables([]string{"-t", "nat", "-F", chain}); err != nil {
		return fmt.Errorf("failed to flush a chain: %w", err)
	}

	if err := p.iptables([]string{"-t", "nat", "-X", chain}); err != nil {
		return fmt.Errorf("failed to delete a chain: %w", err)
	}

	return nil
}

func (p *Iptables) GetNATDestination(conn *net.TCPConn) (string, *net.TCPConn, error) {
	// https://gist.github.com/cannium/55ec625516a24da8f547aa2d93f49ecf
	f, err := conn.File()
	if err != nil {
		return "", nil, err
	}
	defer f.Close()

	addr, err := syscall.GetsockoptIPv6Mreq(int(f.Fd()), syscall.IPPROTO_IP, SO_ORIGINAL_DST)
	if err != nil {
		return "", nil, err
	}

	newConn, err := net.FileConn(f)
	if err != nil {
		return "", nil, err
	}

	newTCPConn, ok := newConn.(*net.TCPConn)
	if !ok {
		panic("BUG: not TCPConn")
	}

	dest := fmt.Sprintf("%d.%d.%d.%d:%d",
		addr.Multiaddr[4],
		addr.Multiaddr[5],
		addr.Multiaddr[6],
		addr.Multiaddr[7],
		uint16(addr.Multiaddr[2])<<8+uint16(addr.Multiaddr[3]))

	return dest, newTCPConn, nil
}

func (p *Iptables) Cleanup() error {
	return fmt.Errorf("not implemented")
}

func (p *Iptables) chainName() string {
	return fmt.Sprintf("tagane-pid%d", os.Getpid())
}

func (p *Iptables) iptables(args []string) error {
	resp, err := p.privClient.Command(&priv.CommandRequest{
		Command: "iptables",
		Args:    args,
	})
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return fmt.Errorf("iptables exit status: %d", resp.ExitCode)
	}

	return nil
}

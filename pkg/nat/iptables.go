package nat

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"syscall"

	"github.com/mitchellh/go-ps"
	"github.com/rs/zerolog"
)

const (
	SO_ORIGINAL_DST = 80
)

type Iptables struct {
	logger    zerolog.Logger
	proxyPort int
	subnets   []string
}

func NewIptables(logger zerolog.Logger, proxyPort int) *Iptables {
	return &Iptables{
		logger:    logger,
		proxyPort: proxyPort,
	}
}

func (p *Iptables) Setup() error {
	chain := p.chainName()

	if _, err := p.iptables([]string{"-t", "nat", "-N", chain}); err != nil {
		return fmt.Errorf("failed to create a chain: %w", err)
	}

	if _, err := p.iptables([]string{"-t", "nat", "-F", chain}); err != nil {
		return fmt.Errorf("failed to flush a chain: %w", err)
	}

	if _, err := p.iptables([]string{"-t", "nat", "-I", "OUTPUT", "1", "-j", chain}); err != nil {
		return fmt.Errorf("failed to insert a jump rule to OUTPUT chain: %w", err)
	}

	if _, err := p.iptables([]string{"-t", "nat", "-I", "PREROUTING", "1", "-j", chain}); err != nil {
		return fmt.Errorf("failed to insert a jump rule to OUTPUT chain: %w", err)
	}

	if _, err := p.iptables([]string{"-t", "nat", "-A", chain, "-j", "RETURN", "-m", "addrtype", "--dst-type", "LOCAL"}); err != nil {
		return fmt.Errorf("failed to add a rule to return dst==local: %w", err)
	}

	return nil
}

func (p *Iptables) RedirectSubnets(subnets []string) error {
	chain := p.chainName()

	currentSubnets := map[string]struct{}{}
	for _, subnet := range p.subnets {
		currentSubnets[subnet] = struct{}{}
	}

	newSubnets := map[string]struct{}{}
	for _, subnet := range subnets {
		newSubnets[subnet] = struct{}{}
	}

	// add
	for subnet := range newSubnets {
		if _, found := currentSubnets[subnet]; !found {
			if _, err := p.iptables([]string{"-t", "nat", "-A", chain, "-j", "REDIRECT", "--dest", subnet, "-p", "tcp", "--to-ports", strconv.Itoa(p.proxyPort)}); err != nil {
				return fmt.Errorf("failed to redirect rule for %s: %w", subnet, err)
			}
		}
	}

	// delete
	for subnet := range currentSubnets {
		if _, found := newSubnets[subnet]; !found {
			if _, err := p.iptables([]string{"-t", "nat", "-D", chain, "-j", "REDIRECT", "--dest", subnet, "-p", "tcp", "--to-ports", strconv.Itoa(p.proxyPort)}); err != nil {
				return fmt.Errorf("failed to delete redirect rule for %s: %w", subnet, err)
			}
		}
	}

	p.subnets = subnets

	return nil
}

func (p *Iptables) Shutdown() error {
	chain := p.chainName()

	return p.deleteChain(chain)
}

func (p *Iptables) deleteChain(chain string) error {
	if _, err := p.iptables([]string{"-t", "nat", "-D", "OUTPUT", "-j", chain}); err != nil {
		return fmt.Errorf("failed to delete a jump rule to OUTPUT chain: %w", err)
	}

	if _, err := p.iptables([]string{"-t", "nat", "-D", "PREROUTING", "-j", chain}); err != nil {
		return fmt.Errorf("failed to delete a jump rule to OUTPUT chain: %w", err)
	}

	if _, err := p.iptables([]string{"-t", "nat", "-F", chain}); err != nil {
		return fmt.Errorf("failed to flush a chain: %w", err)
	}

	if _, err := p.iptables([]string{"-t", "nat", "-X", chain}); err != nil {
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
	re := regexp.MustCompile("mallet-pid(\\d+)")

	stdout, err := p.iptables([]string{"-t", "nat", "-n", "-L"})
	if err != nil {
		return err
	}

	pids := map[int]struct{}{}
	procs, err := ps.Processes()
	for _, proc := range procs {
		pids[proc.Pid()] = struct{}{}
	}

	for _, match := range re.FindAllStringSubmatch(stdout, -1) {
		pid, err := strconv.Atoi(match[1])
		if err != nil {
			return err
		}

		if _, ok := pids[pid]; !ok {
			p.logger.Info().Int("pid", pid).Msg("Deleting zombie iptables chain")
			if err := p.deleteChain(fmt.Sprintf("mallet-pid%d", pid)); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *Iptables) chainName() string {
	return fmt.Sprintf("mallet-pid%d", os.Getpid())
}

func (p *Iptables) iptables(args []string) (string, error) {
	stdout := &bytes.Buffer{}
	cmd := exec.Command("iptables", args...)
	cmd.Stdout = stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

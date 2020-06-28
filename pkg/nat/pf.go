package nat

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/rs/zerolog"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const pfConfMarker = " # added by tagane"
const pfConf = "/etc/pf.conf"

type PF struct {
	logger zerolog.Logger
}

func NewPF(logger zerolog.Logger) *PF {
	return &PF{
		logger: logger,
	}
}

func (p *PF) Setup(proxyPort int, subnets []string) error {
	// enable
	if err := p.pfctl([]string{"-E"}, nil, nil); err != nil {
		return fmt.Errorf("failed to enable pf: %w", err)
	}

	// main ruleset
	if err := p.writeMainRules(); err != nil {
		return err
	}

	if err := p.pfctl([]string{"-f", pfConf}, nil, nil); err != nil {
		return err
	}

	// load anchor
	if err := p.loadAnchorRules(proxyPort, subnets); err != nil {
		return fmt.Errorf("failed to load pf rules: %w", err)
	}

	return nil
}

func (p *PF) Shutdown() error {
	return p.pfctl([]string{"-F", "all", "-a", p.anchorName()}, nil, nil)
}

func (p *PF) GetNATDestination(conn *net.TCPConn) (string, *net.TCPConn, error) {
	stdout := &bytes.Buffer{}

	if err := p.pfctl([]string{"-s", "states"}, nil, stdout); err != nil {
		return "", nil, err
	}

	p.logger.Trace().Str("states", stdout.String()).Msg("Output of pfctl -s states")

	re, err := regexp.Compile(fmt.Sprintf("(?m)^ALL tcp %s -> ([^\\s]+).+$", regexp.QuoteMeta(conn.RemoteAddr().String())))
	if err != nil {
		return "", nil, err
	}

	p.logger.Trace().Str("re", re.String()).Msg("Finding state")

	if match := re.FindStringSubmatch(stdout.String()); match != nil {
		return match[1], conn, nil
	}

	return "", nil, StateNotFoundError
}

func (p *PF) pfctl(args []string, stdin io.Reader, stdout io.Writer) error {
	c := exec.Command("pfctl", args...)
	if stdin != nil {
		c.Stdin = stdin
	}
	if stdout != nil {
		c.Stdout = stdout
	}
	p.logger.Debug().Strs("args", c.Args).Msg("Running pfctl")
	if err := c.Run(); err != nil {
		return err
	}

	return nil
}

func (p *PF) anchorName() string {
	return fmt.Sprintf("tagane/pid%d", os.Getpid())
}

func (p *PF) loadAnchorRules(proxyPort int, subnets []string) error {
	buf := &bytes.Buffer{}

	for _, subnet := range subnets {
		fmt.Fprintf(buf, "rdr pass on lo0 inet proto tcp from ! 127.0.0.1 to %s -> 127.0.0.1 port %d\n", subnet, proxyPort)
		fmt.Fprintf(buf, "pass out route-to lo0 inet proto tcp from any to %s flags S/SA keep state\n", subnet)
	}
	fmt.Fprintf(buf, "pass out inet proto tcp from any to 127.0.0.1 flags S/SA keep state\n")

	p.logger.Debug().Str("rules", buf.String()).Msg("Loading pf rules")

	if err := p.pfctl([]string{"-a", p.anchorName(), "-f", "-"}, buf, nil); err != nil {
		return err
	}

	return nil
}

func (p *PF) generatePfConf() (string, error) {
	f, err := os.Open(pfConf)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var builder strings.Builder

	rdrAnchorAdded := false
	anchorAdded := false

	scanner := bufio.NewScanner(f)

	addRdrAnchor := func() {
		builder.WriteString(`rdr-anchor "tagane/*"`)
		builder.WriteString(pfConfMarker)
		builder.WriteString("\n")
	}
	addAnchor := func() {
		builder.WriteString(`anchor "tagane/*"`)
		builder.WriteString(pfConfMarker)
		builder.WriteString("\n")
	}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, pfConfMarker) {
			continue
		}
		builder.WriteString(line)
		builder.WriteString("\n")

		if !rdrAnchorAdded && strings.HasPrefix(line, "rdr-anchor ") {
			addRdrAnchor()
			rdrAnchorAdded = true
		} else if !anchorAdded && strings.HasPrefix(line, "anchor ") {
			addAnchor()
			anchorAdded = true
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	if !rdrAnchorAdded {
		addRdrAnchor()
	}
	if !anchorAdded {
		addAnchor()
	}

	return builder.String(), nil
}

func (p *PF) writeMainRules() error {
	f, err := os.OpenFile(fmt.Sprintf("%s.tmp", pfConf), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	content, err := p.generatePfConf()
	if err != nil {
		return err
	}

	if _, err := f.WriteString(content); err != nil {
		return err
	}

	f.Close()

	if err := os.Rename(fmt.Sprintf("%s.tmp", pfConf), pfConf); err != nil {
		return err
	}

	return nil
}

package nat

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/mitchellh/go-ps"
	"github.com/rs/zerolog"
	"github.com/ryotarai/tagane/pkg/priv"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const pfConfMarker = " # added by tagane"
const pfConf = "/etc/pf.conf"

type PF struct {
	logger     zerolog.Logger
	privClient *priv.Client
	proxyPort  int
}

func NewPF(logger zerolog.Logger, privClient *priv.Client, proxyPort int) *PF {
	return &PF{
		logger:     logger,
		privClient: privClient,
		proxyPort:  proxyPort,
	}
}

func (p *PF) Setup() error {
	// enable
	if _, err := p.pfctl([]string{"-E"}, ""); err != nil {
		return fmt.Errorf("failed to enable pf: %w", err)
	}

	// main ruleset
	if err := p.writeMainRules(); err != nil {
		return err
	}

	if _, err := p.pfctl([]string{"-f", pfConf}, ""); err != nil {
		return err
	}

	return nil
}

func (p *PF) RedirectSubnets(subnets []string) error {
	buf := &bytes.Buffer{}

	for _, subnet := range subnets {
		fmt.Fprintf(buf, "rdr pass on lo0 inet proto tcp from ! 127.0.0.1 to %s -> 127.0.0.1 port %d\n", subnet, p.proxyPort)
	}
	for _, subnet := range subnets {
		fmt.Fprintf(buf, "pass out route-to lo0 inet proto tcp from any to %s flags S/SA keep state\n", subnet)
	}

	p.logger.Debug().Str("rules", buf.String()).Msg("Loading pf rules")

	if _, err := p.pfctl([]string{"-a", p.anchorName(), "-f", "-"}, buf.String()); err != nil {
		return err
	}

	return nil
}

func (p *PF) Shutdown() error {
	_, err := p.pfctl([]string{"-F", "all", "-a", p.anchorName()}, "")
	return err
}

func (p *PF) GetNATDestination(conn *net.TCPConn) (string, *net.TCPConn, error) {
	stdout, err := p.pfctl([]string{"-s", "states"}, "")
	if err != nil {
		return "", nil, err
	}

	p.logger.Trace().Str("states", stdout).Msg("Output of pfctl -s states")

	re, err := regexp.Compile(fmt.Sprintf("(?m)^ALL tcp %s -> ([^\\s]+).+$", regexp.QuoteMeta(conn.RemoteAddr().String())))
	if err != nil {
		return "", nil, err
	}

	p.logger.Trace().Str("re", re.String()).Msg("Finding state")

	if match := re.FindStringSubmatch(stdout); match != nil {
		return match[1], conn, nil
	}

	return "", nil, StateNotFoundError
}

func (p *PF) Cleanup() error {
	stdout, err := p.pfctl([]string{"-s", "Anchors", "-a", "tagane"}, "")
	if err != nil {
		return err
	}

	pids := map[int]struct{}{}
	procs, err := ps.Processes()
	for _, proc := range procs {
		pids[proc.Pid()] = struct{}{}
	}

	re := regexp.MustCompile("tagane/pid(\\d+)")
	for _, match := range re.FindAllStringSubmatch(stdout, -1) {
		pid, err := strconv.Atoi(match[1])
		if err != nil {
			return err
		}

		if _, ok := pids[pid]; !ok {
			p.logger.Info().Int("pid", pid).Msg("Deleting zombie pf anchor")
			if _, err := p.pfctl([]string{"-F", "all", "-a", p.anchorNameForPid(pid)}, ""); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *PF) pfctl(args []string, stdin string) (string, error) {
	resp, err := p.privClient.Command(&priv.CommandRequest{
		Command: "pfctl",
		Args:    args,
		Stdin:   stdin,
	})

	if err != nil {
		return "", err
	}

	if resp.ExitCode != 0 {
		return "", fmt.Errorf("pfctl exit status: %d", resp.ExitCode)
	}

	return resp.Stdout, nil
}

func (p *PF) anchorName() string {
	return p.anchorNameForPid(os.Getpid())
}

func (p *PF) anchorNameForPid(pid int) string {
	return fmt.Sprintf("tagane/pid%d", pid)
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
	content, err := p.generatePfConf()
	if err != nil {
		return err
	}

	resp, err := p.privClient.WritePfConf(&priv.WritePfConfRequest{
		Content: content,
	})
	if err != nil {
		return err
	}

	if resp.Error != "" {
		return fmt.Errorf(resp.Error)
	}

	return nil
}

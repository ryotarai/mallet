package priv

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog"
	"os"
	"os/exec"
	"strings"
)

type Server struct {
	logger zerolog.Logger
}

func NewServer(logger zerolog.Logger) *Server {
	return &Server{
		logger: logger,
	}
}

func (s *Server) Start() error {
	var err error

	pipe := os.Stdout

	fmt.Fprintln(pipe, "ready1")
	line, err := bufio.NewReader(pipe).ReadString('\n')
	if err != nil {
		return err
	}
	if line != "ready2\n" {
		return fmt.Errorf("not expected str: %s", line)
	}
	fmt.Fprintln(pipe, "ready3")

	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "\t", 2)
		action := parts[0]
		reqJSON := parts[1]

		s.logger.Debug().Str("action", action).Str("req", reqJSON).Msg("received request")

		var resp interface{}
		switch action {
		case CommandAction:
			resp, err = s.handleCommand(reqJSON)
		case WritePfConfAction:
			resp, err = s.handleWritePfConf(reqJSON)
		}
		if err != nil {
			return err
		}

		if err := json.NewEncoder(pipe).Encode(resp); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleCommand(reqJSON string) (*CommandResponse, error) {
	req := &CommandRequest{}
	if err := json.NewDecoder(strings.NewReader(reqJSON)).Decode(req); err != nil {
		return nil, err
	}

	switch req.Command {
	case "pfctl", "iptables":
	default:
		return nil, fmt.Errorf("%s is not allowed", req.Command)
	}

	stdin := strings.NewReader(req.Stdin)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	cmd := exec.Command(req.Command, req.Args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	s.logger.Debug().Str("command", req.Command).Strs("args", req.Args).Msg("Running a command")

	var exitCode int
	if err := cmd.Run(); err != nil {
		if eerr, ok := err.(*exec.ExitError); ok {
			exitCode = eerr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return &CommandResponse{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, nil
}

const pfConfPath = "/etc/pf.conf"

func (s *Server) handleWritePfConf(reqJSON string) (*WritePfConfResponse, error) {
	req := &WritePfConfRequest{}
	if err := json.NewDecoder(strings.NewReader(reqJSON)).Decode(req); err != nil {
		return nil, err
	}

	s.logger.Debug().Str("content", req.Content).Msgf("Writing %s", pfConfPath)

	f, err := os.OpenFile(fmt.Sprintf("%s.tmp", pfConfPath), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return &WritePfConfResponse{
			Error: err.Error(),
		}, nil
	}
	defer f.Close()

	if _, err := f.WriteString(req.Content); err != nil {
		return &WritePfConfResponse{
			Error: err.Error(),
		}, nil
	}

	f.Close()

	if err := os.Rename(fmt.Sprintf("%s.tmp", pfConfPath), pfConfPath); err != nil {
		return &WritePfConfResponse{
			Error: err.Error(),
		}, nil
	}

	return &WritePfConfResponse{
		Error: "",
	}, nil
}


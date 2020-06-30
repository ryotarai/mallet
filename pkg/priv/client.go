package priv

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type Client struct {
	cmd    *exec.Cmd
	pipeR  io.Reader
	pipeW  io.Writer
	mutex  sync.Mutex
	logger zerolog.Logger
}

func NewClient(logger zerolog.Logger) *Client {
	return &Client{
		logger: logger,
		mutex:  sync.Mutex{},
	}
}

func (c *Client) Start() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}

	// client->server
	r1, w1, err := os.Pipe()
	if err != nil {
		return err
	}
	// server->client
	r2, w2, err := os.Pipe()
	if err != nil {
		return err
	}

	var args []string
	if !(os.Geteuid() == 0 && os.Getegid() == 0) {
		args = append(args, "sudo", "-p", "[local sudo] Password:")
	}
	args = append(args, self, "priv", "--log-level", c.logger.GetLevel().String())

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = w2
	cmd.Stderr = os.Stderr
	cmd.Stdin = r1
	if err := cmd.Start(); err != nil {
		return err
	}

	c.cmd = cmd
	c.pipeR = r2
	c.pipeW = w1

	// ping pong
	line, err := c.readLine()
	if err != nil {
		return err
	}
	if line != "ready1" {
		return fmt.Errorf("not expected str: %s", line)
	}

	fmt.Fprintln(c.pipeW, "ready2")

	line, err = c.readLine()
	if err != nil {
		return err
	}
	if line != "ready3" {
		return fmt.Errorf("not expected str: %s", line)
	}

	return nil
}

func (c *Client) Command(req *CommandRequest) (*CommandResponse, error) {
	resp := &CommandResponse{}
	if err := c.action(CommandAction, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) WritePfConf(req *WritePfConfRequest) (*WritePfConfResponse, error) {
	resp := &WritePfConfResponse{}
	if err := c.action(WritePfConfAction, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) action(action string, req interface{}, resp interface{}) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	b, err := json.Marshal(req)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(c.pipeW, "%s\t%s\n", action, string(b)); err != nil {
		return err
	}

	line, err := c.readLine()
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(line), resp); err != nil {
		return err
	}

	return nil
}

func (c *Client) readLine() (string, error) {
	line, err := bufio.NewReader(c.pipeR).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

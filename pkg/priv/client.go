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
	"syscall"
)

type Client struct {
	cmd    *exec.Cmd
	pipe   io.ReadWriter
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

	fds, err := syscall.Socketpair(syscall.AF_LOCAL, syscall.SOCK_STREAM, 0)
	if err != nil {
		return err
	}
	f0 := os.NewFile(uintptr(fds[0]), "sock0")
	f1 := os.NewFile(uintptr(fds[1]), "sock1")

	var args []string
	if !(os.Geteuid() == 0 && os.Getegid() == 0) {
		args = append(args, "sudo", "-p", "[local sudo] Password:")
	}
	args = append(args, self, "priv", "--log-level", c.logger.GetLevel().String())

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = f0
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return err
	}
	f0.Close()

	c.cmd = cmd
	c.pipe = f1
	// TODO: close f1 on shutdown

	// ping pong
	line, err := c.readLine()
	if err != nil {
		return err
	}
	if line != "ready1" {
		return fmt.Errorf("not expected str: %s", line)
	}

	fmt.Fprintln(c.pipe, "ready2")

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

	if _, err := fmt.Fprintf(c.pipe, "%s\t%s\n", action, string(b)); err != nil {
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
	line, err := bufio.NewReader(c.pipe).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

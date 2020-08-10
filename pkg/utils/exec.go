package utils

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

func RunCommand(cmd *exec.Cmd) error {
	buf := &bytes.Buffer{}
	if cmd.Stderr == nil {
		cmd.Stderr = buf
	} else {
		cmd.Stderr = io.MultiWriter(cmd.Stderr, buf)
	}
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSuffix(buf.String(), "\n"))
	}
	return nil
}

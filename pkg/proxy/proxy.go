package proxy

import (
	"fmt"
	"github.com/rs/zerolog"
	"github.com/ryotarai/tagane/pkg/nat"
	"github.com/ryotarai/tagane/pkg/utils"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

type Proxy struct {
	Logger        zerolog.Logger
	nat           nat.NAT
	chiselServer  string
	chiselOptions []string
}

func New(logger zerolog.Logger, nat nat.NAT, chiselServer string, chiselOptions []string) *Proxy {
	return &Proxy{
		Logger:        logger.With().Str("component", "proxy").Logger(),
		nat:           nat,
		chiselServer:  chiselServer,
		chiselOptions: chiselOptions,
	}
}

func (p *Proxy) Start(port int) error {
	addr, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("failed to resolve address: %w", err)
	}

	listener, err := net.ListenTCP("tcp4", addr)
	if err != nil {
		return fmt.Errorf("failed to listen TCP: %w", err)
	}
	defer listener.Close()

	p.Logger.Info().Msgf("Listening on %s", addr.String())

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			if ne, ok := err.(net.Error); ok {
				if ne.Temporary() {
					p.Logger.Warn().Err(err).Msg("Failed to accept TCP")
					continue
				}
			}
			return err
		}

		go func(conn *net.TCPConn) {
			if err := p.handleConn(conn); err != nil {
				p.Logger.Warn().Err(err).Msg("Failed to handle TCP connection")
			}
		}(conn)
	}

	return nil
}

func (p *Proxy) handleConn(conn *net.TCPConn) error {
	defer conn.Close()

	dest, newConn, err := p.nat.GetNATDestination(conn)
	if err != nil {
		return err
	}
	conn = newConn

	p.Logger.Debug().Str("src", conn.RemoteAddr().String()).Str("dst", dest).Msg("Starting proxy")

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	self, err := os.Executable()
	if err != nil {
		return err
	}

	var args []string
	args = append(args, "chisel-client")
	args = append(args, p.chiselOptions...)
	args = append(args, p.chiselServer, fmt.Sprintf("stdio:%s", dest))

	p.Logger.Debug().Strs("args", args).Msg("Starting chisel client")

	chisel := exec.Command(self, args...)
	chisel.Stdin = stdinR
	chisel.Stdout = stdoutW
	chisel.Stderr = &utils.LoggerWriter{Logger: p.Logger.With().Str("from", "chisel-client").Logger(), Level: zerolog.DebugLevel}
	if err := chisel.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		if _, err := io.Copy(stdinW, conn); err != nil {
			p.Logger.Warn().Err(err).Msg("local->remote")
		}
		p.Logger.Debug().Msg("local->remote copy done")
		stdoutR.Close()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		if _, err := io.Copy(conn, stdoutR); err != nil {
			if operr, ok := err.(*net.OpError); ok && operr.Unwrap() == io.ErrClosedPipe {
				// expected
				p.Logger.Debug().Err(operr).Msg("closed pipe")
			} else {
				p.Logger.Warn().Err(err).Msg("remote->local")
			}
		}

		p.Logger.Debug().Msg("remote->local copy done")
	}()

	wg.Wait()

	p.Logger.Debug().Msg("Stopping chisel-client")
	if err := chisel.Process.Signal(syscall.SIGTERM); err != nil {
		return err
	}

	return nil
}

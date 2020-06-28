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
	"syscall"
)

type Proxy struct {
	Logger       zerolog.Logger
	nat          nat.NAT
	chiselServer string
}

func New(logger zerolog.Logger, nat nat.NAT, chiselServer string) *Proxy {
	return &Proxy{
		Logger:       logger.With().Str("component", "proxy").Logger(),
		nat:          nat,
		chiselServer: chiselServer,
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

	chisel := exec.Command(self, "chisel-client", p.chiselServer, fmt.Sprintf("stdio:%s", dest))
	chisel.Stdin = stdinR
	chisel.Stdout = stdoutW
	chisel.Stderr = &utils.LoggerWriter{Logger: p.Logger.With().Str("from", "chisel-client").Logger(), Level: zerolog.DebugLevel}
	if err := chisel.Start(); err != nil {
		return err
	}

	stopCh1 := make(chan struct{})
	stopCh2 := make(chan struct{})

	go func() {
		if err := pipe(p.Logger.With().Str("direction", "local->remote").Logger(), conn, stdinW); err != nil {
			if err == io.EOF {
				p.Logger.Debug().Err(err).Msg("EOF on local->remote pipe")
			} else {
				p.Logger.Warn().Err(err).Msg("Error on local->remote pipe")
			}
			stdoutR.Close() // to stop remote->local pipe
			close(stopCh1)
		}
	}()
	go func() {
		if err := pipe(p.Logger.With().Str("direction", "remote->local").Logger(), stdoutR, conn); err != nil {
			if err == io.ErrClosedPipe {
				p.Logger.Debug().Err(err).Msg("Closed pipe on remote->local pipe")
			} else {
				p.Logger.Warn().Err(err).Msg("Error on local->remote pipe")
			}
			conn.Close() // to stop local->remote pipe
			close(stopCh2)
		}
	}()

	<-stopCh1
	<-stopCh2

	p.Logger.Debug().Msg("Stopping chisel-client")
	if err := chisel.Process.Signal(syscall.SIGTERM); err != nil {
		return err
	}

	return nil
}

func pipe(logger zerolog.Logger, src io.Reader, dst io.Writer) error {
	buff := make([]byte, 64*1024) // 64 kB
	for {
		n, err := src.Read(buff)
		if err != nil {
			return err
		}
		//logger.Trace().Int("bytes", n).Msg("read")
		b := buff[:n]

		n, err = dst.Write(b)
		if err != nil {
			return err
		}
		//logger.Trace().Int("bytes", n).Msg("write")
	}
}

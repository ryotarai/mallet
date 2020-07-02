package proxy

import (
	"context"
	"fmt"
	chclient "github.com/jpillora/chisel/client"
	chshare "github.com/jpillora/chisel/share"
	"github.com/rs/zerolog"
	"github.com/ryotarai/mallet/pkg/nat"
	"io"
	"net"
	"sync"
)

type Proxy struct {
	Logger       zerolog.Logger
	nat          nat.NAT
	chiselConfig *chclient.Config
	chiselClient *chclient.Client
}

func New(logger zerolog.Logger, nat nat.NAT, chiselConfig *chclient.Config) *Proxy {
	return &Proxy{
		Logger:       logger.With().Str("component", "proxy").Logger(),
		nat:          nat,
		chiselConfig: chiselConfig,
	}
}

func (p *Proxy) Start(port int) error {
	// chisel
	c, err := chclient.NewClient(p.chiselConfig)
	if err != nil {
		return err
	}
	p.chiselClient = c

	p.chiselClient.Logger.Debug = true
	if err := p.chiselClient.Start(context.TODO()); err != nil {
		return err
	}

	// listen
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

type readWriteCloser struct {
	io.ReadCloser
	io.Writer
}

func (p *Proxy) handleConn(conn *net.TCPConn) error {
	defer conn.Close()

	dest, newConn, err := p.nat.GetNATDestination(conn)
	if err != nil {
		return err
	}
	conn = newConn

	p.Logger.Debug().Str("src", conn.RemoteAddr().String()).Str("dst", dest).Msg("Starting proxy")

	// me -> chisel
	pipeOutR, pipeOutW := io.Pipe()
	// chisel -> me
	pipeInR, pipeInW := io.Pipe()

	host, port, err := net.SplitHostPort(dest)
	if err != nil {
		return err
	}

	proxy := p.chiselClient.CreateTCPProxy(0, &chshare.Remote{
		RemoteHost: host,
		RemotePort: port,
		LocalIO: &readWriteCloser{
			ReadCloser: pipeOutR,
			Writer:     pipeInW,
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	if err := proxy.Start(ctx); err != nil {
		return err
	}
	defer cancel()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		if _, err := io.Copy(pipeOutW, conn); err != nil {
			p.Logger.Warn().Err(err).Msg("local->remote")
		}
		p.Logger.Debug().Msg("local->remote copy done")
		pipeInR.Close() // stop remote->local
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		if _, err := io.Copy(conn, pipeInR); err != nil {
			if operr, ok := err.(*net.OpError); ok && operr.Unwrap() == io.ErrClosedPipe {
				// expected
				p.Logger.Debug().Err(operr).Msg("closed pipe")
			} else {
				p.Logger.Warn().Err(err).Msg("remote->local")
			}
		}

		p.Logger.Debug().Msg("remote->local copy done")
		pipeOutW.Close()
	}()

	wg.Wait()

	return nil
}

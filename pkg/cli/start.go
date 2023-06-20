package cli

import (
	chclient "github.com/jpillora/chisel/client"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ryotarai/mallet/pkg/nat"
	"github.com/ryotarai/mallet/pkg/proxy"
	"github.com/ryotarai/mallet/pkg/resolver"
	"github.com/spf13/cobra"
)

var startFlags struct {
	chiselServer     string
	listenPort       int
	listenHost       string
	dnsCheckInterval time.Duration
	excludeSubnets   []string

	chiselFingerprint      string
	chiselAuth             string
	chiselKeepalive        time.Duration
	chiselMaxRetryCount    int
	chiselMaxRetryInterval time.Duration
	chiselProxy            string
	chiselHostname         string
}

func init() {
	c := &cobra.Command{
		Use:  "start",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			logger.Debug().Strs("args", args).Msgf("Starting")

			// check user is root
			if os.Getegid() != 0 {
				logger.Warn().Msg("Mallet requires root privilege to redirect packets")
			}

			// find port
			listenPort := startFlags.listenPort
			if listenPort == 0 {
				port, err := findFreeTCPPort()
				if err != nil {
					return err
				}
				listenPort = port
			}

			sigCh := make(chan os.Signal)
			signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

			exitCh := make(chan struct{})

			nat, err := nat.New(logger, listenPort)
			if err != nil {
				return err
			}

			if err := nat.Cleanup(); err != nil {
				return err
			}

			if err := nat.Setup(); err != nil {
				return err
			}

			resolver := resolver.New(logger, nat, startFlags.excludeSubnets)
			go func() {
				resolver.Start(startFlags.dnsCheckInterval, args)
			}()

			chiselHeaders := http.Header{}
			if startFlags.chiselHostname != "" {
				chiselHeaders["Host"] = []string{startFlags.chiselHostname}
			}
			chiselConfig := &chclient.Config{
				Fingerprint:      startFlags.chiselFingerprint,
				Auth:             startFlags.chiselAuth,
				KeepAlive:        startFlags.chiselKeepalive,
				MaxRetryCount:    startFlags.chiselMaxRetryCount,
				MaxRetryInterval: startFlags.chiselMaxRetryInterval,
				Server:           startFlags.chiselServer,
				Proxy:            startFlags.chiselProxy,
				Headers:          chiselHeaders,
			}

			prx := proxy.New(logger, nat, chiselConfig)
			go func() {
				if err := prx.Start(startFlags.listenHost, listenPort); err != nil {
					logger.Error().Err(err).Msg("")
					close(exitCh)
				}
			}()

			select {
			case sig := <-sigCh:
				logger.Debug().Str("signal", sig.String()).Msg("Received signal")
			case <-exitCh:
			}

			logger.Info().Msg("Shutting down")
			resolver.Stop()
			if err := nat.Shutdown(); err != nil {
				logger.Warn().Err(err).Msg("Failed to shutdown NAT")
			}

			return nil
		},
	}
	c.Flags().StringVar(&startFlags.chiselServer, "chisel-server", "", "")
	c.MarkFlagRequired("chisel-server")
	c.Flags().IntVar(&startFlags.listenPort, "listen-port", 0, "0 for auto")
	c.Flags().StringVar(&startFlags.listenHost, "listen-host", "127.0.0.1", "local proxy server listens on")
	c.Flags().DurationVar(&startFlags.dnsCheckInterval, "dns-check-interval", time.Minute*5, "")
	c.Flags().StringSliceVar(&startFlags.excludeSubnets, "exclude-subnet", nil, "subnets to exclude")

	// flags for chisel client
	c.Flags().StringVar(&startFlags.chiselFingerprint, "chisel-fingerprint", "", "")
	c.Flags().StringVar(&startFlags.chiselAuth, "chisel-auth", "", "")
	c.Flags().DurationVar(&startFlags.chiselKeepalive, "chisel-keepalive", 0, "")
	c.Flags().IntVar(&startFlags.chiselMaxRetryCount, "chisel-max-retry-count", -1, "")
	c.Flags().DurationVar(&startFlags.chiselMaxRetryInterval, "chisel-max-retry-interval", 0, "")
	c.Flags().StringVar(&startFlags.chiselProxy, "chisel-proxy", "", "")
	c.Flags().StringVar(&startFlags.chiselHostname, "chisel-hostname", "", "")

	rootCmd.AddCommand(c)
}

func findFreeTCPPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}

	if err := l.Close(); err != nil {
		return 0, err
	}

	parts := strings.Split(l.Addr().String(), ":")
	return strconv.Atoi(parts[len(parts)-1])
}

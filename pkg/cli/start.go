package cli

import (
	"github.com/ryotarai/tagane/pkg/nat"
	"github.com/ryotarai/tagane/pkg/priv"
	"github.com/ryotarai/tagane/pkg/proxy"
	"github.com/spf13/cobra"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

var startFlags struct {
	chiselServer string
	listenPort   int
	subnets      []string

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
		Use: "start",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger.Debug().Strs("subnets", startFlags.subnets).Msgf("Starting")

			sigCh := make(chan os.Signal)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			exitCh := make(chan struct{})

			privClient := priv.NewClient(logger)
			if err := privClient.Start(); err != nil {
				return err
			}

			nat, err := nat.New(logger, privClient)
			if err != nil {
				return err
			}

			if err := nat.Setup(startFlags.listenPort, startFlags.subnets); err != nil {
				return err
			}

			prx := proxy.New(logger, nat, startFlags.chiselServer, chiselOptions())
			go func() {
				if err := prx.Start(startFlags.listenPort); err != nil {
					logger.Error().Err(err).Msg("")
					close(exitCh)
				}
			}()

			select {
			case <-sigCh:
			case <-exitCh:
			}

			logger.Info().Msg("Shutting down")
			if err := nat.Shutdown(); err != nil {
				logger.Warn().Err(err).Msg("Failed to shutdown NAT")
			}

			return nil
		},
	}
	c.Flags().StringVar(&startFlags.chiselServer, "chisel-server", "", "")
	c.MarkFlagRequired("chisel-server")
	c.Flags().IntVar(&startFlags.listenPort, "listen-port", 10000, "")
	c.Flags().StringArrayVar(&startFlags.subnets, "subnet", nil, "")
	c.MarkFlagRequired("subnet")

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

func chiselOptions() []string {
	var opts []string

	opts = append(opts, "--fingerprint", startFlags.chiselFingerprint)
	opts = append(opts, "--auth", startFlags.chiselAuth)
	opts = append(opts, "--keepalive", startFlags.chiselKeepalive.String())
	opts = append(opts, "--max-retry-count", strconv.Itoa(startFlags.chiselMaxRetryCount))
	opts = append(opts, "--max-retry-interval", startFlags.chiselMaxRetryInterval.String())
	opts = append(opts, "--proxy", startFlags.chiselProxy)
	opts = append(opts, "--hostname", startFlags.chiselHostname)

	return opts
}

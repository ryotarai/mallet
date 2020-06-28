package cli

import (
	"github.com/spf13/cobra"
	"github.com/ryotarai/tagane/pkg/nat"
	"github.com/ryotarai/tagane/pkg/proxy"
	"os"
	"os/signal"
	"syscall"
)

var startFlags struct {
	chiselServer string
	listenPort   int
	subnets      []string
}

func init() {
	c := &cobra.Command{
		Use: "start",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger.Debug().Strs("subnets", startFlags.subnets).Msgf("Starting")

			sigCh := make(chan os.Signal)
			signal.Notify(sigCh, syscall.SIGINT)

			exitCh := make(chan struct{})

			nat, err := nat.New(logger)
			if err != nil {
				return err
			}

			if err := nat.Setup(startFlags.listenPort, startFlags.subnets); err != nil {
				return err
			}

			prx := proxy.New(logger, nat, startFlags.chiselServer)
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

	rootCmd.AddCommand(c)
}

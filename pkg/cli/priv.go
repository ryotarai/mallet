package cli

import (
	"github.com/ryotarai/tagane/pkg/priv"
	"github.com/spf13/cobra"
	"os"
	"os/signal"
	"syscall"
)

func init() {
	c := &cobra.Command{
		Use:    "priv",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// ignore signal to avoid to exit before main process
			sigCh := make(chan os.Signal)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			return priv.NewServer(logger.With().Str("component", "priv").Logger()).Start()
		},
	}

	rootCmd.AddCommand(c)
}

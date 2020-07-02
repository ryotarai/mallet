package cli

import (
	"github.com/ryotarai/mallet/pkg/nat"
	"github.com/spf13/cobra"
)

var cleanupFlags struct {
}

func init() {
	c := &cobra.Command{
		Use: "cleanup",
		RunE: func(cmd *cobra.Command, args []string) error {
			nat, err := nat.New(logger, -1)
			if err != nil {
				return err
			}

			if err := nat.Cleanup(); err != nil {
				return err
			}

			return nil
		},
	}

	rootCmd.AddCommand(c)
}

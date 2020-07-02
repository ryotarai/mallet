package cli

import (
	"fmt"
	"github.com/ryotarai/mallet/pkg/version"
	"github.com/spf13/cobra"
)

func init() {
	c := &cobra.Command{
		Use: "version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(version.Version)
			return nil
		},
	}

	rootCmd.AddCommand(c)
}

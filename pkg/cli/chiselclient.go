package cli

import (
	chclient "github.com/jpillora/chisel/client"
	"github.com/spf13/cobra"
	"net/http"
	"os"
	"time"
)

var chiselClientFlags struct {
	fingerprint      string
	auth             string
	keepalive        time.Duration
	maxRetryCount    int
	maxRetryInterval time.Duration
	proxy            string
	hostname         string
	verbose          bool
}

func init() {
	c := &cobra.Command{
		Use:    "chisel-client",
		Args:   cobra.MinimumNArgs(2),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			config := chclient.Config{
				Fingerprint:      chiselClientFlags.fingerprint,
				Auth:             chiselClientFlags.auth,
				KeepAlive:        chiselClientFlags.keepalive,
				MaxRetryCount:    chiselClientFlags.maxRetryCount,
				MaxRetryInterval: chiselClientFlags.maxRetryInterval,
				Headers:          http.Header{},
			}
			config.Server = args[0]
			config.Remotes = args[1:]

			if config.Auth == "" {
				config.Auth = os.Getenv("AUTH")
			}
			if chiselClientFlags.hostname != "" {
				config.Headers.Set("Host", chiselClientFlags.hostname)
			}

			c, err := chclient.NewClient(&config)
			if err != nil {
				return err
			}
			c.Debug = chiselClientFlags.verbose
			if err = c.Run(); err != nil {
				return err
			}
			return nil
		},
	}
	c.Flags().StringVar(&chiselClientFlags.fingerprint, "fingerprint", "", "")
	c.Flags().StringVar(&chiselClientFlags.auth, "auth", "", "")
	c.Flags().DurationVar(&chiselClientFlags.keepalive, "keepalive", 0, "")
	c.Flags().IntVar(&chiselClientFlags.maxRetryCount, "max-retry-count", -1, "")
	c.Flags().DurationVar(&chiselClientFlags.maxRetryInterval, "max-retry-interval", 0, "")
	c.Flags().StringVar(&chiselClientFlags.proxy, "proxy", "", "")
	c.Flags().StringVar(&chiselClientFlags.hostname, "hostname", "", "")
	c.Flags().BoolVar(&chiselClientFlags.verbose, "verbose", false, "")

	rootCmd.AddCommand(c)
}

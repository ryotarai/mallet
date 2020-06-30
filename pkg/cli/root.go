package cli

import (
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"os"
	"time"
)

var logger zerolog.Logger

var rootFlags struct {
	logLevel   string
	configPath string
}

var rootCmd = &cobra.Command{
	Use:           "tagane",
	SilenceUsage:  true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := setupLogger(); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&rootFlags.logLevel, "log-level", "", "info", "log level (one of debug, info, warn and error)")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logger.Error().Msg(err.Error())
		os.Exit(1)
	}
}

func setupLogger() error {
	l, err := zerolog.ParseLevel(rootFlags.logLevel)
	if err != nil {
		return err
	}

	logger = zerolog.New(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
	}).Level(l).With().Timestamp().Logger()

	return nil
}

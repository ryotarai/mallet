package utils

import "github.com/rs/zerolog"

type LoggerWriter struct {
	Logger zerolog.Logger
	Level zerolog.Level
}

func (w *LoggerWriter) Write(p []byte) (int, error) {
	n := len(p)
	if n > 0 && p[n-1] == '\n' {
		p = p[0 : n-1]
	}
	w.Logger.WithLevel(w.Level).Msg(string(p))
	return n, nil
}


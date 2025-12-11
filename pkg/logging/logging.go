package logging

import (
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
)

var Logger *slog.Logger

func init() {
	// You can tweak options here:
	handler := tint.NewHandler(os.Stderr, &tint.Options{
		// Minimum log level (Info, Debug, etc.)
		Level: slog.LevelDebug,

		// Show time (you can also set a custom time format)
		TimeFormat: time.RFC3339,

		// Add source file:line
		AddSource: true,

		// You can also set NoColor: true if needed, but you want colors :)
	})

	Logger = slog.New(handler)
}

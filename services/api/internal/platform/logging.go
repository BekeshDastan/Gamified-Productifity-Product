package platform

import (
	"io"
	"log/slog"
	"strings"
)

func NewLogger(out io.Writer, level string) *slog.Logger {
	var minimum slog.Level
	_ = minimum.UnmarshalText([]byte(level))
	return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{
		Level: minimum,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			key := strings.ToLower(a.Key)
			for _, private := range []string{"password", "secret", "token", "cookie", "authorization", "database_url"} {
				if strings.Contains(key, private) {
					return slog.String(a.Key, "[redacted]")
				}
			}
			return a
		},
	}))
}

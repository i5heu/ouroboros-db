package apiServer

import (
	"encoding/json"
	"log/slog"
	"net/http"

	cryptHash "github.com/i5heu/ouroboros-crypt/pkg/hash"
)

func parseHash(value string) (cryptHash.Hash, error) { // A
	if value == "" {
		return cryptHash.Hash{}, nil
	}

	return cryptHash.HashHexadecimal(value)
}

func writeJSON(w http.ResponseWriter, status int, payload any) { // A
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Default().Error("failed to encode response", "error", err)
	}
}

func WithLogger(logger *slog.Logger) Option { // HC
	return func(s *Server) {
		if logger != nil {
			s.log = logger
		}
	}
}

func WithAuth(auth AuthFunc) Option { // HC
	return func(s *Server) {
		if auth != nil {
			s.auth = auth
		}
	}
}

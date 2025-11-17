package apiServer

import (
	"log/slog"
	"net/http"

	ouroboros "github.com/i5heu/ouroboros-db"
)

const (
	defaultDataShards   = 4
	defaultParityShards = 2
)

type Server struct {
	mux  *http.ServeMux
	db   *ouroboros.OuroborosDB
	log  *slog.Logger
	auth AuthFunc
}

func New(db *ouroboros.OuroborosDB, opts ...Option) *Server { // A
	s := &Server{
		mux:  http.NewServeMux(),
		db:   db,
		log:  slog.Default(),
		auth: defaultAuth,
	}

	for _, opt := range opts {
		opt(s)
	}

	s.routes()
	return s
}

func (s *Server) routes() { // AC
	s.mux.HandleFunc("POST /data", s.handleCreate)
	s.mux.HandleFunc("GET /data/{key}", s.handleGet)
	s.mux.HandleFunc("GET /data/{key}/children", s.handleChildren)
	s.mux.HandleFunc("GET /data", s.handleList)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { // AC
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = "*"
	} else {
		w.Header().Set("Vary", "Origin")
	}

	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}

	allowedHeaders := r.Header.Get("Access-Control-Request-Headers")
	if allowedHeaders == "" {
		allowedHeaders = "Content-Type, Accept"
	}
	w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
	w.Header().Set(
		"Access-Control-Expose-Headers",
		"Content-Type, Content-Length, X-Ouroboros-Key, X-Ouroboros-Mime, X-Ouroboros-Is-Text, X-Ouroboros-Parent, X-Ouroboros-Children, X-Ouroboros-Created-At",
	)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := s.auth(r); err != nil {
		s.log.Warn("authentication failed", "error", err)
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	s.mux.ServeHTTP(w, r)
}

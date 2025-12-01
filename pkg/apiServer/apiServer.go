package apiServer

import (
	"log/slog"
	"net/http"

	ouroboros "github.com/i5heu/ouroboros-db"
	"github.com/i5heu/ouroboros-db/internal/browserCrypt"
	indexpkg "github.com/i5heu/ouroboros-db/pkg/index"
)

const (
	defaultDataShards   = 4
	defaultParityShards = 2
)

type Server struct {
	mux       *http.ServeMux
	db        *ouroboros.OuroborosDB
	log       *slog.Logger
	auth      AuthFunc
	authStore browserCrypt.AuthStore
	indexer   *indexpkg.Indexer
}

func New(db *ouroboros.OuroborosDB, opts ...Option) *Server { // A
	s := &Server{
		mux:  http.NewServeMux(),
		db:   db,
		log:  slog.Default(),
		auth: defaultAuth,
		authStore: browserCrypt.AuthStore{
			OTK: make(map[string]browserCrypt.BrowserKey),
		},
	}

	for _, opt := range opts {
		opt(s)
	}

	// If an indexer wasn't provided via options, use the db's indexer if available.
	if s.indexer == nil && db != nil {
		s.indexer = db.Indexer()
	}

	s.routes()
	return s
}

func (s *Server) routes() { // AC
	s.mux.HandleFunc("POST /data", s.handleCreate)
	s.mux.HandleFunc("GET /data/{key}", s.handleGet)
	s.mux.HandleFunc("GET /data/{key}/children", s.handleChildren)
	s.mux.HandleFunc("GET /data", s.handleList)
	s.mux.HandleFunc("POST /data/bulk", s.handleBulkData)
	s.mux.HandleFunc("GET /meta/threads", s.handleThreadSummaries)
	s.mux.HandleFunc("GET /meta/thread/{key}/stream", s.handleThreadNodeStream)
	s.mux.HandleFunc("GET /authProcess", s.handleAuthProcess)
	s.mux.HandleFunc("POST /authProcess", s.handleAuthProcess)
	s.mux.HandleFunc("POST /search", s.handleSearch)
	s.mux.HandleFunc("GET /lookup/", s.handleLookupByComputedID)
	s.mux.HandleFunc("GET /lookupData/", s.handleLookupWithData)
	s.mux.HandleFunc("GET /computedId/{key}", s.handleGetComputedID)
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

	// Use explicit, fixed CORS header lists so preflight responses are cacheable
	// and not dynamically varied by the incoming Access-Control-Request-Headers header.
	// Keep headers aligned with what the client sends: Content-Type + Accept + our
	// custom X-Auth headers and internal X-Ouroboros response/exposed headers.
	allowedHeaders := "Content-Type, Accept, X-Auth-Token, X-Auth-Nonce, X-Auth-KeyHash-Base64"
	w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
	// Allow caching of preflight responses so repeated identical requests don't cause
	// a preflight for every request. 86400s == 24 hours.
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
	// If the API ever uses browser credentials (cookies) set to true. Current
	// client uses custom headers, so credentials is not required; enable as false for now.
	// If credentials are used, then Access-Control-Allow-Origin must not be '*'.
	// w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set(
		"Access-Control-Expose-Headers",
		"Content-Type, Content-Length, X-Ouroboros-Key, X-Ouroboros-Requested-Key, X-Ouroboros-Suggested-Edit, X-Ouroboros-Edit-Of, X-Ouroboros-Mime, X-Ouroboros-Is-Text, X-Ouroboros-Parent, X-Ouroboros-Children, X-Ouroboros-Created-At, X-Ouroboros-Title",
	)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.URL.Path != "/authProcess" {
		if err := s.auth(r, s.db); err != nil {
			s.log.Warn("authentication failed", "error", err)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
	}

	s.mux.ServeHTTP(w, r)
}

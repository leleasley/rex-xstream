package api

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/leleasley/rex-xstream/internal/config"
	"github.com/leleasley/rex-xstream/internal/store"
	xsync "github.com/leleasley/rex-xstream/internal/sync"
)

type Server struct {
	cfg     *config.Config
	store   *store.Store
	syncer  *xsync.Syncer
	log     *slog.Logger
	started time.Time
	limiter *rateLimiter
	client  *http.Client
}

func New(cfg *config.Config, st *store.Store, syncer *xsync.Syncer, logger *slog.Logger) *Server {
	return &Server{
		cfg:     cfg,
		store:   st,
		syncer:  syncer,
		log:     logger,
		started: time.Now(),
		limiter: newRateLimiter(cfg.RateLimitPerMinute),
		client:  &http.Client{Timeout: 0},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/info", s.handleInfo)
	mux.HandleFunc("GET /api/v1/openapi.yaml", s.handleOpenAPI)
	mux.HandleFunc("GET /api/v1/playlists", s.admin(s.handleListPlaylists))
	mux.HandleFunc("POST /api/v1/playlists", s.admin(s.handleCreatePlaylist))
	mux.HandleFunc("GET /api/v1/playlists/{id}", s.admin(s.handleGetPlaylist))
	mux.HandleFunc("PATCH /api/v1/playlists/{id}", s.admin(s.handleUpdatePlaylist))
	mux.HandleFunc("DELETE /api/v1/playlists/{id}", s.admin(s.handleDeletePlaylist))
	mux.HandleFunc("POST /api/v1/playlists/{id}/test", s.admin(s.handleTestPlaylist))
	mux.HandleFunc("POST /api/v1/playlists/{id}/sync", s.admin(s.handleSyncPlaylist))
	mux.HandleFunc("GET /api/v1/playlists/{id}/status", s.admin(s.handlePlaylistStatus))
	mux.HandleFunc("GET /api/v1/playlists/{id}/live/categories", s.admin(s.handleLiveCategories))
	mux.HandleFunc("GET /api/v1/playlists/{id}/live/channels", s.admin(s.handleLiveChannels))
	mux.HandleFunc("GET /api/v1/playlists/{id}/movies/categories", s.admin(s.handleMovieCategories))
	mux.HandleFunc("GET /api/v1/playlists/{id}/movies", s.admin(s.handleMovies))
	mux.HandleFunc("GET /api/v1/playlists/{id}/series/categories", s.admin(s.handleSeriesCategories))
	mux.HandleFunc("GET /api/v1/playlists/{id}/series", s.admin(s.handleSeries))
	mux.HandleFunc("GET /api/v1/playlists/{id}/series/{sid}", s.admin(s.handleSeriesDetail))
	mux.HandleFunc("GET /api/v1/playlists/{id}/series/{sid}/seasons", s.admin(s.handleSeasons))
	mux.HandleFunc("GET /api/v1/playlists/{id}/series/{sid}/episodes", s.admin(s.handleEpisodes))
	mux.HandleFunc("GET /api/v1/playlists/{id}/epg", s.admin(s.handleEPG))
	mux.HandleFunc("GET /api/v1/libraries", s.admin(s.handleLibraries))
	mux.HandleFunc("GET /api/v1/libraries/{directory}", s.admin(s.handleLibrary))
	mux.HandleFunc("POST /api/v1/stream/resolve", s.admin(s.handleResolveStream))
	mux.HandleFunc("/assets/", s.handleAssets)
	mux.HandleFunc("/", s.handleStream)
	return s.recoverer(s.cors(s.rateLimit(mux)))
}

func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.validBearer(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "a valid API token is required")
			return
		}
		next(w, r)
	}
}

func (s *Server) validBearer(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	return constantEqual(token, s.cfg.APIToken)
}

func (s *Server) validStreamAuth(r *http.Request) bool {
	if s.validBearer(r) {
		return true
	}
	user, pass, ok := r.BasicAuth()
	return ok && constantEqual(user, s.cfg.RexSourceUser) && constantEqual(pass, s.cfg.RexSourcePassword)
}

func constantEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	code := http.StatusOK
	if err := s.store.DB().PingContext(r.Context()); err != nil {
		status = "error"
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, map[string]any{
		"status":     status,
		"version":    s.cfg.Version,
		"uptime_sec": int(time.Since(s.started).Seconds()),
		"time":       time.Now().UTC(),
	})
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.Stats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_error", "could not read statistics")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":        "rex-xstream",
		"version":     s.cfg.Version,
		"api_version": "v1",
		"features":    []string{"xc-playlists", "live-tv", "movies", "series", "epg", "rex-addon"},
		"rex": map[string]any{
			"enabled":     s.cfg.RexEnabled,
			"source_name": s.cfg.RexSourceName,
			"connected":   s.cfg.RexURL != "",
		},
		"stats": stats,
	})
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "docs/openapi.yaml")
}

func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/assets/")
	if strings.Contains(path, "..") || strings.Contains(path, "/") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, "assets/"+path)
}

func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && !s.limiter.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, origin := range s.cfg.CORSOrigins {
		allowed[origin] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.log.Error("panic in request", "path", r.URL.Path, "panic", recovered)
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func decodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body is not valid JSON")
		return false
	}
	return true
}

func pathID(r *http.Request, name string) (int64, bool) {
	value := r.PathValue(name)
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func queryInt(r *http.Request, name string, fallback, max int) int {
	value := r.URL.Query().Get(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	if parsed > max {
		return max
	}
	return parsed
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type rateLimiter struct {
	limit  int
	mu     sync.Mutex
	counts map[string]*bucket
}

type bucket struct {
	window int64
	count  int
}

func newRateLimiter(limit int) *rateLimiter {
	if limit < 1 {
		limit = 120
	}
	return &rateLimiter{limit: limit, counts: map[string]*bucket{}}
}

func (l *rateLimiter) allow(key string) bool {
	now := time.Now().Unix()
	l.mu.Lock()
	defer l.mu.Unlock()
	current := l.counts[key]
	if current == nil || current.window != now/60 {
		l.counts[key] = &bucket{window: now / 60, count: 1}
		return true
	}
	if current.count >= l.limit {
		return false
	}
	current.count++
	return true
}

var errNotFound = errors.New("not found")
var errDB = sql.ErrNoRows

func contextWithTimeout(r *http.Request, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return context.WithTimeout(r.Context(), timeout)
}

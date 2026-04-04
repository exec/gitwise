package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/config"
)

type Server struct {
	cfg    *config.Config
	router *chi.Mux
	db     *pgxpool.Pool
	http   *http.Server
}

func New(cfg *config.Config, db *pgxpool.Pool) *Server {
	s := &Server{
		cfg:    cfg,
		router: chi.NewRouter(),
		db:     db,
	}
	s.setupMiddleware()
	s.setupRoutes()
	return s
}

func (s *Server) setupMiddleware() {
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Timeout(30 * time.Second))
}

func (s *Server) setupRoutes() {
	s.router.Get("/healthz", s.handleHealth)

	s.router.Route("/api/v1", func(r chi.Router) {
		// Repository routes
		r.Route("/repos", func(r chi.Router) {
			r.Post("/", s.handleNotImplemented)
			r.Get("/{owner}/{repo}", s.handleNotImplemented)
			r.Get("/{owner}/{repo}/tree/{ref}/*", s.handleNotImplemented)
			r.Get("/{owner}/{repo}/blob/{ref}/*", s.handleNotImplemented)
			r.Get("/{owner}/{repo}/commits", s.handleNotImplemented)

			// Pull request routes
			r.Post("/{owner}/{repo}/pulls", s.handleNotImplemented)
			r.Get("/{owner}/{repo}/pulls/{number}", s.handleNotImplemented)
			r.Post("/{owner}/{repo}/pulls/{number}/reviews", s.handleNotImplemented)
			r.Put("/{owner}/{repo}/pulls/{number}/merge", s.handleNotImplemented)

			// Issue routes
			r.Post("/{owner}/{repo}/issues", s.handleNotImplemented)
			r.Get("/{owner}/{repo}/issues/{number}", s.handleNotImplemented)
		})

		// Search
		r.Post("/search", s.handleNotImplemented)
	})
}

func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	s.http = &http.Server{
		Addr:              addr,
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	slog.Info("server starting", "addr", addr)
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok"}`)
}

func (s *Server) handleNotImplemented(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	fmt.Fprintf(w, `{"error":"not implemented"}`)
}

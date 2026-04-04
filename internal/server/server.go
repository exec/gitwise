package server

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/gitwise-io/gitwise/internal/api/handlers"
	"github.com/gitwise-io/gitwise/internal/config"
	"github.com/gitwise-io/gitwise/internal/git"
	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/services/repo"
	"github.com/gitwise-io/gitwise/internal/services/user"
)

type Server struct {
	cfg    *config.Config
	router *chi.Mux
	db     *pgxpool.Pool
	rdb    *redis.Client
	http   *http.Server

	// Services
	userSvc *user.Service
	repoSvc *repo.Service
	gitSvc  *git.Service

	// Middleware
	sessions *middleware.SessionManager
	auth     *middleware.Auth

	// Handlers
	authHandler   *handlers.AuthHandler
	repoHandler   *handlers.RepoHandler
	browseHandler *handlers.BrowseHandler

	// Git protocol
	gitHTTP *git.HTTPHandler
}

func New(cfg *config.Config, db *pgxpool.Pool, rdb *redis.Client) *Server {
	s := &Server{
		cfg:    cfg,
		router: chi.NewRouter(),
		db:     db,
		rdb:    rdb,
	}

	s.initServices()
	s.setupMiddleware()
	s.setupRoutes()

	return s
}

func (s *Server) initServices() {
	// Core services
	s.gitSvc = git.NewService(s.cfg.Git.ReposPath)
	s.userSvc = user.NewService(s.db)
	s.repoSvc = repo.NewService(s.db, s.gitSvc)

	// Auth
	s.sessions = middleware.NewSessionManager(s.rdb)
	s.auth = middleware.NewAuth(s.sessions, s.userSvc)

	// Handlers
	s.authHandler = handlers.NewAuthHandler(s.userSvc, s.sessions)
	s.repoHandler = handlers.NewRepoHandler(s.repoSvc)
	s.browseHandler = handlers.NewBrowseHandler(s.repoSvc, s.gitSvc)

	// Git HTTP protocol
	s.gitHTTP = git.NewHTTPHandler(s.gitSvc, func(username, password string) (string, bool) {
		u, err := s.userSvc.Authenticate(context.Background(), username, password)
		if err != nil {
			// Try API token
			u, err = s.userSvc.ValidateToken(context.Background(), password)
			if err != nil {
				return "", false
			}
		}
		return u.Username, true
	})
}

func (s *Server) setupMiddleware() {
	s.router.Use(chimw.RequestID)
	s.router.Use(chimw.RealIP)
	s.router.Use(chimw.Logger)
	s.router.Use(chimw.Recoverer)
	s.router.Use(corsMiddleware)
	s.router.Use(s.auth.Handler)
}

func (s *Server) setupRoutes() {
	// Health check
	s.router.Get("/healthz", s.handleHealth)

	// Git smart HTTP protocol
	s.router.Handle("/{owner}/{repo}.git/*", s.gitHTTP)

	// API v1
	s.router.Route("/api/v1", func(r chi.Router) {
		// Auth
		r.Post("/auth/register", s.authHandler.Register)
		r.Post("/auth/login", s.authHandler.Login)
		r.Post("/auth/logout", s.authHandler.Logout)
		r.Get("/auth/me", s.authHandler.Me)

		// API tokens (authenticated)
		r.Route("/auth/tokens", func(r chi.Router) {
			r.Use(middleware.RequireAuth)
			r.Post("/", s.authHandler.CreateToken)
			r.Get("/", s.authHandler.ListTokens)
			r.Delete("/{tokenID}", s.authHandler.DeleteToken)
		})

		// User repos (authenticated)
		r.With(middleware.RequireAuth).Get("/user/repos", s.repoHandler.ListMine)

		// Repository operations
		r.Route("/repos", func(r chi.Router) {
			r.With(middleware.RequireAuth).Post("/", s.repoHandler.Create)

			r.Route("/{owner}/{repo}", func(r chi.Router) {
				r.Get("/", s.repoHandler.Get)
				r.With(middleware.RequireAuth).Patch("/", s.repoHandler.Update)
				r.With(middleware.RequireAuth).Delete("/", s.repoHandler.Delete)

				// Code browsing
				r.Get("/tree/{ref}/*", s.browseHandler.GetTree)
				r.Get("/tree/{ref}", s.browseHandler.GetTree) // root tree
				r.Get("/blob/{ref}/*", s.browseHandler.GetBlob)
				r.Get("/raw/{ref}/*", s.browseHandler.GetRawBlob)

				// Commits
				r.Get("/commits", s.browseHandler.ListCommits)
				r.Get("/commits/{sha}", s.browseHandler.GetCommit)

				// Branches
				r.Get("/branches", s.browseHandler.ListBranches)

				// Pull requests (stub)
				r.Post("/pulls", handleNotImplemented)
				r.Get("/pulls/{number}", handleNotImplemented)
				r.Post("/pulls/{number}/reviews", handleNotImplemented)
				r.Put("/pulls/{number}/merge", handleNotImplemented)

				// Issues (stub)
				r.Post("/issues", handleNotImplemented)
				r.Get("/issues/{number}", handleNotImplemented)
			})
		})

		// User profiles
		r.Get("/users/{username}", s.handleGetUser)
		r.Get("/users/{username}/repos", s.handleListUserRepos)

		// Search (stub)
		r.Post("/search", handleNotImplemented)
	})

	// SPA frontend: serve static files from web/dist, fallback to index.html
	s.router.NotFound(s.spaHandler())
}

func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	s.http = &http.Server{
		Addr:              addr,
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
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
	fmt.Fprintf(w, `{"data":{"status":"ok"}}`)
}

func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	u, err := s.userSvc.GetByUsername(r.Context(), username)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"errors":[{"code":"not_found","message":"user not found"}]}`)
		return
	}
	handlers.WriteUserJSON(w, u)
}

func (s *Server) handleListUserRepos(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "username")
	repos, err := s.repoSvc.ListByOwner(r.Context(), owner, middleware.GetUserID(r.Context()), 100)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"errors":[{"code":"server_error","message":"failed to list repos"}]}`)
		return
	}
	handlers.WriteReposJSON(w, repos)
}

// spaHandler serves the React SPA from the frontend dist directory.
// Static assets (JS, CSS, images) are served directly. All other requests
// get index.html so client-side routing works.
func (s *Server) spaHandler() http.HandlerFunc {
	distPath := s.cfg.Frontend.DistPath
	fileServer := http.FileServer(http.Dir(distPath))

	absDistPath, err := filepath.Abs(distPath)
	if err != nil {
		slog.Error("failed to resolve frontend dist path", "path", distPath, "error", err)
		return func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// Don't serve SPA for git protocol or API paths
		path := r.URL.Path
		if strings.HasSuffix(path, ".git") || strings.Contains(path, ".git/") {
			http.NotFound(w, r)
			return
		}

		// Try to serve the file directly (JS, CSS, images, etc.)
		// Resolve and verify the path stays within the dist directory.
		filePath := filepath.Join(absDistPath, filepath.Clean("/"+path))
		if !strings.HasPrefix(filePath, absDistPath+string(filepath.Separator)) && filePath != absDistPath {
			http.NotFound(w, r)
			return
		}
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Fallback: serve index.html for client-side routing
		indexPath := filepath.Join(distPath, "index.html")
		if _, err := fs.Stat(os.DirFS(distPath), "index.html"); err != nil {
			slog.Warn("frontend not built", "path", indexPath, "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"errors":[{"code":"no_frontend","message":"frontend not built — run 'cd web && npm run build'"}]}`)
			return
		}

		http.ServeFile(w, r, indexPath)
	}
}

func handleNotImplemented(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	fmt.Fprintf(w, `{"errors":[{"code":"not_implemented","message":"this endpoint is not yet implemented"}]}`)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			// In production, validate against an allowlist. For self-hosted
			// single-binary deployments, same-origin requests don't send Origin,
			// so this only fires for legitimate cross-origin or dev proxy setups.
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

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
	gossh "github.com/gliderlabs/ssh"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/gitwise-io/gitwise/internal/api/handlers"
	"github.com/gitwise-io/gitwise/internal/config"
	"github.com/gitwise-io/gitwise/internal/git"
	"github.com/gitwise-io/gitwise/internal/middleware"
	"github.com/gitwise-io/gitwise/internal/services/activity"
	"github.com/gitwise-io/gitwise/internal/services/comment"
	"github.com/gitwise-io/gitwise/internal/services/issue"
	"github.com/gitwise-io/gitwise/internal/services/label"
	"github.com/gitwise-io/gitwise/internal/services/milestone"
	"github.com/gitwise-io/gitwise/internal/services/notification"
	"github.com/gitwise-io/gitwise/internal/services/org"
	"github.com/gitwise-io/gitwise/internal/services/protection"
	"github.com/gitwise-io/gitwise/internal/services/pull"
	"github.com/gitwise-io/gitwise/internal/services/repo"
	"github.com/gitwise-io/gitwise/internal/services/review"
	"github.com/gitwise-io/gitwise/internal/services/embedding"
	"github.com/gitwise-io/gitwise/internal/services/commit"
	"github.com/gitwise-io/gitwise/internal/services/oauth"
	"github.com/gitwise-io/gitwise/internal/services/search"
	"github.com/gitwise-io/gitwise/internal/services/sshkey"
	"github.com/gitwise-io/gitwise/internal/services/user"
	"github.com/gitwise-io/gitwise/internal/services/webhook"
	gitwisews "github.com/gitwise-io/gitwise/internal/websocket"
)

type Server struct {
	cfg    *config.Config
	router *chi.Mux
	db     *pgxpool.Pool
	rdb    *redis.Client
	http   *http.Server

	// Services
	userSvc    *user.Service
	repoSvc    *repo.Service
	gitSvc     *git.Service
	issueSvc   *issue.Service
	pullSvc    *pull.Service
	reviewSvc  *review.Service
	commentSvc    *comment.Service
	labelSvc      *label.Service
	milestoneSvc  *milestone.Service
	notifSvc      *notification.Service
	protectionSvc *protection.Service
	activitySvc   *activity.Service
	searchSvc     *search.Service
	orgSvc        *org.Service
	webhookSvc    *webhook.Service
	sshkeySvc     *sshkey.Service
	embeddingSvc  *embedding.Service
	commitIndexer *commit.Indexer

	// WebSocket
	wsHub *gitwisews.Hub

	// Middleware
	sessions *middleware.SessionManager
	auth     *middleware.Auth

	// Handlers
	authHandler   *handlers.AuthHandler
	repoHandler   *handlers.RepoHandler
	browseHandler *handlers.BrowseHandler
	issueHandler  *handlers.IssueHandler
	pullHandler       *handlers.PullHandler
	labelHandler      *handlers.LabelHandler
	milestoneHandler  *handlers.MilestoneHandler
	notifHandler      *handlers.NotificationHandler
	protectionHandler *handlers.ProtectionHandler
	profileHandler    *handlers.ProfileHandler
	activityHandler   *handlers.ActivityHandler
	searchHandler     *handlers.SearchHandler
	orgHandler        *handlers.OrgHandler
	webhookHandler  *handlers.WebhookHandler
	sshkeyHandler   *handlers.SSHKeyHandler

	// Git protocol
	gitHTTP *git.HTTPHandler
	gitSSH  *git.SSHServer
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

	// Phase 2 services
	s.issueSvc = issue.NewService(s.db)
	s.protectionSvc = protection.NewService(s.db)
	s.pullSvc = pull.NewService(s.db, s.gitSvc, s.protectionSvc)
	s.reviewSvc = review.NewService(s.db)
	s.commentSvc = comment.NewService(s.db)
	s.labelSvc = label.NewService(s.db)
	s.milestoneSvc = milestone.NewService(s.db)

	// WebSocket hub (created before notification service for real-time push)
	s.wsHub = gitwisews.NewHub()

	// Notification service (wired to WebSocket hub)
	s.notifSvc = notification.NewService(s.db, s.wsHub)

	// Activity service
	s.activitySvc = activity.NewService(s.db)

	// Webhook service (before handlers that depend on it)
	s.webhookSvc = webhook.NewService(s.db)

	// OAuth service (nil if not configured)
	var oauthSvc *oauth.Service
	if s.cfg.GitHubOAuth.Enabled {
		oauthSvc = oauth.NewService(s.cfg.GitHubOAuth, s.cfg.BaseURL, s.rdb)
		slog.Info("github oauth enabled")
	}

	// Handlers
	s.authHandler = handlers.NewAuthHandler(s.userSvc, s.sessions, oauthSvc)
	s.repoHandler = handlers.NewRepoHandler(s.repoSvc)
	s.browseHandler = handlers.NewBrowseHandler(s.repoSvc, s.gitSvc)
	s.issueHandler = handlers.NewIssueHandler(s.repoSvc, s.issueSvc, s.commentSvc, s.webhookSvc)
	s.pullHandler = handlers.NewPullHandler(s.repoSvc, s.pullSvc, s.reviewSvc, s.commentSvc, s.webhookSvc)
	s.labelHandler = handlers.NewLabelHandler(s.repoSvc, s.labelSvc)
	s.milestoneHandler = handlers.NewMilestoneHandler(s.repoSvc, s.milestoneSvc)
	s.notifHandler = handlers.NewNotificationHandler(s.notifSvc)
	s.protectionHandler = handlers.NewProtectionHandler(s.repoSvc, s.protectionSvc)
	s.profileHandler = handlers.NewProfileHandler(s.userSvc)
	s.activityHandler = handlers.NewActivityHandler(s.repoSvc, s.activitySvc, s.userSvc)

	// Embedding service
	var embProvider embedding.Provider
	if s.cfg.Embedding.Provider == "openai" && s.cfg.Embedding.APIKey != "" {
		embProvider = embedding.NewOpenAIProvider(s.cfg.Embedding.APIKey, s.cfg.Embedding.Model, s.cfg.Embedding.Dimensions)
		slog.Info("embedding provider enabled", "provider", "openai", "model", s.cfg.Embedding.Model)
	}
	s.embeddingSvc = embedding.NewService(s.db, embProvider)

	// Search service
	s.searchSvc = search.NewService(s.db, s.gitSvc)
	s.searchSvc.SetEmbeddingService(s.embeddingSvc)
	s.searchHandler = handlers.NewSearchHandler(s.searchSvc, s.repoSvc)

	// Org service
	s.orgSvc = org.NewService(s.db)
	s.orgHandler = handlers.NewOrgHandler(s.orgSvc)

	s.webhookHandler = handlers.NewWebhookHandler(s.repoSvc, s.webhookSvc)

	// SSH key service + handler
	s.sshkeySvc = sshkey.NewService(s.db)
	s.sshkeyHandler = handlers.NewSSHKeyHandler(s.sshkeySvc)

	// SSH server (git over SSH) — auth via stored SSH keys
	sshAddr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.SSHPort)
	hostKeyPath := filepath.Join(s.cfg.Git.ReposPath, ".ssh_host_ed25519_key")
	s.gitSSH = git.NewSSHServer(s.gitSvc, sshAddr, hostKeyPath,
		func(key gossh.PublicKey) (string, bool) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			fp := sshkey.Fingerprint(key)
			username, err := s.sshkeySvc.LookupByFingerprint(ctx, fp)
			if err != nil {
				return "", false
			}
			return username, true
		},
		func(username, owner, repoName, service string) bool {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return s.checkSSHAccess(ctx, username, owner, repoName, service)
		},
	)

	// Commit indexer
	s.commitIndexer = commit.NewIndexer(s.db, s.gitSvc)

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

	// Index commits after every push
	s.gitHTTP.PostReceiveHook = func(owner, repoName string) {
		var repoID uuid.UUID
		err := s.db.QueryRow(context.Background(), `
			SELECT r.id FROM repositories r
			JOIN users u ON r.owner_id = u.id
			WHERE u.username = $1 AND r.name = $2`, owner, repoName).Scan(&repoID)
		if err != nil {
			slog.Error("post-receive: repo lookup failed", "owner", owner, "repo", repoName, "error", err)
			return
		}
		if _, err := s.commitIndexer.IndexRepo(context.Background(), repoID, owner, repoName); err != nil {
			slog.Error("post-receive: commit indexing failed", "owner", owner, "repo", repoName, "error", err)
		}
	}
}

func (s *Server) setupMiddleware() {
	s.router.Use(chimw.RequestID)
	s.router.Use(chimw.RealIP)
	s.router.Use(chimw.Logger)
	s.router.Use(chimw.Recoverer)
	s.router.Use(corsMiddleware)
	s.router.Use(middleware.MaxBodySize(1 << 30)) // 1 GB body limit (git pushes can be large)
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
		r.Get("/auth/providers", s.authHandler.ListProviders)
		r.Get("/auth/github", s.authHandler.GitHubLogin)
		r.Get("/auth/github/callback", s.authHandler.GitHubCallback)

		// API tokens (authenticated)
		r.Route("/auth/tokens", func(r chi.Router) {
			r.Use(middleware.RequireAuth)
			r.Post("/", s.authHandler.CreateToken)
			r.Get("/", s.authHandler.ListTokens)
			r.Delete("/{tokenID}", s.authHandler.DeleteToken)
		})

		// SSH keys (authenticated)
		r.Route("/user/ssh-keys", func(r chi.Router) {
			r.Use(middleware.RequireAuth)
			r.Post("/", s.sshkeyHandler.Create)
			r.Get("/", s.sshkeyHandler.List)
			r.Delete("/{keyID}", s.sshkeyHandler.Delete)
		})

		// User repos (authenticated)
		r.With(middleware.RequireAuth).Get("/user/repos", s.repoHandler.ListMine)

		// Repository operations
		r.Route("/repos", func(r chi.Router) {
			r.Get("/", s.repoHandler.List)
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
				r.Get("/blame/{ref}/*", s.browseHandler.GetBlame)

				// Commits
				r.Get("/commits", s.browseHandler.ListCommits)
				r.Get("/commits/{sha}", s.browseHandler.GetCommit)

				// Branches
				r.Get("/branches", s.browseHandler.ListBranches)

				// Pull requests
				r.Get("/pulls", s.pullHandler.List)
				r.With(middleware.RequireAuth).Post("/pulls", s.pullHandler.Create)
				r.Route("/pulls/{number}", func(r chi.Router) {
					r.Get("/", s.pullHandler.Get)
					r.With(middleware.RequireAuth).Patch("/", s.pullHandler.Update)
					r.With(middleware.RequireAuth).Put("/merge", s.pullHandler.Merge)
					r.Get("/diff", s.pullHandler.GetDiff)
					r.Get("/reviews", s.pullHandler.ListReviews)
					r.With(middleware.RequireAuth).Post("/reviews", s.pullHandler.CreateReview)
					r.With(middleware.RequireAuth).Put("/threads/{threadID}/resolve", s.pullHandler.ResolveThread)
					r.Get("/comments", s.pullHandler.ListComments)
					r.With(middleware.RequireAuth).Post("/comments", s.pullHandler.CreateComment)
				})

				// Issues
				r.Get("/issues", s.issueHandler.List)
				r.With(middleware.RequireAuth).Post("/issues", s.issueHandler.Create)
				r.Route("/issues/{number}", func(r chi.Router) {
					r.Get("/", s.issueHandler.Get)
					r.With(middleware.RequireAuth).Patch("/", s.issueHandler.Update)
					r.Get("/comments", s.issueHandler.ListComments)
					r.With(middleware.RequireAuth).Post("/comments", s.issueHandler.CreateComment)
				})

				// Labels
				r.Get("/labels", s.labelHandler.List)
				r.With(middleware.RequireAuth).Post("/labels", s.labelHandler.Create)
				r.With(middleware.RequireAuth).Patch("/labels/{labelID}", s.labelHandler.Update)
				r.With(middleware.RequireAuth).Delete("/labels/{labelID}", s.labelHandler.Delete)

				// Milestones
				r.Get("/milestones", s.milestoneHandler.List)
				r.With(middleware.RequireAuth).Post("/milestones", s.milestoneHandler.Create)
				r.With(middleware.RequireAuth).Patch("/milestones/{milestoneID}", s.milestoneHandler.Update)
				r.With(middleware.RequireAuth).Delete("/milestones/{milestoneID}", s.milestoneHandler.Delete)

				// Activity feed
				r.Get("/activity", s.activityHandler.ListByRepo)

				// Branch protection
				r.Get("/branch-protection", s.protectionHandler.List)
				r.With(middleware.RequireAuth).Post("/branch-protection", s.protectionHandler.Create)
				r.With(middleware.RequireAuth).Patch("/branch-protection/{ruleID}", s.protectionHandler.Update)
				r.With(middleware.RequireAuth).Delete("/branch-protection/{ruleID}", s.protectionHandler.Delete)

					// Webhooks
					r.Route("/webhooks", func(r chi.Router) {
						r.Use(middleware.RequireAuth)
						r.Get("/", s.webhookHandler.List)
						r.Post("/", s.webhookHandler.Create)
						r.Get("/{webhookID}", s.webhookHandler.Get)
						r.Patch("/{webhookID}", s.webhookHandler.Update)
						r.Delete("/{webhookID}", s.webhookHandler.Delete)
						r.Get("/{webhookID}/deliveries", s.webhookHandler.ListDeliveries)
						r.Post("/{webhookID}/test", s.webhookHandler.Test)
					})
			})
		})

		// Notifications (authenticated)
		r.Route("/notifications", func(r chi.Router) {
			r.Use(middleware.RequireAuth)
			r.Get("/", s.notifHandler.List)
			r.Post("/read-all", s.notifHandler.MarkAllRead)
			r.Post("/{notifID}/read", s.notifHandler.MarkRead)
		})

		// User profiles
		r.Get("/users/{username}", s.handleGetUser)
		r.Get("/users/{username}/repos", s.handleListUserRepos)
		r.Get("/users/{username}/contributions", s.profileHandler.GetContributions)
		r.Get("/users/{username}/pinned-repos", s.profileHandler.ListPinnedRepos)
		r.Get("/users/{username}/activity", s.activityHandler.ListByUser)

		// Authenticated profile actions
		r.With(middleware.RequireAuth).Patch("/user/profile", s.profileHandler.UpdateProfile)
		r.With(middleware.RequireAuth).Put("/user/pinned-repos", s.profileHandler.SetPinnedRepos)

		// Organizations
		r.Route("/orgs/{name}", func(r chi.Router) {
			r.Get("/", s.orgHandler.Get)
			r.Get("/members", s.orgHandler.ListMembers)
			r.Get("/repos", s.orgHandler.ListRepos)
		})

		// Search
		r.Get("/search", s.searchHandler.Search)
		r.Post("/search", s.searchHandler.Search)
		r.With(middleware.RequireAuth).Post("/search/code/index", s.searchHandler.IndexRepo)

		// Admin: backfill commit index for all repos
		r.With(middleware.RequireAuth).Post("/admin/index-commits", s.handleIndexAllCommits)
	})

	// WebSocket endpoint (authenticated via session cookie or bearer token)
	s.router.Get("/ws", s.wsHub.HandleWS(func(r *http.Request) *uuid.UUID {
		return middleware.GetUserID(r.Context())
	}))

	// SPA frontend: serve static files from web/dist, fallback to index.html
	s.router.NotFound(s.spaHandler())
}

func (s *Server) Start() error {
	// Start SSH server in background
	go func() {
		if err := s.gitSSH.Start(); err != nil {
			slog.Error("SSH server error", "error", err)
		}
	}()

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
	if s.gitSSH != nil {
		if err := s.gitSSH.Shutdown(); err != nil {
			slog.Error("SSH server shutdown error", "error", err)
		}
	}
	return s.http.Shutdown(ctx)
}

// checkSSHAccess verifies that the authenticated SSH user can access the repo.
// For git-upload-pack (read): owner match, public repo, or collaborator.
// For git-receive-pack (write): owner match or collaborator with write/admin role.
func (s *Server) checkSSHAccess(ctx context.Context, username, owner, repoName, service string) bool {
	// Owner always has full access
	if strings.EqualFold(username, owner) {
		return true
	}

	// Look up the repo (checks existence + visibility)
	user, err := s.userSvc.GetByUsername(ctx, username)
	if err != nil {
		return false
	}

	repo, err := s.repoSvc.GetByOwnerAndName(ctx, owner, repoName, &user.ID)
	if err != nil {
		return false
	}

	// Public repos are readable by any authenticated user
	if service == "git-upload-pack" && repo.Visibility == "public" {
		return true
	}

	// Check collaborator role
	var role string
	err = s.db.QueryRow(ctx, `
		SELECT role FROM repo_collaborators
		WHERE repo_id = $1 AND user_id = $2`, repo.ID, user.ID,
	).Scan(&role)
	if err != nil {
		return false
	}

	if service == "git-upload-pack" {
		return true // any collaborator role grants read
	}
	// git-receive-pack requires write or admin
	return role == "write" || role == "admin"
}

// EmbeddingService returns the embedding service for use by the embedding worker.
func (s *Server) EmbeddingService() *embedding.Service {
	return s.embeddingSvc
}

// WebhookService returns the webhook service for use by the retry worker.
func (s *Server) WebhookService() *webhook.Service {
	return s.webhookSvc
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

func (s *Server) handleIndexAllCommits(w http.ResponseWriter, r *http.Request) {
	go func() {
		if err := s.commitIndexer.IndexAll(context.Background()); err != nil {
			slog.Error("commit backfill failed", "error", err)
		}
	}()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"data":{"status":"indexing started"}}`)
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

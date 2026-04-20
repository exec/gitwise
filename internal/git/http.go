package git

import (
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// HTTPHandler handles git smart HTTP protocol requests.
type HTTPHandler struct {
	git *Service
	// authFn resolves HTTP basic auth credentials to a username.
	// Returns ("", false) if auth fails.
	authFn func(username, password string) (string, bool)
	// PostReceiveHook is called asynchronously after a successful git push.
	// Parameters: owner, repoName.
	PostReceiveHook func(owner, repoName string)
	// IsPullMirror returns true when the repo is the destination of a pull mirror,
	// so receive-pack should be rejected. nil = never reject (backwards compatible).
	IsPullMirror func(owner, repoName string) bool
}

func NewHTTPHandler(git *Service, authFn func(string, string) (string, bool)) *HTTPHandler {
	return &HTTPHandler{git: git, authFn: authFn}
}

// ServeHTTP routes git HTTP requests to the appropriate handler.
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Extract owner/repo from path: /{owner}/{repo}.git/...
	parts := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 3)
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	owner := parts[0]
	repoName := strings.TrimSuffix(parts[1], ".git")
	action := parts[2]

	if err := ValidatePath(owner, repoName); err != nil {
		http.NotFound(w, r)
		return
	}

	repoPath := h.git.RepoPath(owner, repoName)
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	switch {
	case action == "info/refs":
		h.handleInfoRefs(w, r, repoPath)
	case action == "git-upload-pack":
		h.handleUploadPack(w, r, repoPath)
	case action == "git-receive-pack":
		h.handleReceivePack(w, r, repoPath, owner)
	default:
		http.NotFound(w, r)
	}
}

func (h *HTTPHandler) handleInfoRefs(w http.ResponseWriter, r *http.Request, repoPath string) {
	serviceName := r.URL.Query().Get("service")
	if serviceName != "git-upload-pack" && serviceName != "git-receive-pack" {
		http.Error(w, "unsupported service", http.StatusBadRequest)
		return
	}

	if serviceName == "git-receive-pack" {
		if !h.requireAuth(w, r) {
			return
		}
	}

	cmd := exec.Command("git", serviceName[4:], "--stateless-rpc", "--advertise-refs", repoPath)
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		slog.Error("git info/refs failed", "error", err, "service", serviceName)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", fmt.Sprintf("application/x-%s-advertisement", serviceName))
	w.Header().Set("Cache-Control", "no-cache")

	// Write pkt-line header
	header := fmt.Sprintf("# service=%s\n", serviceName)
	pktLine := fmt.Sprintf("%04x%s", len(header)+4, header)
	w.Write([]byte(pktLine))
	w.Write([]byte("0000"))
	w.Write(out)
}

func (h *HTTPHandler) handleUploadPack(w http.ResponseWriter, r *http.Request, repoPath string) {
	h.serveGitCommand(w, r, "upload-pack", repoPath)
}

func (h *HTTPHandler) handleReceivePack(w http.ResponseWriter, r *http.Request, repoPath string, owner string) {
	if !h.requireAuth(w, r) {
		return
	}

	// Extract repo name from path for the post-receive hook
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 3)
	repoName := strings.TrimSuffix(parts[1], ".git")

	if h.IsPullMirror != nil && h.IsPullMirror(owner, repoName) {
		slog.Warn("rejected git push to pull-mirror destination",
			"transport", "http", "owner", owner, "repo", repoName)
		http.Error(w,
			"this repo is mirrored from GitHub (read-only on Gitwise). Push to GitHub to update.",
			http.StatusForbidden)
		return
	}

	h.serveGitCommand(w, r, "receive-pack", repoPath)

	if h.PostReceiveHook != nil {
		go h.PostReceiveHook(owner, repoName)
	}
}

func (h *HTTPHandler) serveGitCommand(w http.ResponseWriter, r *http.Request, service, repoPath string) {
	var body io.Reader = r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, "bad gzip", http.StatusBadRequest)
			return
		}
		defer gz.Close()
		body = gz
	}

	cmd := exec.Command("git", service, "--stateless-rpc", repoPath)
	cmd.Stdin = body
	cmd.Stdout = w
	cmd.Stderr = os.Stderr

	w.Header().Set("Content-Type", fmt.Sprintf("application/x-git-%s-result", service))
	w.Header().Set("Cache-Control", "no-cache")

	if err := cmd.Run(); err != nil {
		slog.Error("git command failed", "error", err, "service", service)
		// Can't write HTTP error — headers/body may already be partially written
		return
	}
}

func (h *HTTPHandler) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	username, password, ok := r.BasicAuth()
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="Gitwise"`)
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return false
	}

	if _, valid := h.authFn(username, password); !valid {
		w.Header().Set("WWW-Authenticate", `Basic realm="Gitwise"`)
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return false
	}

	return true
}

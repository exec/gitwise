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

// HTTPReadAccessChecker is called for read operations (clone/fetch/info-refs).
// It receives the authenticated username (empty string = anonymous) plus the
// owner and repo name. Return true to allow, false to deny.
// When nil, all authenticated requests are allowed and anonymous requests are
// denied (same as the old behaviour for receive-pack).
type HTTPReadAccessChecker func(username, owner, repoName string) bool

// HTTPHandler handles git smart HTTP protocol requests.
type HTTPHandler struct {
	git *Service
	// authFn resolves HTTP basic auth credentials to a username.
	// Returns ("", false) if auth fails.
	authFn func(username, password string) (string, bool)
	// PostReceiveHook is called asynchronously after a successful git push.
	// Parameters: owner, repoName.
	PostReceiveHook func(owner, repoName string)
	// PreReceiveHook, when set, is called before git-receive-pack executes.
	// It receives the pushing username, the owner/repo names, the ref being
	// updated, and the old/new SHAs. Return a non-nil error to reject the push.
	// The error message is sent to the client via the git side-band protocol.
	PreReceiveHook func(username, owner, repoName, ref, oldSHA, newSHA string) error
	// CheckReadAccess, when set, is called for upload-pack / info-refs with
	// service=git-upload-pack. It gates read access: if nil, anonymous clones
	// of public repos are permitted (backwards-compatible). If the function
	// returns false, a 401/404 is sent to the client.
	CheckReadAccess HTTPReadAccessChecker
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
		h.handleInfoRefs(w, r, owner, repoName, repoPath)
	case action == "git-upload-pack":
		h.handleUploadPack(w, r, owner, repoName, repoPath)
	case action == "git-receive-pack":
		h.handleReceivePack(w, r, repoPath, owner, repoName)
	default:
		http.NotFound(w, r)
	}
}

// resolveOptionalAuth attempts HTTP Basic auth without requiring it.
// Returns ("", false) when no credentials are provided (anonymous).
// Returns ("", false) when credentials are provided but invalid.
// Returns (username, true) on success.
func (h *HTTPHandler) resolveOptionalAuth(r *http.Request) (string, bool) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return "", false
	}
	u, valid := h.authFn(username, password)
	if !valid {
		return "", false
	}
	return u, true
}

func (h *HTTPHandler) handleInfoRefs(w http.ResponseWriter, r *http.Request, owner, repoName, repoPath string) {
	serviceName := r.URL.Query().Get("service")
	if serviceName != "git-upload-pack" && serviceName != "git-receive-pack" {
		http.Error(w, "unsupported service", http.StatusBadRequest)
		return
	}

	if serviceName == "git-receive-pack" {
		// Write operations always require authentication.
		if !h.requireAuth(w, r) {
			return
		}
	} else {
		// git-upload-pack (read): enforce visibility via CheckReadAccess.
		username, _ := h.resolveOptionalAuth(r)
		if !h.checkRead(w, r, username, owner, repoName) {
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

func (h *HTTPHandler) handleUploadPack(w http.ResponseWriter, r *http.Request, owner, repoName, repoPath string) {
	// Enforce read-access visibility on the upload-pack POST as well.
	username, _ := h.resolveOptionalAuth(r)
	if !h.checkRead(w, r, username, owner, repoName) {
		return
	}
	h.serveGitCommand(w, r, "upload-pack", repoPath)
}

func (h *HTTPHandler) handleReceivePack(w http.ResponseWriter, r *http.Request, repoPath, owner, repoName string) {
	username, password, ok := r.BasicAuth()
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="Gitwise"`)
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	authedUser, valid := h.authFn(username, password)
	if !valid {
		w.Header().Set("WWW-Authenticate", `Basic realm="Gitwise"`)
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if h.IsPullMirror != nil && h.IsPullMirror(owner, repoName) {
		slog.Warn("rejected git push to pull-mirror destination",
			"transport", "http", "owner", owner, "repo", repoName)
		http.Error(w,
			"this repo is mirrored from GitHub (read-only on Gitwise). Push to GitHub to update.",
			http.StatusForbidden)
		return
	}

	// Pre-receive branch-protection check.
	// Approach: parse the pkt-line ref-update commands from the request body
	// before handing off to git. If any ref is protected, reject upfront with a
	// plain HTTP 403 containing the protection error. This avoids needing a
	// hook script on disk and works cleanly with stateless-rpc.
	//
	// The client sends a sequence of pkt-lines in the form:
	//   <old-sha> SP <new-sha> SP refs/heads/<branch> NUL cap1 cap2…
	// followed by a flush packet (0000). We read them all, check each ref
	// against the hook, then replay the bytes to git.
	if h.PreReceiveHook != nil {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusInternalServerError)
			return
		}

		if refErr := h.checkPreReceive(authedUser, owner, repoName, body); refErr != nil {
			slog.Info("pre-receive hook rejected push",
				"user", authedUser, "owner", owner, "repo", repoName, "reason", refErr)
			// Reply with a git-receive-pack error response so git CLI shows the message.
			writePktError(w, refErr.Error())
			return
		}

		// Replace the body with the already-consumed bytes so serveGitCommand can read them.
		r.Body = io.NopCloser(strings.NewReader(string(body)))
	}

	h.serveGitCommand(w, r, "receive-pack", repoPath)

	if h.PostReceiveHook != nil {
		go h.PostReceiveHook(owner, repoName)
	}
}

// checkRead enforces read-access policy. Returns true when the request is allowed.
// If CheckReadAccess is nil, all requests are permitted (backwards-compatible with
// older wiring that predates the visibility fix).
func (h *HTTPHandler) checkRead(w http.ResponseWriter, r *http.Request, username, owner, repoName string) bool {
	if h.CheckReadAccess == nil {
		// No access checker configured — fall back to open (pre-fix behaviour).
		return true
	}
	if h.CheckReadAccess(username, owner, repoName) {
		return true
	}
	// Distinguish: if the request carries credentials they were bad (or lack perms),
	// otherwise prompt for them. Use 401 in both cases to avoid repo enumeration.
	w.Header().Set("WWW-Authenticate", `Basic realm="Gitwise"`)
	http.Error(w, "repository not found", http.StatusUnauthorized)
	return false
}

// checkPreReceive parses pkt-lines from the receive-pack body and calls
// PreReceiveHook for each ref update. Returns the first rejection error.
func (h *HTTPHandler) checkPreReceive(username, owner, repoName string, body []byte) error {
	data := string(body)
	for len(data) > 0 {
		if len(data) < 4 {
			break
		}
		// Parse pkt-line length prefix (4 hex bytes).
		var length int
		_, err := fmt.Sscanf(data[:4], "%04x", &length)
		if err != nil {
			break
		}
		if length == 0 {
			// Flush packet — end of ref-update commands.
			break
		}
		if length < 4 || length > len(data) {
			break
		}
		line := data[4:length]
		data = data[length:]

		// Strip trailing newline and NUL-delimited capabilities.
		if idx := strings.IndexByte(line, 0); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimRight(line, "\n")

		// Each line: "<old-sha> <new-sha> <ref>"
		fields := strings.SplitN(line, " ", 3)
		if len(fields) != 3 {
			continue
		}
		oldSHA, newSHA, ref := fields[0], fields[1], fields[2]

		// Only check branch refs.
		if !strings.HasPrefix(ref, "refs/heads/") {
			continue
		}
		branchName := strings.TrimPrefix(ref, "refs/heads/")

		if err := h.PreReceiveHook(username, owner, repoName, branchName, oldSHA, newSHA); err != nil {
			return err
		}
	}
	return nil
}

// writePktError writes a minimal git-receive-pack error response so the git
// client displays a human-readable rejection message. We write the unpack-status
// and a band-2 (error) sideband message, then flush.
func writePktError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusForbidden)

	// unpack ok
	unpack := "unpack ok\n"
	fmt.Fprintf(w, "%04x%s", len(unpack)+4, unpack)

	// ng ref error
	ngLine := fmt.Sprintf("ng refs/heads/protected %s\n", msg)
	fmt.Fprintf(w, "%04x%s", len(ngLine)+4, ngLine)

	// flush
	fmt.Fprint(w, "0000")
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

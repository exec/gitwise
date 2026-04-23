package git

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestRepo creates an initialised bare repo under tmpDir and returns the
// HTTPHandler wired against it.
func setupTestRepo(t *testing.T, owner, name string) (svc *Service, tmpDir string) {
	t.Helper()
	tmpDir = t.TempDir()
	svc = NewService(tmpDir)

	if err := svc.InitBare(owner, name); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if err := svc.AutoInit(owner, name, "main"); err != nil {
		t.Fatalf("AutoInit: %v", err)
	}
	return svc, tmpDir
}

// newHandler creates an HTTPHandler with a simple always-pass auth function.
func newHandler(svc *Service) *HTTPHandler {
	return NewHTTPHandler(svc, func(u, p string) (string, bool) {
		if u == "alice" && p == "secret" {
			return "alice", true
		}
		return "", false
	})
}

func TestHandleInfoRefs_PrivateRepoRequiresAuth(t *testing.T) {
	svc, _ := setupTestRepo(t, "alice", "private-repo")
	h := newHandler(svc)

	// Simulate a private repo: CheckReadAccess always denies anonymous.
	h.CheckReadAccess = func(username, owner, repoName string) bool {
		if username == "" {
			return false // anonymous = deny
		}
		return username == "alice"
	}

	tests := []struct {
		name       string
		auth       string // "user:pass" or ""
		wantStatus int
	}{
		{"anonymous denied", "", http.StatusUnauthorized},
		{"wrong creds denied", "bob:wrong", http.StatusUnauthorized},
		{"valid creds allowed", "alice:secret", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/alice/private-repo.git/info/refs?service=git-upload-pack", nil)
			if tt.auth != "" {
				parts := strings.SplitN(tt.auth, ":", 2)
				req.SetBasicAuth(parts[0], parts[1])
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rr.Code, tt.wantStatus, rr.Body.String())
			}
		})
	}
}

func TestHandleInfoRefs_PublicRepoAllowsAnonymous(t *testing.T) {
	svc, _ := setupTestRepo(t, "alice", "public-repo")
	h := newHandler(svc)

	// Public repo: CheckReadAccess always allows.
	h.CheckReadAccess = func(username, owner, repoName string) bool {
		return true
	}

	req := httptest.NewRequest("GET", "/alice/public-repo.git/info/refs?service=git-upload-pack", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for public repo, got %d", rr.Code)
	}
}

func TestHandleInfoRefs_NilCheckReadAccess_AllowsAll(t *testing.T) {
	svc, _ := setupTestRepo(t, "alice", "nochecker-repo")
	h := newHandler(svc)
	// CheckReadAccess is nil — backwards-compatible: allow all reads.

	req := httptest.NewRequest("GET", "/alice/nochecker-repo.git/info/refs?service=git-upload-pack", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 when CheckReadAccess is nil, got %d", rr.Code)
	}
}

func TestHandleInfoRefs_ReceivePack_RequiresAuth(t *testing.T) {
	svc, _ := setupTestRepo(t, "alice", "push-repo")
	h := newHandler(svc)

	// Anonymous request for receive-pack should get 401.
	req := httptest.NewRequest("GET", "/alice/push-repo.git/info/refs?service=git-receive-pack", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated receive-pack info-refs, got %d", rr.Code)
	}
}

func TestHandleInfoRefs_PathTraversal(t *testing.T) {
	svc, _ := setupTestRepo(t, "alice", "repo")
	h := newHandler(svc)

	req := httptest.NewRequest("GET", "/../etc/passwd.git/info/refs?service=git-upload-pack", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for path traversal, got %d", rr.Code)
	}
}

func TestHandleInfoRefs_NonExistentRepo(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)
	h := newHandler(svc)

	req := httptest.NewRequest("GET", "/alice/nonexistent.git/info/refs?service=git-upload-pack", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent repo, got %d", rr.Code)
	}
}

func TestCheckPreReceive_ProtectedBranchRejected(t *testing.T) {
	svc, _ := setupTestRepo(t, "alice", "repo")
	h := newHandler(svc)

	h.PreReceiveHook = func(username, owner, repoName, branch, oldSHA, newSHA string) error {
		if branch == "main" {
			return fmt.Errorf("branch 'main' is protected")
		}
		return nil
	}

	// Build a fake pkt-line body for pushing to refs/heads/main.
	// Format: <4-hex-len><old-sha> <new-sha> refs/heads/main\n
	oldSHA := strings.Repeat("0", 40)
	newSHA := strings.Repeat("a", 40)
	line := fmt.Sprintf("%s %s refs/heads/main\n", oldSHA, newSHA)
	pktLine := fmt.Sprintf("%04x%s0000", len(line)+4, line)

	body := strings.NewReader(pktLine)

	req := httptest.NewRequest("POST", "/alice/repo.git/git-receive-pack", body)
	req.SetBasicAuth("alice", "secret")
	req.Header.Set("Content-Type", "application/x-git-receive-pack-request")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	// The hook should reject with 403 Forbidden.
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for protected branch push, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestCheckPreReceive_UnprotectedBranchAllowed(t *testing.T) {
	svc, tmpDir := setupTestRepo(t, "alice", "repo")
	h := newHandler(svc)

	h.PreReceiveHook = func(username, owner, repoName, branch, oldSHA, newSHA string) error {
		if branch == "main" {
			return fmt.Errorf("branch 'main' is protected")
		}
		return nil // feature branches are allowed
	}

	repoPath := filepath.Join(tmpDir, "alice", "repo.git")
	// Stat to make sure the repo exists.
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		t.Fatalf("test repo not created at %s", repoPath)
	}

	// Build pkt-line for pushing to refs/heads/feature — should pass the hook.
	oldSHA := strings.Repeat("0", 40)
	newSHA := strings.Repeat("b", 40)
	line := fmt.Sprintf("%s %s refs/heads/feature\n", oldSHA, newSHA)
	pktLine := fmt.Sprintf("%04x%s0000", len(line)+4, line)

	body := strings.NewReader(pktLine)
	req := httptest.NewRequest("POST", "/alice/repo.git/git-receive-pack", body)
	req.SetBasicAuth("alice", "secret")
	req.Header.Set("Content-Type", "application/x-git-receive-pack-request")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	// The hook passes; git itself may fail (SHA doesn't exist) but the HTTP
	// layer should not return 403.
	if rr.Code == http.StatusForbidden {
		t.Errorf("expected non-403 for unprotected branch, got 403 (body: %s)", rr.Body.String())
	}
}

func TestCheckPreReceive_ParsePktLines(t *testing.T) {
	h := &HTTPHandler{
		PreReceiveHook: func(username, owner, repoName, branch, oldSHA, newSHA string) error {
			if branch == "protected" {
				return fmt.Errorf("protected")
			}
			return nil
		},
	}

	tests := []struct {
		name      string
		ref       string
		wantError bool
	}{
		{"protected branch", "refs/heads/protected", true},
		{"normal branch", "refs/heads/feature", false},
		{"tag ref skipped", "refs/tags/v1.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := strings.Repeat("0", 40)
			new_ := strings.Repeat("1", 40)
			line := fmt.Sprintf("%s %s %s\n", old, new_, tt.ref)
			pktLine := fmt.Sprintf("%04x%s0000", len(line)+4, line)

			err := h.checkPreReceive("alice", "owner", "repo", []byte(pktLine))
			if (err != nil) != tt.wantError {
				t.Errorf("checkPreReceive error = %v, wantError = %v", err, tt.wantError)
			}
		})
	}
}

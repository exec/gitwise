package git

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	gossh "github.com/gliderlabs/ssh"
	cryptossh "golang.org/x/crypto/ssh"
)

// SSHAccessChecker verifies whether a user has access to a repository.
// service is "git-upload-pack" (read) or "git-receive-pack" (write).
type SSHAccessChecker func(username, owner, repoName, service string) bool

type SSHServer struct {
	git         *Service
	addr        string
	hostKeyPath string
	// authFn validates an SSH public key and returns the username.
	authFn func(key gossh.PublicKey) (string, bool)
	// accessCheck verifies repo-level authorization for the authenticated user.
	accessCheck SSHAccessChecker
	// IsPullMirror returns true when the repo is the destination of a pull mirror,
	// so receive-pack should be rejected. nil = never reject (backwards compatible).
	IsPullMirror func(owner, repoName string) bool
	server       *gossh.Server
}

func NewSSHServer(gitSvc *Service, addr, hostKeyPath string, authFn func(gossh.PublicKey) (string, bool), accessCheck SSHAccessChecker) *SSHServer {
	return &SSHServer{
		git:         gitSvc,
		addr:        addr,
		hostKeyPath: hostKeyPath,
		authFn:      authFn,
		accessCheck: accessCheck,
	}
}

func (s *SSHServer) Start() error {
	s.server = &gossh.Server{
		Addr: s.addr,
		Handler: func(sess gossh.Session) {
			s.handleSession(sess)
		},
		PublicKeyHandler: func(ctx gossh.Context, key gossh.PublicKey) bool {
			username, ok := s.authFn(key)
			if ok {
				ctx.SetValue("username", username)
			}
			return ok
		},
	}

	// Load or generate persistent host key
	if err := s.loadOrGenerateHostKey(); err != nil {
		return fmt.Errorf("ssh host key: %w", err)
	}

	slog.Info("SSH server starting", "addr", s.addr)
	return s.server.ListenAndServe()
}

func (s *SSHServer) Shutdown() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

func (s *SSHServer) handleSession(sess gossh.Session) {
	rawCmd := sess.RawCommand()
	if rawCmd == "" {
		fmt.Fprintln(sess, "Hi! You've connected to Gitwise via SSH. Interactive shell is not supported.")
		sess.Exit(0)
		return
	}

	// Parse: git-upload-pack '/owner/repo.git' or git-receive-pack '/owner/repo.git'
	parts := strings.SplitN(rawCmd, " ", 2)
	if len(parts) != 2 {
		fmt.Fprintln(sess.Stderr(), "invalid command")
		sess.Exit(1)
		return
	}

	service := parts[0]
	if service != "git-upload-pack" && service != "git-receive-pack" {
		fmt.Fprintln(sess.Stderr(), "unsupported command: "+service)
		sess.Exit(1)
		return
	}

	repoArg := strings.Trim(parts[1], "'\"")
	repoArg = strings.TrimPrefix(repoArg, "/")
	repoArg = strings.TrimSuffix(repoArg, ".git")

	repoParts := strings.SplitN(repoArg, "/", 2)
	if len(repoParts) != 2 {
		fmt.Fprintln(sess.Stderr(), "invalid repository path")
		sess.Exit(1)
		return
	}

	owner := repoParts[0]
	repoName := repoParts[1]

	if err := ValidatePath(owner, repoName); err != nil {
		fmt.Fprintln(sess.Stderr(), "invalid repository path")
		sess.Exit(1)
		return
	}

	// Authorization: verify the authenticated user can access this repo
	username, _ := sess.Context().Value("username").(string)
	if username == "" {
		fmt.Fprintln(sess.Stderr(), "authentication required")
		sess.Exit(1)
		return
	}

	if s.accessCheck != nil && !s.accessCheck(username, owner, repoName, service) {
		fmt.Fprintln(sess.Stderr(), "repository not found")
		sess.Exit(1)
		return
	}

	repoPath := s.git.RepoPath(owner, repoName)

	if service == "git-receive-pack" && s.IsPullMirror != nil && s.IsPullMirror(owner, repoName) {
		fmt.Fprintln(sess.Stderr(), "error: this repo is mirrored from GitHub (read-only on Gitwise). Push to GitHub to update.")
		sess.Exit(1)
		return
	}

	cmd := exec.Command("git", service[4:], repoPath) // strip "git-" prefix
	cmd.Stdin = sess
	cmd.Stdout = sess
	cmd.Stderr = sess.Stderr()

	if err := cmd.Run(); err != nil {
		slog.Error("SSH git command failed", "error", err, "service", service, "repo", owner+"/"+repoName)
		sess.Exit(1)
		return
	}

	sess.Exit(0)
}

// loadOrGenerateHostKey loads an existing ED25519 host key from disk or
// generates one on first start so SSH clients don't see host key changes.
func (s *SSHServer) loadOrGenerateHostKey() error {
	if s.hostKeyPath == "" {
		return nil // ephemeral key (testing only)
	}

	// Try to load existing key
	if keyData, err := os.ReadFile(s.hostKeyPath); err == nil {
		signer, err := cryptossh.ParsePrivateKey(keyData)
		if err != nil {
			return fmt.Errorf("parse host key: %w", err)
		}
		s.server.AddHostKey(signer)
		slog.Info("SSH host key loaded", "path", s.hostKeyPath)
		return nil
	}

	// Generate new ED25519 key
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate host key: %w", err)
	}

	privBytes, err := cryptossh.MarshalPrivateKey(priv, "")
	if err != nil {
		return fmt.Errorf("marshal host key: %w", err)
	}

	keyPEM := pem.EncodeToMemory(privBytes)
	if err := os.WriteFile(s.hostKeyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("write host key: %w", err)
	}

	signer, err := cryptossh.ParsePrivateKey(keyPEM)
	if err != nil {
		return fmt.Errorf("parse generated key: %w", err)
	}
	s.server.AddHostKey(signer)
	slog.Info("SSH host key generated", "path", s.hostKeyPath)
	return nil
}

// Ensure gossh.Session satisfies io.ReadWriter for the compiler
var _ io.ReadWriter = (gossh.Session)(nil)

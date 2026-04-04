package git

import (
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"

	gossh "github.com/gliderlabs/ssh"
)

type SSHServer struct {
	git      *Service
	addr     string
	hostKeys []string
	// authFn validates an SSH public key and returns the username.
	authFn func(key gossh.PublicKey) (string, bool)
}

func NewSSHServer(gitSvc *Service, addr string, authFn func(gossh.PublicKey) (string, bool)) *SSHServer {
	return &SSHServer{
		git:    gitSvc,
		addr:   addr,
		authFn: authFn,
	}
}

func (s *SSHServer) Start() error {
	server := &gossh.Server{
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

	slog.Info("SSH server starting", "addr", s.addr)
	return server.ListenAndServe()
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

	repoPath := s.git.RepoPath(owner, repoName)

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

// Ensure gossh.Session satisfies io.ReadWriter for the compiler
var _ io.ReadWriter = (gossh.Session)(nil)

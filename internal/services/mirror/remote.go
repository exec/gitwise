package mirror

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type Remote struct{}

func NewRemote() *Remote { return &Remote{} }

type SyncOutcome struct {
	RefsChanged int
	Stderr      string
}

// PushMirror runs: git --git-dir=<local> push --mirror --force --prune <remoteURL>.
// If pat is non-empty, an ephemeral GIT_ASKPASS helper supplies it over HTTPS.
func (r *Remote) PushMirror(ctx context.Context, localPath, remoteURL, pat string) (SyncOutcome, error) {
	args := []string{"--git-dir=" + localPath, "push", "--mirror", "--force", "--prune", remoteURL}
	return r.runGit(ctx, args, pat)
}

// FetchMirror runs: git --git-dir=<local> fetch --prune --force <remoteURL> refspecs.
func (r *Remote) FetchMirror(ctx context.Context, localPath, remoteURL, pat string) (SyncOutcome, error) {
	args := []string{
		"--git-dir=" + localPath, "fetch", "--prune", "--force", remoteURL,
		"+refs/heads/*:refs/heads/*",
		"+refs/tags/*:refs/tags/*",
	}
	return r.runGit(ctx, args, pat)
}

// LsRemoteDefault returns the default branch name of the remote via ls-remote --symref.
func (r *Remote) LsRemoteDefault(ctx context.Context, remoteURL, pat string) (string, error) {
	args := []string{"ls-remote", "--symref", remoteURL, "HEAD"}
	out, err := r.runGitCombined(ctx, args, pat)
	if err != nil {
		return "", err
	}
	// Output first line: "ref: refs/heads/main\tHEAD"
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "ref: ") {
			ref := strings.SplitN(line[len("ref: "):], "\t", 2)[0]
			return strings.TrimPrefix(ref, "refs/heads/"), nil
		}
	}
	return "main", nil
}

// SetHead runs: git --git-dir=<local> symbolic-ref HEAD refs/heads/<branch>.
func (r *Remote) SetHead(ctx context.Context, localPath, branch string) error {
	_, err := r.runGit(ctx, []string{"--git-dir=" + localPath, "symbolic-ref", "HEAD", "refs/heads/" + branch}, "")
	return err
}

// InitBare runs: git init --bare <path>.
func (r *Remote) InitBare(ctx context.Context, localPath string) error {
	_, err := r.runGit(ctx, []string{"init", "--bare", localPath}, "")
	return err
}

func (r *Remote) runGit(ctx context.Context, args []string, pat string) (SyncOutcome, error) {
	stderrBuf, askpassDir, env, err := r.prepareEnv(pat)
	if err != nil {
		return SyncOutcome{}, err
	}
	if askpassDir != "" {
		defer os.RemoveAll(askpassDir)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = env
	cmd.Stderr = stderrBuf

	runErr := cmd.Run()
	out := SyncOutcome{
		Stderr:      stderrBuf.String(),
		RefsChanged: countRefChanges(stderrBuf.String()),
	}
	if runErr != nil {
		return out, fmt.Errorf("git %s: %w (stderr: %s)", strings.Join(args, " "), runErr, out.Stderr)
	}
	return out, nil
}

func (r *Remote) runGitCombined(ctx context.Context, args []string, pat string) (string, error) {
	_, askpassDir, env, err := r.prepareEnv(pat)
	if err != nil {
		return "", err
	}
	if askpassDir != "" {
		defer os.RemoveAll(askpassDir)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (r *Remote) prepareEnv(pat string) (*bytes.Buffer, string, []string, error) {
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var askpassDir string
	if pat != "" {
		dir, err := os.MkdirTemp("", "gitwise-askpass-*")
		if err != nil {
			return nil, "", nil, fmt.Errorf("mirror remote: mktemp: %w", err)
		}
		askpass := filepath.Join(dir, "askpass.sh")
		script := "#!/bin/sh\nexec printenv GITWISE_MIRROR_PAT\n"
		if err := os.WriteFile(askpass, []byte(script), 0o700); err != nil {
			os.RemoveAll(dir)
			return nil, "", nil, fmt.Errorf("mirror remote: write askpass: %w", err)
		}
		env = append(env,
			"GIT_ASKPASS="+askpass,
			"GITWISE_MIRROR_PAT="+pat,
		)
		askpassDir = dir
	}
	return &bytes.Buffer{}, askpassDir, env, nil
}

// countRefChanges counts ref-update lines in git's stderr output.
// Git push/fetch emit lines like:
//   " * [new branch]      main -> main"
//   " * [new tag]         v1.0 -> v1.0"
//   " - [deleted]               (none) -> refs/heads/old"
//   " + abc1234...def5678 branch -> branch"
//   "   abc1234..def5678  branch -> branch"
// We match any non-empty line that contains " -> " (with spaces), which is
// the canonical separator git uses for all ref-update status lines.
var refLineRe = regexp.MustCompile(`(?m)^\s+\S.*\s->\s`)

func countRefChanges(stderr string) int {
	if stderr == "" {
		return 0
	}
	return len(refLineRe.FindAllString(stderr, -1))
}

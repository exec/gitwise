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
		return "", fmt.Errorf("git ls-remote %s: %w (output: %s)", remoteURL, err, out)
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
	parent := os.Environ()
	env := make([]string, 0, len(parent)+3)
	for _, e := range parent {
		// Strip any pre-existing GITWISE_MIRROR_PAT from the parent env so a
		// misconfigured deployment can't leak a stale token into git's child.
		if strings.HasPrefix(e, "GITWISE_MIRROR_PAT=") {
			continue
		}
		env = append(env, e)
	}
	env = append(env, "GIT_TERMINAL_PROMPT=0")
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
// Ref-update lines are indented and contain " -> " (the local -> remote
// ref separator git uses for push and fetch). Example shapes:
//   " * [new branch]      main -> main"
//   " * [new tag]         v1.0 -> v1.0"
//   " - [deleted]                (none) -> refs/heads/old"
//   " + abc1234...def5678 branch -> branch"
//   "   abc1234..def5678  branch -> branch"    (fast-forward)
//   " = [up to date]      main -> main"        (NO change, excluded)
// We match the " -> " shape broadly and then drop "up to date" entries
// so no-op syncs report 0 refs changed.
var refLineRe = regexp.MustCompile(`(?m)^\s+\S.*\s->\s`)
var upToDateRe = regexp.MustCompile(`\[up to date\]`)

func countRefChanges(stderr string) int {
	if stderr == "" {
		return 0
	}
	count := 0
	for _, m := range refLineRe.FindAllString(stderr, -1) {
		if !upToDateRe.MatchString(m) {
			count++
		}
	}
	return count
}

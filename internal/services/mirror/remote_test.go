package mirror

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// installFakeGit writes a shell script named "git" to a tempdir and prepends
// that dir to PATH for the duration of the test. The script writes its argv
// and selected env vars to files the test can inspect, then exits 0 with a
// canned stderr.
func installFakeGit(t *testing.T, stderr string) (dir, argvPath, envPath string) {
	t.Helper()
	dir = t.TempDir()
	argvPath = filepath.Join(dir, "argv")
	envPath = filepath.Join(dir, "env")
	script := `#!/bin/sh
printf '%s\n' "$@" > ` + argvPath + `
{ echo "GIT_ASKPASS=$GIT_ASKPASS"; echo "GITWISE_MIRROR_PAT=$GITWISE_MIRROR_PAT"; } > ` + envPath + `
printf '%s' ` + "'" + stderr + "'" + ` >&2
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir, argvPath, envPath
}

func TestPushMirrorInvokesGitWithExpectedArgs(t *testing.T) {
	_, argvPath, envPath := installFakeGit(t, " * [new branch]      main -> main\n")
	r := NewRemote()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := r.PushMirror(ctx, "/tmp/local.git", "https://github.com/o/r.git", "ghp_secret")
	if err != nil {
		t.Fatalf("PushMirror: %v", err)
	}
	if result.RefsChanged != 1 {
		t.Errorf("RefsChanged = %d, want 1", result.RefsChanged)
	}

	argv, _ := os.ReadFile(argvPath)
	for _, want := range []string{"--git-dir=/tmp/local.git", "push", "--mirror", "--force", "--prune",
		"https://github.com/o/r.git"} {
		if !strings.Contains(string(argv), want) {
			t.Errorf("argv missing %q:\n%s", want, argv)
		}
	}
	if strings.Contains(string(argv), "ghp_secret") {
		t.Fatal("PAT leaked into argv")
	}

	env, _ := os.ReadFile(envPath)
	if !strings.Contains(string(env), "GITWISE_MIRROR_PAT=ghp_secret") {
		t.Errorf("PAT not in child env:\n%s", env)
	}
	if !strings.Contains(string(env), "GIT_ASKPASS=") {
		t.Errorf("GIT_ASKPASS not set:\n%s", env)
	}
}

func TestFetchMirrorParsesRefsChanged(t *testing.T) {
	stderr := ` * [new branch]      main         -> main
 * [new branch]      develop      -> develop
   abc1234..def5678  feature/foo  -> feature/foo
`
	installFakeGit(t, stderr)
	r := NewRemote()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := r.FetchMirror(ctx, "/tmp/local.git", "https://github.com/o/r.git", "")
	if err != nil {
		t.Fatalf("FetchMirror: %v", err)
	}
	if result.RefsChanged != 3 {
		t.Errorf("RefsChanged = %d, want 3", result.RefsChanged)
	}
}

func TestFetchMirrorSkipsAskpassWhenNoPAT(t *testing.T) {
	_, _, envPath := installFakeGit(t, "")
	r := NewRemote()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = r.FetchMirror(ctx, "/tmp/local.git", "https://github.com/o/r.git", "")

	env, _ := os.ReadFile(envPath)
	if strings.Contains(string(env), "GIT_ASKPASS=/") {
		t.Errorf("GIT_ASKPASS should be unset for empty PAT:\n%s", env)
	}
}

func TestRemoteReturnsErrorOnNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho boom >&2\nexit 128\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	r := NewRemote()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.PushMirror(ctx, "/tmp/local.git", "https://github.com/o/r.git", "")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected error containing stderr, got %v", err)
	}

	// Sanity: exec.ExitError surfaces
	if _, ok := err.(*exec.ExitError); ok {
		t.Log("got ExitError")
	}
}

func TestFetchMirrorDoesNotCountUpToDateLines(t *testing.T) {
	stderr := ` = [up to date]      main        -> main
 = [up to date]      develop     -> develop
 * [new branch]      feature     -> feature
`
	installFakeGit(t, stderr)
	r := NewRemote()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := r.FetchMirror(ctx, "/tmp/local.git", "https://github.com/o/r.git", "")
	if err != nil {
		t.Fatalf("FetchMirror: %v", err)
	}
	if result.RefsChanged != 1 {
		t.Errorf("RefsChanged = %d, want 1 (only the new branch)", result.RefsChanged)
	}
}

func TestLsRemoteDefaultParsesSymref(t *testing.T) {
	dir := t.TempDir()
	// Fake git that emits a ls-remote --symref style response.
	script := "#!/bin/sh\nprintf 'ref: refs/heads/trunk\\tHEAD\\nabc123\\tHEAD\\n'\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	r := NewRemote()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	branch, err := r.LsRemoteDefault(ctx, "https://github.com/o/r.git", "")
	if err != nil {
		t.Fatalf("LsRemoteDefault: %v", err)
	}
	if branch != "trunk" {
		t.Errorf("branch = %q, want %q", branch, "trunk")
	}
}

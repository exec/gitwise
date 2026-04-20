//go:build integration

package mirror

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIntegration_PushMirror_PopulatesRemote(t *testing.T) {
	source := t.TempDir()
	dest := t.TempDir()
	srcPath := filepath.Join(source, "src.git")
	dstPath := filepath.Join(dest, "dst.git")

	mustRun(t, "git", "init", "--bare", "-b", "main", srcPath)
	mustRun(t, "git", "init", "--bare", "-b", "main", dstPath)

	// Populate source with a commit on 'main'.
	work := t.TempDir()
	mustRun(t, "git", "clone", srcPath, work)
	mustRun(t, "git", "-C", work, "-c", "user.name=test", "-c", "user.email=t@t",
		"commit", "--allow-empty", "-m", "first")
	mustRun(t, "git", "-C", work, "push", "origin", "main")

	r := NewRemote()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := r.PushMirror(ctx, srcPath, dstPath, "")
	if err != nil {
		t.Fatalf("PushMirror: %v (stderr: %s)", err, out.Stderr)
	}
	if out.RefsChanged < 1 {
		t.Errorf("RefsChanged = %d, want >= 1 (stderr: %s)", out.RefsChanged, out.Stderr)
	}

	// Verify dest has refs/heads/main
	lsOut := mustOut(t, "git", "--git-dir="+dstPath, "show-ref", "refs/heads/main")
	if lsOut == "" {
		t.Fatalf("destination missing refs/heads/main")
	}
}

func TestIntegration_FetchMirror_PullsAllRefs(t *testing.T) {
	source := t.TempDir()
	dest := t.TempDir()
	srcPath := filepath.Join(source, "src.git")
	dstPath := filepath.Join(dest, "dst.git")

	mustRun(t, "git", "init", "--bare", "-b", "main", srcPath)
	mustRun(t, "git", "init", "--bare", "-b", "main", dstPath)

	// Populate source with two branches
	work := t.TempDir()
	mustRun(t, "git", "clone", srcPath, work)
	mustRun(t, "git", "-C", work, "-c", "user.name=test", "-c", "user.email=t@t",
		"commit", "--allow-empty", "-m", "one")
	mustRun(t, "git", "-C", work, "push", "origin", "main")
	mustRun(t, "git", "-C", work, "checkout", "-b", "other")
	mustRun(t, "git", "-C", work, "push", "origin", "other")

	r := NewRemote()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := r.FetchMirror(ctx, dstPath, srcPath, "")
	if err != nil {
		t.Fatalf("FetchMirror: %v (stderr: %s)", err, out.Stderr)
	}
	if out.RefsChanged < 2 {
		t.Errorf("RefsChanged = %d, want >= 2 (stderr: %s)", out.RefsChanged, out.Stderr)
	}

	// Verify both refs present in dest
	for _, ref := range []string{"refs/heads/main", "refs/heads/other"} {
		if mustOut(t, "git", "--git-dir="+dstPath, "show-ref", ref) == "" {
			t.Errorf("dest missing %s", ref)
		}
	}
}

func TestIntegration_PushMirror_ForceOverwritesDivergence(t *testing.T) {
	source := t.TempDir()
	dest := t.TempDir()
	srcPath := filepath.Join(source, "src.git")
	dstPath := filepath.Join(dest, "dst.git")

	mustRun(t, "git", "init", "--bare", "-b", "main", srcPath)
	mustRun(t, "git", "init", "--bare", "-b", "main", dstPath)

	// Put different commits on source and dest "main"
	srcWork := t.TempDir()
	mustRun(t, "git", "clone", srcPath, srcWork)
	mustRun(t, "git", "-C", srcWork, "-c", "user.name=test", "-c", "user.email=t@t",
		"commit", "--allow-empty", "-m", "src-a")
	mustRun(t, "git", "-C", srcWork, "push", "origin", "main")

	dstWork := t.TempDir()
	mustRun(t, "git", "clone", dstPath, dstWork)
	mustRun(t, "git", "-C", dstWork, "-c", "user.name=test", "-c", "user.email=t@t",
		"commit", "--allow-empty", "-m", "dst-a")
	mustRun(t, "git", "-C", dstWork, "push", "origin", "main")

	r := NewRemote()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := r.PushMirror(ctx, srcPath, dstPath, ""); err != nil {
		t.Fatalf("force push failed: %v", err)
	}

	// After force-mirror, dest main SHA equals source main SHA
	srcSha := strings.TrimSpace(mustOut(t, "git", "--git-dir="+srcPath, "rev-parse", "refs/heads/main"))
	dstSha := strings.TrimSpace(mustOut(t, "git", "--git-dir="+dstPath, "rev-parse", "refs/heads/main"))
	if srcSha == "" || dstSha == "" {
		t.Fatalf("empty SHA src=%q dst=%q", srcSha, dstSha)
	}
	if srcSha != dstSha {
		t.Fatalf("divergence not resolved: src=%s dst=%s", srcSha, dstSha)
	}
}

func TestIntegration_PushMirror_PrunesDeletedBranch(t *testing.T) {
	source := t.TempDir()
	dest := t.TempDir()
	srcPath := filepath.Join(source, "src.git")
	dstPath := filepath.Join(dest, "dst.git")
	mustRun(t, "git", "init", "--bare", "-b", "main", srcPath)
	mustRun(t, "git", "init", "--bare", "-b", "main", dstPath)

	// Populate source with main + throwaway
	work := t.TempDir()
	mustRun(t, "git", "clone", srcPath, work)
	mustRun(t, "git", "-C", work, "-c", "user.name=test", "-c", "user.email=t@t",
		"commit", "--allow-empty", "-m", "one")
	mustRun(t, "git", "-C", work, "push", "origin", "main")
	mustRun(t, "git", "-C", work, "checkout", "-b", "throwaway")
	mustRun(t, "git", "-C", work, "push", "origin", "throwaway")

	r := NewRemote()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First sync: both branches should appear on dest
	if _, err := r.PushMirror(ctx, srcPath, dstPath, ""); err != nil {
		t.Fatalf("first PushMirror: %v", err)
	}
	if mustOut(t, "git", "--git-dir="+dstPath, "show-ref", "refs/heads/throwaway") == "" {
		t.Fatalf("dest missing throwaway after first push")
	}

	// Delete throwaway on source side
	mustRun(t, "git", "-C", work, "push", "origin", "--delete", "throwaway")

	// Second sync: --prune should remove throwaway from dest
	if _, err := r.PushMirror(ctx, srcPath, dstPath, ""); err != nil {
		t.Fatalf("second PushMirror: %v", err)
	}
	out, err := exec.Command("git", "--git-dir="+dstPath, "show-ref", "refs/heads/throwaway").CombinedOutput()
	if err == nil || len(out) != 0 {
		t.Fatalf("throwaway not pruned from dest; show-ref output: %s", out)
	}
}

func TestIntegration_InitBareAndFetchMirror_SucceedsOnEmpty(t *testing.T) {
	source := t.TempDir()
	dest := t.TempDir()
	srcPath := filepath.Join(source, "src.git")
	dstPath := filepath.Join(dest, "dst.git")

	mustRun(t, "git", "init", "--bare", "-b", "main", srcPath)

	// Populate source
	work := t.TempDir()
	mustRun(t, "git", "clone", srcPath, work)
	mustRun(t, "git", "-C", work, "-c", "user.name=test", "-c", "user.email=t@t",
		"commit", "--allow-empty", "-m", "initial")
	mustRun(t, "git", "-C", work, "push", "origin", "main")

	r := NewRemote()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Init-bare then fetch — mimics InitialClone's path without the service layer
	if err := r.InitBare(ctx, dstPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	out, err := r.FetchMirror(ctx, dstPath, srcPath, "")
	if err != nil {
		t.Fatalf("FetchMirror: %v (stderr: %s)", err, out.Stderr)
	}
	if mustOut(t, "git", "--git-dir="+dstPath, "show-ref", "refs/heads/main") == "" {
		t.Fatalf("dest missing refs/heads/main after InitBare+Fetch")
	}

	// Set HEAD explicitly via SetHead and verify
	if err := r.SetHead(ctx, dstPath, "main"); err != nil {
		t.Fatalf("SetHead: %v", err)
	}
	headOut := strings.TrimSpace(mustOut(t, "git", "--git-dir="+dstPath, "symbolic-ref", "HEAD"))
	if headOut != "refs/heads/main" {
		t.Errorf("HEAD = %q, want refs/heads/main", headOut)
	}
}

// -- helpers --

func mustRun(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
}

func mustOut(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
	return string(out)
}

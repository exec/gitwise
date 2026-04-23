package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		parts   []string
		wantErr bool
	}{
		{"valid single", []string{"myrepo"}, false},
		{"valid multiple", []string{"owner", "repo"}, false},
		{"valid with dots", []string{"my.repo"}, false},
		{"valid with hyphens", []string{"my-repo"}, false},
		{"empty string", []string{""}, true},
		{"dot", []string{"."}, true},
		{"double dot", []string{".."}, true},
		{"contains slash", []string{"foo/bar"}, true},
		{"contains backslash", []string{"foo\\bar"}, true},
		{"contains double dot", []string{"foo..bar"}, true},
		{"contains null byte", []string{"foo\x00bar"}, true},
		{"first valid second invalid", []string{"valid", ".."}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.parts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePath(%v) error = %v, wantErr %v", tt.parts, err, tt.wantErr)
			}
		})
	}
}

func TestValidateBranchName(t *testing.T) {
	tests := []struct {
		name    string
		branch  string
		wantErr bool
	}{
		{"simple name", "main", false},
		{"with slash", "feature/my-branch", false},
		{"with dots", "release/v1.0.0", false},
		{"with underscore", "my_branch", false},
		{"single char", "a", true},   // regex requires at least 2 chars
		{"two chars", "ab", false},
		{"max length", string(make([]byte, 256)), true},
		{"empty", "", true},
		{"contains double dot", "feature..branch", true},
		{"contains tilde", "feature~1", true},
		{"contains caret", "feature^2", true},
		{"contains colon", "feature:branch", true},
		{"contains space", "my branch", true},
		{"contains null", "my\x00branch", true},
		{"starts with dot", ".hidden", true},
		{"starts with slash", "/branch", true},
		{"ends with dot", "branch.", true},
		{"ends with slash", "branch/", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBranchName(tt.branch)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBranchName(%q) error = %v, wantErr %v", tt.branch, err, tt.wantErr)
			}
		})
	}
}

func TestIsHexString(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"abcdef0123456789", true},
		{"ABCDEF0123456789", true},
		{"abcDEF012", true},
		{"", true}, // empty string has no non-hex chars
		{"xyz", false},
		{"0123456789abcdefg", false},
		{"hello", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isHexString(tt.input)
			if got != tt.want {
				t.Errorf("isHexString(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestRepoPath(t *testing.T) {
	svc := NewService("/data/repos")
	got := svc.RepoPath("alice", "myproject")
	want := filepath.Join("/data/repos", "alice", "myproject.git")
	if got != want {
		t.Errorf("RepoPath = %q, want %q", got, want)
	}
}

func TestRepoPath_InvalidPanics(t *testing.T) {
	svc := NewService("/data/repos")
	tests := []struct {
		owner, name string
	}{
		{"../etc", "passwd"},
		{"alice", "../../evil"},
		{"", "repo"},
		{"alice", ""},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.owner+"/"+tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("RepoPath(%q, %q) expected panic but did not panic", tt.owner, tt.name)
				}
			}()
			svc.RepoPath(tt.owner, tt.name)
		})
	}
}

func TestRepoPathSafe(t *testing.T) {
	svc := NewService("/data/repos")
	tests := []struct {
		name    string
		owner   string
		repo    string
		wantErr bool
		wantSuf string // suffix of the expected path
	}{
		{"valid", "alice", "myproject", false, filepath.Join("alice", "myproject.git")},
		{"traversal owner", "../etc", "passwd", true, ""},
		{"traversal repo", "alice", "../../evil", true, ""},
		{"empty owner", "", "repo", true, ""},
		{"empty repo", "alice", "", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.RepoPathSafe(tt.owner, tt.repo)
			if (err != nil) != tt.wantErr {
				t.Errorf("RepoPathSafe(%q, %q) error = %v, wantErr %v", tt.owner, tt.repo, err, tt.wantErr)
			}
			if !tt.wantErr && !filepath.IsAbs(got) {
				t.Errorf("RepoPathSafe returned non-absolute path: %q", got)
			}
			if tt.wantSuf != "" && got != filepath.Join("/data/repos", tt.wantSuf) {
				t.Errorf("RepoPathSafe = %q, want suffix %q", got, tt.wantSuf)
			}
		})
	}
}

func TestInitBare(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	err := svc.InitBare("testowner", "testrepo")
	if err != nil {
		t.Fatalf("InitBare error: %v", err)
	}

	// Verify the bare repo was created
	repoPath := filepath.Join(tmpDir, "testowner", "testrepo.git")
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		t.Error("bare repo directory was not created")
	}

	// Verify it has HEAD file (bare repo marker)
	headPath := filepath.Join(repoPath, "HEAD")
	if _, err := os.Stat(headPath); os.IsNotExist(err) {
		t.Error("HEAD file not found in bare repo")
	}
}

func TestAutoInit(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	// First init bare
	if err := svc.InitBare("testowner", "testrepo"); err != nil {
		t.Fatalf("InitBare error: %v", err)
	}

	// Then auto-init with README
	if err := svc.AutoInit("testowner", "testrepo", "main"); err != nil {
		t.Fatalf("AutoInit error: %v", err)
	}

	// Verify we can list branches
	branches, err := svc.ListBranches("testowner", "testrepo")
	if err != nil {
		t.Fatalf("ListBranches error: %v", err)
	}
	if len(branches) == 0 {
		t.Fatal("expected at least one branch after auto-init")
	}

	found := false
	for _, b := range branches {
		if b.Name == "main" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected branch 'main', got branches: %v", branches)
	}
}

func TestAutoInit_CustomBranch(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	if err := svc.InitBare("owner", "repo"); err != nil {
		t.Fatalf("InitBare error: %v", err)
	}

	if err := svc.AutoInit("owner", "repo", "develop"); err != nil {
		t.Fatalf("AutoInit error: %v", err)
	}

	branches, err := svc.ListBranches("owner", "repo")
	if err != nil {
		t.Fatalf("ListBranches error: %v", err)
	}

	found := false
	for _, b := range branches {
		if b.Name == "develop" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected branch 'develop', got branches: %v", branches)
	}
}

func TestResolveRef(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	if err := svc.InitBare("owner", "repo"); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if err := svc.AutoInit("owner", "repo", "main"); err != nil {
		t.Fatalf("AutoInit: %v", err)
	}

	// Resolve by branch name
	sha, err := svc.ResolveRef("owner", "repo", "main")
	if err != nil {
		t.Fatalf("ResolveRef('main') error: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("expected 40-char SHA, got %q", sha)
	}

	// Resolve by full SHA
	sha2, err := svc.ResolveRef("owner", "repo", sha)
	if err != nil {
		t.Fatalf("ResolveRef(sha) error: %v", err)
	}
	if sha2 != sha {
		t.Errorf("resolved SHA mismatch: %q != %q", sha2, sha)
	}

	// Non-existent ref
	_, err = svc.ResolveRef("owner", "repo", "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent ref")
	}
}

func TestListTree(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	if err := svc.InitBare("owner", "repo"); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if err := svc.AutoInit("owner", "repo", "main"); err != nil {
		t.Fatalf("AutoInit: %v", err)
	}

	entries, err := svc.ListTree("owner", "repo", "main", "")
	if err != nil {
		t.Fatalf("ListTree error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (README.md), got %d", len(entries))
	}
	if entries[0].Name != "README.md" {
		t.Errorf("expected README.md, got %q", entries[0].Name)
	}
	if entries[0].Type != "blob" {
		t.Errorf("expected type blob, got %q", entries[0].Type)
	}
}

func TestGetBlob(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	if err := svc.InitBare("owner", "repo"); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if err := svc.AutoInit("owner", "repo", "main"); err != nil {
		t.Fatalf("AutoInit: %v", err)
	}

	blob, err := svc.GetBlob("owner", "repo", "main", "README.md")
	if err != nil {
		t.Fatalf("GetBlob error: %v", err)
	}

	if blob.Name != "README.md" {
		t.Errorf("Name = %q, want README.md", blob.Name)
	}
	if blob.Encoding != "utf-8" {
		t.Errorf("Encoding = %q, want utf-8", blob.Encoding)
	}
	if blob.Content != "# repo\n" {
		t.Errorf("Content = %q, want %q", blob.Content, "# repo\n")
	}
	if blob.IsBinary {
		t.Error("expected IsBinary = false")
	}
}

func TestListCommits(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	if err := svc.InitBare("owner", "repo"); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if err := svc.AutoInit("owner", "repo", "main"); err != nil {
		t.Fatalf("AutoInit: %v", err)
	}

	commits, hasMore, err := svc.ListCommits("owner", "repo", "main", 1, 30)
	if err != nil {
		t.Fatalf("ListCommits error: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	if hasMore {
		t.Error("expected hasMore = false")
	}
	if commits[0].Message != "Initial commit" {
		t.Errorf("Message = %q, want %q", commits[0].Message, "Initial commit")
	}
	if commits[0].Author.Name != "Gitwise" {
		t.Errorf("Author.Name = %q, want %q", commits[0].Author.Name, "Gitwise")
	}
}

func TestRemove(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	if err := svc.InitBare("owner", "repo"); err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	repoPath := svc.RepoPath("owner", "repo")
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		t.Fatal("repo should exist before removal")
	}

	svc.Remove("owner", "repo")

	if _, err := os.Stat(repoPath); !os.IsNotExist(err) {
		t.Error("repo should not exist after removal")
	}
}

func TestDeleteBranch(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	if err := svc.InitBare("owner", "repo"); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if err := svc.AutoInit("owner", "repo", "main"); err != nil {
		t.Fatalf("AutoInit: %v", err)
	}

	// Delete should succeed (even though it's the only branch)
	err := svc.DeleteBranch("owner", "repo", "main")
	if err != nil {
		t.Fatalf("DeleteBranch error: %v", err)
	}

	// After deletion, resolving the ref should fail
	_, err = svc.ResolveRef("owner", "repo", "main")
	if err == nil {
		t.Error("expected error resolving deleted branch")
	}
}

func TestGetCommit(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	if err := svc.InitBare("owner", "repo"); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if err := svc.AutoInit("owner", "repo", "main"); err != nil {
		t.Fatalf("AutoInit: %v", err)
	}

	sha, err := svc.ResolveRef("owner", "repo", "main")
	if err != nil {
		t.Fatalf("ResolveRef: %v", err)
	}

	detail, err := svc.GetCommit("owner", "repo", sha)
	if err != nil {
		t.Fatalf("GetCommit error: %v", err)
	}

	if detail.SHA != sha {
		t.Errorf("SHA = %q, want %q", detail.SHA, sha)
	}
	if detail.Message != "Initial commit" {
		t.Errorf("Message = %q, want %q", detail.Message, "Initial commit")
	}
	// Initial commit has no parent, so the code path for diff has no parentTree
	// and may return 0 files — that's expected behavior
	// Just verify the detail object is well-formed
	if detail.Stats.TotalFiles != len(detail.Files) {
		t.Errorf("stats mismatch: TotalFiles=%d, len(Files)=%d", detail.Stats.TotalFiles, len(detail.Files))
	}
}

func TestListCommits_Pagination(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	if err := svc.InitBare("owner", "repo"); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if err := svc.AutoInit("owner", "repo", "main"); err != nil {
		t.Fatalf("AutoInit: %v", err)
	}

	// With perPage=1, should get 1 commit with hasMore=false (only 1 commit exists)
	commits, hasMore, err := svc.ListCommits("owner", "repo", "main", 1, 1)
	if err != nil {
		t.Fatalf("ListCommits error: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	if hasMore {
		t.Error("expected hasMore = false with single commit")
	}

	// Default perPage when invalid
	commits, _, err = svc.ListCommits("owner", "repo", "main", 1, 0)
	if err != nil {
		t.Fatalf("ListCommits with perPage=0 error: %v", err)
	}
	if len(commits) != 1 {
		t.Errorf("expected 1 commit with default perPage, got %d", len(commits))
	}
}

func TestOpenRepo_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	_, err := svc.ListBranches("noone", "norepo")
	if err == nil {
		t.Error("expected error opening non-existent repo")
	}
}

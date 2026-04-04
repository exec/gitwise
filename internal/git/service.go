package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/gitwise-io/gitwise/internal/models"
)

var errStopIteration = errors.New("stop iteration")

type Service struct {
	reposPath string
}

func NewService(reposPath string) *Service {
	return &Service{reposPath: reposPath}
}

// RepoPath returns the filesystem path to a bare repo.
// Returns ("", error) if the owner or name contains path traversal characters.
func (s *Service) RepoPath(owner, name string) string {
	return filepath.Join(s.reposPath, owner, name+".git")
}

// ValidatePath checks that owner and repo name are safe path components
// with no traversal (../, /, or null bytes).
// Note: Go's net/http URL-decodes paths before handlers see them, and SSH
// clients don't URL-encode, so checking literal characters is sufficient.
func ValidatePath(parts ...string) error {
	for _, p := range parts {
		if p == "" || p == "." || p == ".." ||
			strings.ContainsAny(p, "/\\") ||
			strings.Contains(p, "..") ||
			strings.ContainsRune(p, 0) {
			return fmt.Errorf("invalid path component: %q", p)
		}
	}
	return nil
}

var branchNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]{0,253}[a-zA-Z0-9]$`)

// ValidateBranchName checks that a branch name is safe for use in refspecs.
func ValidateBranchName(name string) error {
	if len(name) < 1 || len(name) > 255 {
		return fmt.Errorf("branch name must be 1-255 characters")
	}
	if strings.Contains(name, "..") || strings.Contains(name, "~") ||
		strings.Contains(name, "^") || strings.Contains(name, ":") ||
		strings.Contains(name, " ") || strings.ContainsRune(name, 0) {
		return fmt.Errorf("branch name contains invalid characters")
	}
	if !branchNameRe.MatchString(name) {
		return fmt.Errorf("branch name contains invalid characters")
	}
	return nil
}

// InitBare creates a new bare git repository on disk.
func (s *Service) InitBare(owner, name string) error {
	path := s.RepoPath(owner, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	_, err := gogit.PlainInit(path, true)
	if err != nil {
		return fmt.Errorf("git init --bare: %w", err)
	}
	return nil
}

// AutoInit creates an initial commit with a README on the given branch.
func (s *Service) AutoInit(owner, name, branch string) error {
	path := s.RepoPath(owner, name)

	// Open the bare repo and create the initial commit directly via go-git
	r, err := gogit.PlainOpen(path)
	if err != nil {
		return fmt.Errorf("open bare repo: %w", err)
	}

	// We need a worktree to create files, so clone to a temp dir, commit, push back.
	// Use a unique temp dir to avoid collisions.
	tmpDir, err := os.MkdirTemp("", "gitwise-autoinit-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Clone the bare repo to the temp dir
	wt, err := gogit.PlainClone(tmpDir, false, &gogit.CloneOptions{
		URL: path,
	})
	if err != nil {
		// Empty repo can't be cloned — init fresh instead
		wt, err = gogit.PlainInit(tmpDir, false)
		if err != nil {
			return fmt.Errorf("init temp repo: %w", err)
		}
		// Add the bare repo as remote
		_, err = wt.CreateRemote(&gogitconfig.RemoteConfig{
			Name: "origin",
			URLs: []string{path},
		})
		if err != nil {
			return fmt.Errorf("add remote: %w", err)
		}
	}
	_ = r // keep linter happy

	w, err := wt.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}

	// Create README
	readmePath := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# "+name+"\n"), 0o644); err != nil {
		return fmt.Errorf("write readme: %w", err)
	}

	if _, err := w.Add("README.md"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	_, err = w.Commit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Gitwise",
			Email: "noreply@gitwise.dev",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	// Create the branch ref pointing to HEAD
	head, err := wt.Head()
	if err != nil {
		return fmt.Errorf("get head: %w", err)
	}

	// If the branch name differs from the default, create it
	if head.Name().Short() != branch {
		ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), head.Hash())
		if err := wt.Storer.SetReference(ref); err != nil {
			return fmt.Errorf("create branch ref: %w", err)
		}
	}

	// Push to the bare repo
	err = wt.Push(&gogit.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []gogitconfig.RefSpec{gogitconfig.RefSpec("+refs/heads/" + branch + ":refs/heads/" + branch)},
	})
	if err != nil {
		return fmt.Errorf("git push: %w", err)
	}

	return nil
}

// Remove deletes a bare repo from disk.
func (s *Service) Remove(owner, name string) {
	os.RemoveAll(s.RepoPath(owner, name))
}

// openRepo opens a bare repository via go-git.
func (s *Service) openRepo(owner, name string) (*gogit.Repository, error) {
	path := s.RepoPath(owner, name)
	r, err := gogit.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("open repo %s/%s: %w", owner, name, err)
	}
	return r, nil
}

// ResolveRef resolves a ref name (branch, tag, or full SHA hex) to a commit hash.
// Only branches, tags, and 40-char hex SHAs are accepted — arbitrary revision
// syntax (HEAD~3, @{upstream}, etc.) is rejected for safety.
func (s *Service) ResolveRef(owner, name, ref string) (string, error) {
	r, err := s.openRepo(owner, name)
	if err != nil {
		return "", err
	}

	// Try as a full hex SHA
	if len(ref) == 40 && isHexString(ref) {
		return ref, nil
	}

	// Try branch
	h, err := r.ResolveRevision(plumbing.Revision("refs/heads/" + ref))
	if err == nil {
		return h.String(), nil
	}

	// Try tag
	h, err = r.ResolveRevision(plumbing.Revision("refs/tags/" + ref))
	if err == nil {
		return h.String(), nil
	}

	return "", fmt.Errorf("resolve ref %q: ref not found", ref)
}

func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// ListTree returns the entries of a tree at the given path and ref.
func (s *Service) ListTree(owner, name, ref, treePath string) ([]models.TreeEntry, error) {
	r, err := s.openRepo(owner, name)
	if err != nil {
		return nil, err
	}

	sha, err := s.ResolveRef(owner, name, ref)
	if err != nil {
		return nil, err
	}

	commit, err := r.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		return nil, fmt.Errorf("get commit: %w", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("get tree: %w", err)
	}

	// Navigate to subpath if needed
	if treePath != "" && treePath != "/" {
		treePath = strings.TrimPrefix(treePath, "/")
		tree, err = tree.Tree(treePath)
		if err != nil {
			return nil, fmt.Errorf("get subtree %q: %w", treePath, err)
		}
	}

	var entries []models.TreeEntry
	for _, entry := range tree.Entries {
		e := models.TreeEntry{
			Name: entry.Name,
			Path: filepath.Join(treePath, entry.Name),
			SHA:  entry.Hash.String(),
			Mode: entry.Mode.String(),
		}
		if entry.Mode.IsFile() {
			e.Type = "blob"
			f, err := tree.TreeEntryFile(&entry)
			if err == nil {
				e.Size = f.Size
			}
		} else {
			e.Type = "tree"
		}
		entries = append(entries, e)
	}

	// Sort: directories first, then alphabetical
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Type != entries[j].Type {
			return entries[i].Type == "tree"
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	return entries, nil
}

// GetBlob returns the content of a file at the given path and ref.
func (s *Service) GetBlob(owner, name, ref, filePath string) (*models.BlobContent, error) {
	r, err := s.openRepo(owner, name)
	if err != nil {
		return nil, err
	}

	sha, err := s.ResolveRef(owner, name, ref)
	if err != nil {
		return nil, err
	}

	commit, err := r.CommitObject(plumbing.NewHash(sha))
	if err != nil {
		return nil, fmt.Errorf("get commit: %w", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("get tree: %w", err)
	}

	filePath = strings.TrimPrefix(filePath, "/")
	f, err := tree.File(filePath)
	if err != nil {
		return nil, fmt.Errorf("get file %q: %w", filePath, err)
	}

	blob := &models.BlobContent{
		Name: filepath.Base(filePath),
		Path: filePath,
		SHA:  f.Hash.String(),
		Size: f.Size,
	}

	isBin, err := f.IsBinary()
	if err == nil && isBin {
		blob.IsBinary = true
		blob.Encoding = "base64"
		return blob, nil
	}

	content, err := f.Contents()
	if err != nil {
		return nil, fmt.Errorf("read content: %w", err)
	}

	blob.Content = content
	blob.Encoding = "utf-8"
	return blob, nil
}

// ListCommits returns commits for the given ref with pagination.
func (s *Service) ListCommits(owner, name, ref string, page, perPage int) ([]models.Commit, bool, error) {
	r, err := s.openRepo(owner, name)
	if err != nil {
		return nil, false, err
	}

	sha, err := s.ResolveRef(owner, name, ref)
	if err != nil {
		return nil, false, err
	}

	if perPage <= 0 || perPage > 100 {
		perPage = 30
	}
	if page <= 0 {
		page = 1
	}
	skip := (page - 1) * perPage

	iter, err := r.Log(&gogit.LogOptions{
		From: plumbing.NewHash(sha),
	})
	if err != nil {
		return nil, false, fmt.Errorf("git log: %w", err)
	}

	var commits []models.Commit
	idx := 0
	hasMore := false

	err = iter.ForEach(func(c *object.Commit) error {
		if idx < skip {
			idx++
			return nil
		}
		if len(commits) >= perPage {
			hasMore = true
			return errStopIteration
		}

		var parents []string
		for _, p := range c.ParentHashes {
			parents = append(parents, p.String())
		}

		commits = append(commits, models.Commit{
			SHA:     c.Hash.String(),
			Message: c.Message,
			Author: models.GitUser{
				Name:  c.Author.Name,
				Email: c.Author.Email,
				Date:  c.Author.When,
			},
			Committer: models.GitUser{
				Name:  c.Committer.Name,
				Email: c.Committer.Email,
				Date:  c.Committer.When,
			},
			Parents: parents,
			TreeSHA: c.TreeHash.String(),
			Date:    c.Author.When,
		})
		idx++
		return nil
	})
	if err != nil && !errors.Is(err, errStopIteration) {
		return nil, false, fmt.Errorf("iterate commits: %w", err)
	}

	return commits, hasMore, nil
}

// GetCommit returns a single commit with diff stats.
func (s *Service) GetCommit(owner, name, sha string) (*models.CommitDetail, error) {
	r, err := s.openRepo(owner, name)
	if err != nil {
		return nil, err
	}

	resolved, err := s.ResolveRef(owner, name, sha)
	if err != nil {
		return nil, err
	}

	commit, err := r.CommitObject(plumbing.NewHash(resolved))
	if err != nil {
		return nil, fmt.Errorf("get commit: %w", err)
	}

	var parents []string
	for _, p := range commit.ParentHashes {
		parents = append(parents, p.String())
	}

	detail := &models.CommitDetail{
		Commit: models.Commit{
			SHA:     commit.Hash.String(),
			Message: commit.Message,
			Author: models.GitUser{
				Name:  commit.Author.Name,
				Email: commit.Author.Email,
				Date:  commit.Author.When,
			},
			Committer: models.GitUser{
				Name:  commit.Committer.Name,
				Email: commit.Committer.Email,
				Date:  commit.Committer.When,
			},
			Parents: parents,
			TreeSHA: commit.TreeHash.String(),
			Date:    commit.Author.When,
		},
	}

	// Get diff stats
	commitTree, err := commit.Tree()
	if err != nil {
		return detail, nil
	}

	var parentTree *object.Tree
	if commit.NumParents() > 0 {
		parent, err := commit.Parents().Next()
		if err == nil {
			parentTree, _ = parent.Tree()
		}
	}

	if parentTree != nil {
		changes, err := parentTree.Diff(commitTree)
		if err == nil {
			for _, change := range changes {
				df := models.DiffFile{}
				action, err := change.Action()
				if err == nil {
					switch action.String() {
					case "Insert":
						df.Status = "added"
					case "Delete":
						df.Status = "deleted"
					case "Modify":
						df.Status = "modified"
					}
				}

				from := change.From
				to := change.To
				if to.Name != "" {
					df.Path = to.Name
				} else {
					df.Path = from.Name
				}
				if from.Name != "" && to.Name != "" && from.Name != to.Name {
					df.OldPath = from.Name
					df.Status = "renamed"
				}

				patch, err := change.Patch()
				if err == nil {
					df.Patch = patch.String()
					for _, stat := range patch.Stats() {
						df.Insertions += stat.Addition
						df.Deletions += stat.Deletion
					}
				}

				detail.Files = append(detail.Files, df)
				detail.Stats.TotalFiles++
				detail.Stats.TotalAdditions += df.Insertions
				detail.Stats.TotalDeletions += df.Deletions
			}
		}
	}

	return detail, nil
}

// CompareBranches returns the diff between base and head branches.
// This is used for pull request diffs — it shows what head introduces on top of base.
func (s *Service) CompareBranches(owner, name, base, head string) (*models.PRDiffResponse, error) {
	r, err := s.openRepo(owner, name)
	if err != nil {
		return nil, err
	}

	baseSHA, err := s.ResolveRef(owner, name, base)
	if err != nil {
		return nil, fmt.Errorf("resolve base: %w", err)
	}
	headSHA, err := s.ResolveRef(owner, name, head)
	if err != nil {
		return nil, fmt.Errorf("resolve head: %w", err)
	}

	baseCommit, err := r.CommitObject(plumbing.NewHash(baseSHA))
	if err != nil {
		return nil, fmt.Errorf("get base commit: %w", err)
	}
	headCommit, err := r.CommitObject(plumbing.NewHash(headSHA))
	if err != nil {
		return nil, fmt.Errorf("get head commit: %w", err)
	}

	// Find merge base
	mergeBaseCommits, err := baseCommit.MergeBase(headCommit)
	if err != nil || len(mergeBaseCommits) == 0 {
		// No common ancestor — diff entire head tree against empty
		mergeBaseCommits = nil
	}

	// Get diff from merge-base (or base) to head
	var fromTree *object.Tree
	if len(mergeBaseCommits) > 0 {
		fromTree, _ = mergeBaseCommits[0].Tree()
	} else {
		fromTree, _ = baseCommit.Tree()
	}
	toTree, err := headCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("get head tree: %w", err)
	}

	resp := &models.PRDiffResponse{}

	if fromTree != nil {
		changes, err := fromTree.Diff(toTree)
		if err == nil {
			for _, change := range changes {
				df := models.DiffFile{}
				action, err := change.Action()
				if err == nil {
					switch action.String() {
					case "Insert":
						df.Status = "added"
					case "Delete":
						df.Status = "deleted"
					case "Modify":
						df.Status = "modified"
					}
				}

				from := change.From
				to := change.To
				if to.Name != "" {
					df.Path = to.Name
				} else {
					df.Path = from.Name
				}
				if from.Name != "" && to.Name != "" && from.Name != to.Name {
					df.OldPath = from.Name
					df.Status = "renamed"
				}

				patch, err := change.Patch()
				if err == nil {
					df.Patch = patch.String()
					for _, stat := range patch.Stats() {
						df.Insertions += stat.Addition
						df.Deletions += stat.Deletion
					}
				}

				resp.Files = append(resp.Files, df)
				resp.Stats.TotalFiles++
				resp.Stats.TotalAdditions += df.Insertions
				resp.Stats.TotalDeletions += df.Deletions
			}
		}
	}

	// Collect commits from head back to merge-base
	var mergeBaseSHA string
	if len(mergeBaseCommits) > 0 {
		mergeBaseSHA = mergeBaseCommits[0].Hash.String()
	}

	iter, err := r.Log(&gogit.LogOptions{From: plumbing.NewHash(headSHA)})
	if err == nil {
		_ = iter.ForEach(func(c *object.Commit) error {
			if c.Hash.String() == mergeBaseSHA || c.Hash.String() == baseSHA {
				return errStopIteration
			}

			var parents []string
			for _, p := range c.ParentHashes {
				parents = append(parents, p.String())
			}

			resp.Commits = append(resp.Commits, models.Commit{
				SHA:     c.Hash.String(),
				Message: c.Message,
				Author: models.GitUser{
					Name:  c.Author.Name,
					Email: c.Author.Email,
					Date:  c.Author.When,
				},
				Committer: models.GitUser{
					Name:  c.Committer.Name,
					Email: c.Committer.Email,
					Date:  c.Committer.When,
				},
				Parents: parents,
				TreeSHA: c.TreeHash.String(),
				Date:    c.Author.When,
			})
			resp.Stats.TotalCommits++
			return nil
		})
	}

	return resp, nil
}

// MergeBranches merges head branch into base branch using the given strategy.
func (s *Service) MergeBranches(owner, name, base, head, strategy, message, authorName, authorEmail string) error {
	r, err := s.openRepo(owner, name)
	if err != nil {
		return err
	}

	baseSHA, err := s.ResolveRef(owner, name, base)
	if err != nil {
		return fmt.Errorf("resolve base: %w", err)
	}
	headSHA, err := s.ResolveRef(owner, name, head)
	if err != nil {
		return fmt.Errorf("resolve head: %w", err)
	}

	baseCommit, err := r.CommitObject(plumbing.NewHash(baseSHA))
	if err != nil {
		return fmt.Errorf("get base commit: %w", err)
	}
	headCommit, err := r.CommitObject(plumbing.NewHash(headSHA))
	if err != nil {
		return fmt.Errorf("get head commit: %w", err)
	}

	sig := &object.Signature{
		Name:  authorName,
		Email: authorEmail,
		When:  time.Now(),
	}

	switch strategy {
	case "merge":
		return s.doMergeCommit(r, owner, name, base, baseCommit, headCommit, message, sig)
	case "squash":
		return s.doSquashMerge(r, owner, name, base, baseCommit, headCommit, message, sig)
	case "rebase":
		return fmt.Errorf("rebase merge strategy is not yet supported; use merge or squash")
	default:
		return fmt.Errorf("unknown merge strategy: %s", strategy)
	}
}

func (s *Service) doMergeCommit(r *gogit.Repository, owner, name, base string, baseCommit, headCommit *object.Commit, message string, sig *object.Signature) error {
	// Use head's tree as the merge result (fast-forward-like for simple cases)
	// For a real 3-way merge we'd need to do conflict resolution.
	// Check if head is already ahead of base (fast-forward possible)
	isAncestor, err := baseCommit.IsAncestor(headCommit)
	if err != nil {
		return fmt.Errorf("check ancestor: %w", err)
	}

	var newHash plumbing.Hash
	if isAncestor {
		// Fast-forward: create a merge commit with head's tree
		commit := r.Storer
		newCommit := &object.Commit{
			Author:       *sig,
			Committer:    *sig,
			Message:      message,
			TreeHash:     headCommit.TreeHash,
			ParentHashes: []plumbing.Hash{baseCommit.Hash, headCommit.Hash},
		}

		obj := commit.NewEncodedObject()
		if err := newCommit.Encode(obj); err != nil {
			return fmt.Errorf("encode merge commit: %w", err)
		}
		newHash, err = commit.SetEncodedObject(obj)
		if err != nil {
			return fmt.Errorf("store merge commit: %w", err)
		}
	} else {
		return fmt.Errorf("non-fast-forward merge not supported: rebase or squash instead")
	}

	// Update base branch ref
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(base), newHash)
	return r.Storer.SetReference(ref)
}

func (s *Service) doSquashMerge(r *gogit.Repository, owner, name, base string, baseCommit, headCommit *object.Commit, message string, sig *object.Signature) error {
	// Create a single commit on base with head's tree
	commit := r.Storer
	newCommit := &object.Commit{
		Author:       *sig,
		Committer:    *sig,
		Message:      message,
		TreeHash:     headCommit.TreeHash,
		ParentHashes: []plumbing.Hash{baseCommit.Hash},
	}

	obj := commit.NewEncodedObject()
	if err := newCommit.Encode(obj); err != nil {
		return fmt.Errorf("encode squash commit: %w", err)
	}
	newHash, err := commit.SetEncodedObject(obj)
	if err != nil {
		return fmt.Errorf("store squash commit: %w", err)
	}

	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(base), newHash)
	return r.Storer.SetReference(ref)
}

// DeleteBranch removes a branch reference from a bare repository.
func (s *Service) DeleteBranch(owner, name, branch string) error {
	r, err := s.openRepo(owner, name)
	if err != nil {
		return err
	}
	ref := plumbing.NewBranchReferenceName(branch)
	return r.Storer.RemoveReference(ref)
}

// ListBranches returns all branches.
func (s *Service) ListBranches(owner, name string) ([]models.Branch, error) {
	r, err := s.openRepo(owner, name)
	if err != nil {
		return nil, err
	}

	head, _ := r.Head()
	iter, err := r.Branches()
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}

	var branches []models.Branch
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		b := models.Branch{
			Name: ref.Name().Short(),
			SHA:  ref.Hash().String(),
		}
		if head != nil && ref.Hash() == head.Hash() {
			b.IsHead = true
		}
		branches = append(branches, b)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iterate branches: %w", err)
	}

	sort.Slice(branches, func(i, j int) bool {
		if branches[i].IsHead != branches[j].IsHead {
			return branches[i].IsHead
		}
		return branches[i].Name < branches[j].Name
	})

	return branches, nil
}

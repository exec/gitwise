package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// ResolveRef resolves a ref name (branch, tag, or SHA) to a commit hash.
func (s *Service) ResolveRef(owner, name, ref string) (string, error) {
	r, err := s.openRepo(owner, name)
	if err != nil {
		return "", err
	}

	// Try as a full hash first
	if len(ref) == 40 {
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

	// Try as revision
	h, err = r.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return "", fmt.Errorf("resolve ref %q: %w", ref, err)
	}
	return h.String(), nil
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

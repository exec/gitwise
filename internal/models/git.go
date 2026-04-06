package models

import "time"

// TreeEntry represents a file or directory in a git tree.
type TreeEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"` // blob, tree
	Mode string `json:"mode"`
	SHA  string `json:"sha"`
	Size int64  `json:"size,omitempty"` // only for blobs
}

// BlobContent represents file content from a git blob.
type BlobContent struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	SHA      string `json:"sha"`
	Size     int64  `json:"size"`
	Content  string `json:"content,omitempty"`
	Encoding string `json:"encoding"` // utf-8, base64
	IsBinary bool   `json:"is_binary"`
}

// Commit represents a git commit with metadata.
type Commit struct {
	SHA       string    `json:"sha"`
	Message   string    `json:"message"`
	Author    GitUser   `json:"author"`
	Committer GitUser   `json:"committer"`
	Parents   []string  `json:"parents"`
	TreeSHA   string    `json:"tree_sha"`
	Date      time.Time `json:"date"`
}

type GitUser struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  time.Time `json:"date"`
}

// DiffFile represents a changed file in a diff.
type DiffFile struct {
	Path       string `json:"path"`
	OldPath    string `json:"old_path,omitempty"`
	Status     string `json:"status"` // added, modified, deleted, renamed
	Insertions int    `json:"insertions"`
	Deletions  int    `json:"deletions"`
	Patch      string `json:"patch,omitempty"`
}

// CommitDetail is a commit with its diff.
type CommitDetail struct {
	Commit
	Files []DiffFile `json:"files"`
	Stats struct {
		TotalFiles     int `json:"total_files"`
		TotalAdditions int `json:"total_additions"`
		TotalDeletions int `json:"total_deletions"`
	} `json:"stats"`
}

// BlameLine represents a single line of blame output.
type BlameLine struct {
	CommitSHA   string    `json:"commit_sha"`
	AuthorName  string    `json:"author_name"`
	AuthorEmail string    `json:"author_email"`
	Date        time.Time `json:"date"`
	LineNumber  int       `json:"line_number"`
	LineContent string    `json:"line_content"`
}

// Branch represents a git branch.
type Branch struct {
	Name   string `json:"name"`
	SHA    string `json:"sha"`
	IsHead bool   `json:"is_head"`
}

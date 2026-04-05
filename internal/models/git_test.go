package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTreeEntry_JSON(t *testing.T) {
	entry := TreeEntry{
		Name: "main.go",
		Path: "cmd/main.go",
		Type: "blob",
		Mode: "100644",
		SHA:  "abc123",
		Size: 1024,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded TreeEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Name != "main.go" {
		t.Errorf("Name = %q", decoded.Name)
	}
	if decoded.Type != "blob" {
		t.Errorf("Type = %q", decoded.Type)
	}
	if decoded.Size != 1024 {
		t.Errorf("Size = %d", decoded.Size)
	}
}

func TestTreeEntry_SizeOmittedForTree(t *testing.T) {
	entry := TreeEntry{Name: "src", Type: "tree", Size: 0}
	data, _ := json.Marshal(entry)
	var m map[string]any
	json.Unmarshal(data, &m)
	// Size 0 should be omitted (omitempty)
	if _, ok := m["size"]; ok {
		t.Error("size should be omitted when 0")
	}
}

func TestBlobContent_JSON(t *testing.T) {
	blob := BlobContent{
		Name:     "readme.md",
		Path:     "readme.md",
		SHA:      "def456",
		Size:     512,
		Content:  "# Hello",
		Encoding: "utf-8",
		IsBinary: false,
	}

	data, err := json.Marshal(blob)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded BlobContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Content != "# Hello" {
		t.Errorf("Content = %q", decoded.Content)
	}
}

func TestCommit_JSON(t *testing.T) {
	c := Commit{
		SHA:     "abc123def456",
		Message: "Initial commit",
		Author: GitUser{
			Name:  "Alice",
			Email: "alice@example.com",
			Date:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Parents: []string{"parent1"},
		TreeSHA: "tree123",
		Date:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Commit
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Author.Name != "Alice" {
		t.Errorf("Author.Name = %q", decoded.Author.Name)
	}
	if len(decoded.Parents) != 1 {
		t.Errorf("Parents = %v", decoded.Parents)
	}
}

func TestDiffFile_JSON(t *testing.T) {
	df := DiffFile{
		Path:       "main.go",
		OldPath:    "",
		Status:     "added",
		Insertions: 10,
		Deletions:  0,
	}

	data, _ := json.Marshal(df)
	var decoded DiffFile
	json.Unmarshal(data, &decoded)
	if decoded.Status != "added" {
		t.Errorf("Status = %q", decoded.Status)
	}
	if decoded.Insertions != 10 {
		t.Errorf("Insertions = %d", decoded.Insertions)
	}
}

func TestDiffFile_OldPathOmitted(t *testing.T) {
	df := DiffFile{Path: "main.go", Status: "modified"}
	data, _ := json.Marshal(df)
	var m map[string]any
	json.Unmarshal(data, &m)
	if _, ok := m["old_path"]; ok {
		t.Error("old_path should be omitted when empty")
	}
}

func TestBranch_JSON(t *testing.T) {
	b := Branch{Name: "main", SHA: "abc123", IsHead: true}
	data, _ := json.Marshal(b)
	var decoded Branch
	json.Unmarshal(data, &decoded)
	if !decoded.IsHead {
		t.Error("IsHead should be true")
	}
}

package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// gitLabClient handles GitLab REST API calls with rate limiting.
type gitLabClient struct {
	token       string
	instanceURL string
	httpClient  *http.Client
}

func newGitLabClient(token, instanceURL string) *gitLabClient {
	return &gitLabClient{
		token:       token,
		instanceURL: strings.TrimSuffix(instanceURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// glProject is the GitLab API project response (partial).
type glProject struct {
	ID            int      `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	DefaultBranch string   `json:"default_branch"`
	Visibility    string   `json:"visibility"` // private, internal, public
	Topics        []string `json:"topics"`
	TagList       []string `json:"tag_list"` // older GitLab versions use this
}

// glIssue is the GitLab API issue response (partial).
type glIssue struct {
	IID       int        `json:"iid"`
	Title     string     `json:"title"`
	Description string   `json:"description"`
	State     string     `json:"state"` // opened, closed
	Labels    []string   `json:"labels"`
	Author    glAuthor   `json:"author"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at"`
}

type glAuthor struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

// glMergeRequest is the GitLab API merge request response (partial).
type glMergeRequest struct {
	IID          int        `json:"iid"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	State        string     `json:"state"` // opened, closed, merged, locked
	SourceBranch string     `json:"source_branch"`
	TargetBranch string     `json:"target_branch"`
	Labels       []string   `json:"labels"`
	Author       glAuthor   `json:"author"`
	MergedAt     *time.Time `json:"merged_at"`
	ClosedAt     *time.Time `json:"closed_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// glNote is the GitLab API note/comment response (partial).
type glNote struct {
	Body      string    `json:"body"`
	Author    glAuthor  `json:"author"`
	System    bool      `json:"system"`
	CreatedAt time.Time `json:"created_at"`
}

func (c *gitLabClient) apiURL(path string) string {
	return fmt.Sprintf("%s/api/v4%s", c.instanceURL, path)
}

func (c *gitLabClient) doRequest(ctx context.Context, reqURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Handle rate limiting
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := resp.Header.Get("Retry-After")
		resp.Body.Close()
		if retryAfter != "" {
			secs, _ := strconv.Atoi(retryAfter)
			if secs > 0 && secs < 300 {
				slog.Info("gitlab rate limit hit, sleeping", "seconds", secs)
				time.Sleep(time.Duration(secs) * time.Second)
				return c.doRequest(ctx, reqURL)
			}
		}
		return nil, fmt.Errorf("gitlab rate limit exceeded")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("gitlab API error %d: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}

// paginateGL follows GitLab's page-based pagination.
func (c *gitLabClient) paginateGL(ctx context.Context, baseURL string, perPage int) ([]json.RawMessage, error) {
	var all []json.RawMessage
	page := 1

	for {
		reqURL := fmt.Sprintf("%s%sper_page=%d&page=%d", baseURL, urlSep(baseURL), perPage, page)
		resp, err := c.doRequest(ctx, reqURL)
		if err != nil {
			return all, err
		}

		var items []json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
			resp.Body.Close()
			return all, fmt.Errorf("decode page: %w", err)
		}
		resp.Body.Close()

		if len(items) == 0 {
			break
		}

		all = append(all, items...)

		// Check if there are more pages
		totalPages := resp.Header.Get("X-Total-Pages")
		if totalPages != "" {
			tp, _ := strconv.Atoi(totalPages)
			if page >= tp {
				break
			}
		}

		// If no X-Total-Pages header, stop when we get fewer items than requested
		if len(items) < perPage {
			break
		}

		page++
	}

	return all, nil
}

func (c *gitLabClient) getProject(ctx context.Context, namespace, projectName string) (*glProject, error) {
	// URL-encode the full project path (namespace/project)
	projectPath := url.PathEscape(namespace + "/" + projectName)
	reqURL := c.apiURL(fmt.Sprintf("/projects/%s", projectPath))

	resp, err := c.doRequest(ctx, reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var p glProject
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, fmt.Errorf("decode project: %w", err)
	}

	// Merge tag_list into topics if topics is empty (older GitLab API)
	if len(p.Topics) == 0 && len(p.TagList) > 0 {
		p.Topics = p.TagList
	}

	return &p, nil
}

func (c *gitLabClient) listIssues(ctx context.Context, projectID int) ([]externalIssue, error) {
	reqURL := c.apiURL(fmt.Sprintf("/projects/%d/issues?scope=all&state=all&order_by=created_at&sort=asc", projectID))
	pages, err := c.paginateGL(ctx, reqURL, 100)
	if err != nil {
		return nil, err
	}

	var results []externalIssue
	for _, raw := range pages {
		var iss glIssue
		if err := json.Unmarshal(raw, &iss); err != nil {
			continue
		}

		state := "open"
		if iss.State == "closed" {
			state = "closed"
		}

		ext := externalIssue{
			Number:    iss.IID,
			Title:     iss.Title,
			Body:      iss.Description,
			State:     state,
			Labels:    iss.Labels,
			CreatedAt: iss.CreatedAt,
			UpdatedAt: iss.UpdatedAt,
			ClosedAt:  iss.ClosedAt,
			Author:    iss.Author.Username,
		}

		// Fetch comments (notes) for this issue
		comments, err := c.listIssueNotes(ctx, projectID, iss.IID)
		if err != nil {
			slog.Warn("failed to fetch issue notes", "iid", iss.IID, "error", err)
		} else {
			ext.Comments = comments
		}

		results = append(results, ext)
	}
	return results, nil
}

func (c *gitLabClient) listMergeRequests(ctx context.Context, projectID int) ([]externalPR, error) {
	reqURL := c.apiURL(fmt.Sprintf("/projects/%d/merge_requests?scope=all&state=all&order_by=created_at&sort=asc", projectID))
	pages, err := c.paginateGL(ctx, reqURL, 100)
	if err != nil {
		return nil, err
	}

	var results []externalPR
	for _, raw := range pages {
		var mr glMergeRequest
		if err := json.Unmarshal(raw, &mr); err != nil {
			continue
		}

		state := "open"
		switch mr.State {
		case "closed", "locked":
			state = "closed"
		case "merged":
			state = "merged"
		}

		ext := externalPR{
			Number:       mr.IID,
			Title:        mr.Title,
			Body:         mr.Description,
			State:        state,
			SourceBranch: mr.SourceBranch,
			TargetBranch: mr.TargetBranch,
			Labels:       mr.Labels,
			CreatedAt:    mr.CreatedAt,
			UpdatedAt:    mr.UpdatedAt,
			MergedAt:     mr.MergedAt,
			ClosedAt:     mr.ClosedAt,
			Author:       mr.Author.Username,
		}

		// Fetch comments (notes) for this MR
		comments, err := c.listMRNotes(ctx, projectID, mr.IID)
		if err != nil {
			slog.Warn("failed to fetch MR notes", "iid", mr.IID, "error", err)
		} else {
			ext.Comments = comments
		}

		results = append(results, ext)
	}
	return results, nil
}

func (c *gitLabClient) listIssueNotes(ctx context.Context, projectID, issueIID int) ([]externalComment, error) {
	reqURL := c.apiURL(fmt.Sprintf("/projects/%d/issues/%d/notes?order_by=created_at&sort=asc", projectID, issueIID))
	return c.fetchNotes(ctx, reqURL)
}

func (c *gitLabClient) listMRNotes(ctx context.Context, projectID, mrIID int) ([]externalComment, error) {
	reqURL := c.apiURL(fmt.Sprintf("/projects/%d/merge_requests/%d/notes?order_by=created_at&sort=asc", projectID, mrIID))
	return c.fetchNotes(ctx, reqURL)
}

func (c *gitLabClient) fetchNotes(ctx context.Context, reqURL string) ([]externalComment, error) {
	pages, err := c.paginateGL(ctx, reqURL, 100)
	if err != nil {
		return nil, err
	}

	var results []externalComment
	for _, raw := range pages {
		var note glNote
		if err := json.Unmarshal(raw, &note); err != nil {
			continue
		}
		// Skip system-generated notes (e.g., "closed the issue", "assigned to ...")
		if note.System {
			continue
		}
		results = append(results, externalComment{
			Body:      note.Body,
			Author:    note.Author.Username,
			CreatedAt: note.CreatedAt,
		})
	}
	return results, nil
}

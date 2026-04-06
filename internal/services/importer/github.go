package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// gitHubClient handles GitHub REST API calls with rate limiting.
type gitHubClient struct {
	token      string
	httpClient *http.Client
}

func newGitHubClient(token string) *gitHubClient {
	return &gitHubClient{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ghRepo is the GitHub API repo response (partial).
type ghRepo struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	DefaultBranch string   `json:"default_branch"`
	Private       bool     `json:"private"`
	Topics        []string `json:"topics"`
}

// ghIssue is the GitHub API issue response (partial).
type ghIssue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"` // open, closed
	Labels    []ghLabel `json:"labels"`
	User      ghUser    `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at"`
	PullRequest *ghPullRef `json:"pull_request"` // non-nil if this issue is actually a PR
}

type ghLabel struct {
	Name string `json:"name"`
}

type ghUser struct {
	Login string `json:"login"`
	Email string `json:"email"`
}

type ghPullRef struct {
	URL string `json:"url"`
}

// ghPull is the GitHub API pull request response (partial).
type ghPull struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	State     string     `json:"state"`
	Head      ghRef      `json:"head"`
	Base      ghRef      `json:"base"`
	Labels    []ghLabel  `json:"labels"`
	User      ghUser     `json:"user"`
	MergedAt  *time.Time `json:"merged_at"`
	ClosedAt  *time.Time `json:"closed_at"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Merged    bool       `json:"merged"`
}

type ghRef struct {
	Ref string `json:"ref"`
}

// ghComment is the GitHub API comment response (partial).
type ghComment struct {
	Body      string    `json:"body"`
	User      ghUser    `json:"user"`
	CreatedAt time.Time `json:"created_at"`
}

func (c *gitHubClient) doRequest(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Handle rate limiting
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		resetStr := resp.Header.Get("X-RateLimit-Reset")
		resp.Body.Close()
		if resetStr != "" {
			resetUnix, _ := strconv.ParseInt(resetStr, 10, 64)
			sleepDur := time.Until(time.Unix(resetUnix, 0))
			if sleepDur > 0 && sleepDur < 5*time.Minute {
				slog.Info("github rate limit hit, sleeping", "duration", sleepDur)
				time.Sleep(sleepDur + time.Second)
				return c.doRequest(ctx, url)
			}
		}
		return nil, fmt.Errorf("github rate limit exceeded")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("github API error %d: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}

// paginate follows GitHub's Link header pagination and collects all results.
func (c *gitHubClient) paginate(ctx context.Context, baseURL string, perPage int) ([]json.RawMessage, error) {
	var all []json.RawMessage
	url := fmt.Sprintf("%s%sper_page=%d", baseURL, urlSep(baseURL), perPage)

	for url != "" {
		resp, err := c.doRequest(ctx, url)
		if err != nil {
			return all, err
		}

		var page []json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return all, fmt.Errorf("decode page: %w", err)
		}
		resp.Body.Close()

		all = append(all, page...)

		// Parse Link header for next page
		url = parseNextLink(resp.Header.Get("Link"))
	}

	return all, nil
}

func (c *gitHubClient) getRepo(ctx context.Context, owner, repo string) (*ghRepo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	resp, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var r ghRepo
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode repo: %w", err)
	}
	return &r, nil
}

func (c *gitHubClient) listIssues(ctx context.Context, owner, repo string) ([]externalIssue, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues?state=all&direction=asc", owner, repo)
	pages, err := c.paginate(ctx, url, 100)
	if err != nil {
		return nil, err
	}

	var results []externalIssue
	for _, raw := range pages {
		var iss ghIssue
		if err := json.Unmarshal(raw, &iss); err != nil {
			continue
		}

		isPR := iss.PullRequest != nil

		labels := make([]string, 0, len(iss.Labels))
		for _, l := range iss.Labels {
			labels = append(labels, l.Name)
		}

		ext := externalIssue{
			Number:    iss.Number,
			Title:     iss.Title,
			Body:      iss.Body,
			State:     iss.State,
			Labels:    labels,
			CreatedAt: iss.CreatedAt,
			UpdatedAt: iss.UpdatedAt,
			ClosedAt:  iss.ClosedAt,
			Author:    iss.User.Login,
			IsPR:      isPR,
		}

		if !isPR {
			// Fetch comments for issues only (PR comments fetched separately)
			comments, err := c.listIssueComments(ctx, owner, repo, iss.Number)
			if err != nil {
				slog.Warn("failed to fetch issue comments", "number", iss.Number, "error", err)
			} else {
				ext.Comments = comments
			}
		}

		results = append(results, ext)
	}
	return results, nil
}

func (c *gitHubClient) listPullRequests(ctx context.Context, owner, repo string) ([]externalPR, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls?state=all&direction=asc", owner, repo)
	pages, err := c.paginate(ctx, url, 100)
	if err != nil {
		return nil, err
	}

	var results []externalPR
	for _, raw := range pages {
		var pr ghPull
		if err := json.Unmarshal(raw, &pr); err != nil {
			continue
		}

		state := pr.State
		if pr.Merged || pr.MergedAt != nil {
			state = "merged"
		}

		labels := make([]string, 0, len(pr.Labels))
		for _, l := range pr.Labels {
			labels = append(labels, l.Name)
		}

		ext := externalPR{
			Number:       pr.Number,
			Title:        pr.Title,
			Body:         pr.Body,
			State:        state,
			SourceBranch: pr.Head.Ref,
			TargetBranch: pr.Base.Ref,
			Labels:       labels,
			CreatedAt:    pr.CreatedAt,
			UpdatedAt:    pr.UpdatedAt,
			MergedAt:     pr.MergedAt,
			ClosedAt:     pr.ClosedAt,
			Author:       pr.User.Login,
		}

		// Fetch PR comments
		comments, err := c.listIssueComments(ctx, owner, repo, pr.Number)
		if err != nil {
			slog.Warn("failed to fetch PR comments", "number", pr.Number, "error", err)
		} else {
			ext.Comments = comments
		}

		results = append(results, ext)
	}
	return results, nil
}

func (c *gitHubClient) listIssueComments(ctx context.Context, owner, repo string, number int) ([]externalComment, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/comments", owner, repo, number)
	pages, err := c.paginate(ctx, url, 100)
	if err != nil {
		return nil, err
	}

	var results []externalComment
	for _, raw := range pages {
		var c ghComment
		if err := json.Unmarshal(raw, &c); err != nil {
			continue
		}
		results = append(results, externalComment{
			Body:      c.Body,
			Author:    c.User.Login,
			CreatedAt: c.CreatedAt,
		})
	}
	return results, nil
}

// parseNextLink extracts the "next" URL from a GitHub Link header.
// Format: <url>; rel="next", <url>; rel="last"
func parseNextLink(header string) string {
	if header == "" {
		return ""
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, `rel="next"`) {
			start := strings.Index(part, "<")
			end := strings.Index(part, ">")
			if start >= 0 && end > start {
				return part[start+1 : end]
			}
		}
	}
	return ""
}

// urlSep returns "?" or "&" depending on whether the URL already has query params.
func urlSep(url string) string {
	if strings.Contains(url, "?") {
		return "&"
	}
	return "?"
}

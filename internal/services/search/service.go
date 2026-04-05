package search

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/git"
	"github.com/gitwise-io/gitwise/internal/services/embedding"
)

type Service struct {
	db           *pgxpool.Pool
	gitSv        *git.Service
	embeddingSvc *embedding.Service
}

func NewService(db *pgxpool.Pool, gitSvc *git.Service) *Service {
	return &Service{db: db, gitSv: gitSvc}
}

type SearchRequest struct {
	Query    string     `json:"query"`
	Scope    string     `json:"scope"`
	RepoID   *uuid.UUID `json:"repo_id,omitempty"`
	Language string     `json:"language,omitempty"`
	Limit    int        `json:"limit"`
	Offset   int        `json:"offset"`
	UserID   *uuid.UUID `json:"-"`
}

type SearchResult struct {
	Type    string         `json:"type"`
	ID      string         `json:"id"`
	Title   string         `json:"title"`
	Snippet string         `json:"snippet"`
	URL     string         `json:"url"`
	Score   float64        `json:"score"`
	Meta    map[string]any `json:"meta,omitempty"`
}

type SearchResponse struct {
	Results []SearchResult     `json:"results"`
	Facets  map[string][]Facet `json:"facets"`
	Total   int                `json:"total"`
}

type Facet struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

func (s *Service) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	if req.Query == "" {
		return &SearchResponse{Results: []SearchResult{}, Facets: map[string][]Facet{}}, nil
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 20
	}
	if req.Offset < 0 {
		req.Offset = 0
	}

	switch req.Scope {
	case "repos":
		return s.searchRepos(ctx, req.Query, req.UserID, req.Limit, req.Offset)
	case "issues":
		return s.searchIssues(ctx, req.Query, req.UserID, req.RepoID, req.Limit, req.Offset)
	case "prs":
		return s.searchPRs(ctx, req.Query, req.UserID, req.RepoID, req.Limit, req.Offset)
	case "commits":
		return s.searchCommits(ctx, req.Query, req.UserID, req.RepoID, req.Limit, req.Offset)
	case "code":
		return s.searchCode(ctx, req.Query, req.UserID, req.RepoID, req.Language, req.Limit, req.Offset)
	case "all", "":
		return s.searchAll(ctx, req)
	default:
		return nil, fmt.Errorf("unknown search scope: %s", req.Scope)
	}
}

func (s *Service) searchRepos(ctx context.Context, query string, userID *uuid.UUID, limit, offset int) (*SearchResponse, error) {
	var rows pgx.Rows
	var err error

	if userID != nil {
		rows, err = s.db.Query(ctx, `
			SELECT r.id, r.name, r.description,
				COALESCE(u.username, '') AS owner_name,
				ts_rank(to_tsvector('english', r.name || ' ' || r.description), plainto_tsquery('english', $1)) +
				similarity(r.name, $1) AS score
			FROM repositories r
			LEFT JOIN users u ON r.owner_id = u.id AND r.owner_type = 'user'
			WHERE (to_tsvector('english', r.name || ' ' || r.description) @@ plainto_tsquery('english', $1)
			   OR similarity(r.name, $1) > 0.1)
			  AND (r.visibility = 'public' OR r.owner_id = $4)
			ORDER BY score DESC
			LIMIT $2 OFFSET $3
		`, query, limit, offset, *userID)
	} else {
		rows, err = s.db.Query(ctx, `
			SELECT r.id, r.name, r.description,
				COALESCE(u.username, '') AS owner_name,
				ts_rank(to_tsvector('english', r.name || ' ' || r.description), plainto_tsquery('english', $1)) +
				similarity(r.name, $1) AS score
			FROM repositories r
			LEFT JOIN users u ON r.owner_id = u.id AND r.owner_type = 'user'
			WHERE (to_tsvector('english', r.name || ' ' || r.description) @@ plainto_tsquery('english', $1)
			   OR similarity(r.name, $1) > 0.1)
			  AND r.visibility = 'public'
			ORDER BY score DESC
			LIMIT $2 OFFSET $3
		`, query, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("search repos: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var id uuid.UUID
		var name, desc, owner string
		var score float64
		if err := rows.Scan(&id, &name, &desc, &owner, &score); err != nil {
			return nil, fmt.Errorf("scan repo result: %w", err)
		}
		results = append(results, SearchResult{
			Type:    "repo",
			ID:      id.String(),
			Title:   owner + "/" + name,
			Snippet: truncate(desc, 200),
			URL:     "/" + owner + "/" + name,
			Score:   math.Round(score*1000) / 1000,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repo rows: %w", err)
	}

	total, err := s.countRepos(ctx, query, userID)
	if err != nil {
		slog.Warn("count repos failed", "error", err)
	}

	return &SearchResponse{
		Results: coalesce(results),
		Facets:  map[string][]Facet{},
		Total:   total,
	}, nil
}

func (s *Service) countRepos(ctx context.Context, query string, userID *uuid.UUID) (int, error) {
	var count int
	var err error
	if userID != nil {
		err = s.db.QueryRow(ctx, `
			SELECT count(*) FROM repositories r
			WHERE (to_tsvector('english', r.name || ' ' || r.description) @@ plainto_tsquery('english', $1)
			   OR similarity(r.name, $1) > 0.1)
			  AND (r.visibility = 'public' OR r.owner_id = $2)
		`, query, *userID).Scan(&count)
	} else {
		err = s.db.QueryRow(ctx, `
			SELECT count(*) FROM repositories r
			WHERE (to_tsvector('english', r.name || ' ' || r.description) @@ plainto_tsquery('english', $1)
			   OR similarity(r.name, $1) > 0.1)
			  AND r.visibility = 'public'
		`, query).Scan(&count)
	}
	return count, err
}

func (s *Service) searchIssues(ctx context.Context, query string, userID *uuid.UUID, repoID *uuid.UUID, limit, offset int) (*SearchResponse, error) {
	var rows pgx.Rows
	var err error

	if repoID != nil && userID != nil {
		rows, err = s.db.Query(ctx, `
			SELECT i.id, i.number, i.title, i.body, i.status,
				COALESCE(u.username, '') AS owner_name,
				r.name AS repo_name,
				ts_rank(
					to_tsvector('english', i.title) || to_tsvector('english', i.body),
					plainto_tsquery('english', $1)
				) AS score
			FROM issues i
			JOIN repositories r ON i.repo_id = r.id
			LEFT JOIN users u ON r.owner_id = u.id AND r.owner_type = 'user'
			WHERE i.repo_id = $4
			  AND (to_tsvector('english', i.title) || to_tsvector('english', i.body)) @@ plainto_tsquery('english', $1)
			  AND (r.visibility = 'public' OR r.owner_id = $5)
			ORDER BY score DESC
			LIMIT $2 OFFSET $3
		`, query, limit, offset, *repoID, *userID)
	} else if repoID != nil {
		rows, err = s.db.Query(ctx, `
			SELECT i.id, i.number, i.title, i.body, i.status,
				COALESCE(u.username, '') AS owner_name,
				r.name AS repo_name,
				ts_rank(
					to_tsvector('english', i.title) || to_tsvector('english', i.body),
					plainto_tsquery('english', $1)
				) AS score
			FROM issues i
			JOIN repositories r ON i.repo_id = r.id
			LEFT JOIN users u ON r.owner_id = u.id AND r.owner_type = 'user'
			WHERE i.repo_id = $4
			  AND (to_tsvector('english', i.title) || to_tsvector('english', i.body)) @@ plainto_tsquery('english', $1)
			  AND r.visibility = 'public'
			ORDER BY score DESC
			LIMIT $2 OFFSET $3
		`, query, limit, offset, *repoID)
	} else if userID != nil {
		rows, err = s.db.Query(ctx, `
			SELECT i.id, i.number, i.title, i.body, i.status,
				COALESCE(u.username, '') AS owner_name,
				r.name AS repo_name,
				ts_rank(
					to_tsvector('english', i.title) || to_tsvector('english', i.body),
					plainto_tsquery('english', $1)
				) AS score
			FROM issues i
			JOIN repositories r ON i.repo_id = r.id
			LEFT JOIN users u ON r.owner_id = u.id AND r.owner_type = 'user'
			WHERE (to_tsvector('english', i.title) || to_tsvector('english', i.body)) @@ plainto_tsquery('english', $1)
			  AND (r.visibility = 'public' OR r.owner_id = $4)
			ORDER BY score DESC
			LIMIT $2 OFFSET $3
		`, query, limit, offset, *userID)
	} else {
		rows, err = s.db.Query(ctx, `
			SELECT i.id, i.number, i.title, i.body, i.status,
				COALESCE(u.username, '') AS owner_name,
				r.name AS repo_name,
				ts_rank(
					to_tsvector('english', i.title) || to_tsvector('english', i.body),
					plainto_tsquery('english', $1)
				) AS score
			FROM issues i
			JOIN repositories r ON i.repo_id = r.id
			LEFT JOIN users u ON r.owner_id = u.id AND r.owner_type = 'user'
			WHERE (to_tsvector('english', i.title) || to_tsvector('english', i.body)) @@ plainto_tsquery('english', $1)
			  AND r.visibility = 'public'
			ORDER BY score DESC
			LIMIT $2 OFFSET $3
		`, query, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("search issues: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var id uuid.UUID
		var number int
		var title, body, status, owner, repoName string
		var score float64
		if err := rows.Scan(&id, &number, &title, &body, &status, &owner, &repoName, &score); err != nil {
			return nil, fmt.Errorf("scan issue result: %w", err)
		}
		results = append(results, SearchResult{
			Type:    "issue",
			ID:      id.String(),
			Title:   title,
			Snippet: truncate(body, 200),
			URL:     fmt.Sprintf("/%s/%s/issues/%d", owner, repoName, number),
			Score:   math.Round(score*1000) / 1000,
			Meta: map[string]any{
				"number":    number,
				"status":    status,
				"repo_name": owner + "/" + repoName,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate issue rows: %w", err)
	}

	return &SearchResponse{
		Results: coalesce(results),
		Facets:  map[string][]Facet{},
		Total:   len(results),
	}, nil
}

func (s *Service) searchPRs(ctx context.Context, query string, userID *uuid.UUID, repoID *uuid.UUID, limit, offset int) (*SearchResponse, error) {
	var rows pgx.Rows
	var err error

	if repoID != nil && userID != nil {
		rows, err = s.db.Query(ctx, `
			SELECT p.id, p.number, p.title, p.body, p.status,
				COALESCE(u.username, '') AS owner_name,
				r.name AS repo_name,
				ts_rank(
					to_tsvector('english', p.title) || to_tsvector('english', p.body),
					plainto_tsquery('english', $1)
				) AS score
			FROM pull_requests p
			JOIN repositories r ON p.repo_id = r.id
			LEFT JOIN users u ON r.owner_id = u.id AND r.owner_type = 'user'
			WHERE p.repo_id = $4
			  AND (to_tsvector('english', p.title) || to_tsvector('english', p.body)) @@ plainto_tsquery('english', $1)
			  AND (r.visibility = 'public' OR r.owner_id = $5)
			ORDER BY score DESC
			LIMIT $2 OFFSET $3
		`, query, limit, offset, *repoID, *userID)
	} else if repoID != nil {
		rows, err = s.db.Query(ctx, `
			SELECT p.id, p.number, p.title, p.body, p.status,
				COALESCE(u.username, '') AS owner_name,
				r.name AS repo_name,
				ts_rank(
					to_tsvector('english', p.title) || to_tsvector('english', p.body),
					plainto_tsquery('english', $1)
				) AS score
			FROM pull_requests p
			JOIN repositories r ON p.repo_id = r.id
			LEFT JOIN users u ON r.owner_id = u.id AND r.owner_type = 'user'
			WHERE p.repo_id = $4
			  AND (to_tsvector('english', p.title) || to_tsvector('english', p.body)) @@ plainto_tsquery('english', $1)
			  AND r.visibility = 'public'
			ORDER BY score DESC
			LIMIT $2 OFFSET $3
		`, query, limit, offset, *repoID)
	} else if userID != nil {
		rows, err = s.db.Query(ctx, `
			SELECT p.id, p.number, p.title, p.body, p.status,
				COALESCE(u.username, '') AS owner_name,
				r.name AS repo_name,
				ts_rank(
					to_tsvector('english', p.title) || to_tsvector('english', p.body),
					plainto_tsquery('english', $1)
				) AS score
			FROM pull_requests p
			JOIN repositories r ON p.repo_id = r.id
			LEFT JOIN users u ON r.owner_id = u.id AND r.owner_type = 'user'
			WHERE (to_tsvector('english', p.title) || to_tsvector('english', p.body)) @@ plainto_tsquery('english', $1)
			  AND (r.visibility = 'public' OR r.owner_id = $4)
			ORDER BY score DESC
			LIMIT $2 OFFSET $3
		`, query, limit, offset, *userID)
	} else {
		rows, err = s.db.Query(ctx, `
			SELECT p.id, p.number, p.title, p.body, p.status,
				COALESCE(u.username, '') AS owner_name,
				r.name AS repo_name,
				ts_rank(
					to_tsvector('english', p.title) || to_tsvector('english', p.body),
					plainto_tsquery('english', $1)
				) AS score
			FROM pull_requests p
			JOIN repositories r ON p.repo_id = r.id
			LEFT JOIN users u ON r.owner_id = u.id AND r.owner_type = 'user'
			WHERE (to_tsvector('english', p.title) || to_tsvector('english', p.body)) @@ plainto_tsquery('english', $1)
			  AND r.visibility = 'public'
			ORDER BY score DESC
			LIMIT $2 OFFSET $3
		`, query, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("search prs: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var id uuid.UUID
		var number int
		var title, body, status, owner, repoName string
		var score float64
		if err := rows.Scan(&id, &number, &title, &body, &status, &owner, &repoName, &score); err != nil {
			return nil, fmt.Errorf("scan pr result: %w", err)
		}
		results = append(results, SearchResult{
			Type:    "pr",
			ID:      id.String(),
			Title:   title,
			Snippet: truncate(body, 200),
			URL:     fmt.Sprintf("/%s/%s/pulls/%d", owner, repoName, number),
			Score:   math.Round(score*1000) / 1000,
			Meta: map[string]any{
				"number":    number,
				"status":    status,
				"repo_name": owner + "/" + repoName,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pr rows: %w", err)
	}

	return &SearchResponse{
		Results: coalesce(results),
		Facets:  map[string][]Facet{},
		Total:   len(results),
	}, nil
}

func (s *Service) searchCommits(ctx context.Context, query string, userID *uuid.UUID, repoID *uuid.UUID, limit, offset int) (*SearchResponse, error) {
	var rows pgx.Rows
	var err error

	if repoID != nil && userID != nil {
		rows, err = s.db.Query(ctx, `
			SELECT c.sha, c.message, c.author_email,
				COALESCE(u.username, '') AS owner_name,
				r.name AS repo_name,
				ts_rank(to_tsvector('english', c.message), plainto_tsquery('english', $1)) AS score
			FROM commit_metadata c
			JOIN repositories r ON c.repo_id = r.id
			LEFT JOIN users u ON r.owner_id = u.id AND r.owner_type = 'user'
			WHERE c.repo_id = $4
			  AND to_tsvector('english', c.message) @@ plainto_tsquery('english', $1)
			  AND (r.visibility = 'public' OR r.owner_id = $5)
			ORDER BY score DESC
			LIMIT $2 OFFSET $3
		`, query, limit, offset, *repoID, *userID)
	} else if repoID != nil {
		rows, err = s.db.Query(ctx, `
			SELECT c.sha, c.message, c.author_email,
				COALESCE(u.username, '') AS owner_name,
				r.name AS repo_name,
				ts_rank(to_tsvector('english', c.message), plainto_tsquery('english', $1)) AS score
			FROM commit_metadata c
			JOIN repositories r ON c.repo_id = r.id
			LEFT JOIN users u ON r.owner_id = u.id AND r.owner_type = 'user'
			WHERE c.repo_id = $4
			  AND to_tsvector('english', c.message) @@ plainto_tsquery('english', $1)
			  AND r.visibility = 'public'
			ORDER BY score DESC
			LIMIT $2 OFFSET $3
		`, query, limit, offset, *repoID)
	} else if userID != nil {
		rows, err = s.db.Query(ctx, `
			SELECT c.sha, c.message, c.author_email,
				COALESCE(u.username, '') AS owner_name,
				r.name AS repo_name,
				ts_rank(to_tsvector('english', c.message), plainto_tsquery('english', $1)) AS score
			FROM commit_metadata c
			JOIN repositories r ON c.repo_id = r.id
			LEFT JOIN users u ON r.owner_id = u.id AND r.owner_type = 'user'
			WHERE to_tsvector('english', c.message) @@ plainto_tsquery('english', $1)
			  AND (r.visibility = 'public' OR r.owner_id = $4)
			ORDER BY score DESC
			LIMIT $2 OFFSET $3
		`, query, limit, offset, *userID)
	} else {
		rows, err = s.db.Query(ctx, `
			SELECT c.sha, c.message, c.author_email,
				COALESCE(u.username, '') AS owner_name,
				r.name AS repo_name,
				ts_rank(to_tsvector('english', c.message), plainto_tsquery('english', $1)) AS score
			FROM commit_metadata c
			JOIN repositories r ON c.repo_id = r.id
			LEFT JOIN users u ON r.owner_id = u.id AND r.owner_type = 'user'
			WHERE to_tsvector('english', c.message) @@ plainto_tsquery('english', $1)
			  AND r.visibility = 'public'
			ORDER BY score DESC
			LIMIT $2 OFFSET $3
		`, query, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("search commits: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var sha, message, authorEmail, owner, repoName string
		var score float64
		if err := rows.Scan(&sha, &message, &authorEmail, &owner, &repoName, &score); err != nil {
			return nil, fmt.Errorf("scan commit result: %w", err)
		}

		firstLine := message
		if idx := strings.IndexByte(message, '\n'); idx > 0 {
			firstLine = message[:idx]
		}

		results = append(results, SearchResult{
			Type:    "commit",
			ID:      sha,
			Title:   truncate(firstLine, 120),
			Snippet: truncate(message, 200),
			URL:     fmt.Sprintf("/%s/%s/commits/%s", owner, repoName, sha),
			Score:   math.Round(score*1000) / 1000,
			Meta: map[string]any{
				"sha_short":    sha[:7],
				"author_email": authorEmail,
				"repo_name":    owner + "/" + repoName,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate commit rows: %w", err)
	}

	return &SearchResponse{
		Results: coalesce(results),
		Facets:  map[string][]Facet{},
		Total:   len(results),
	}, nil
}

func (s *Service) searchCode(ctx context.Context, query string, userID *uuid.UUID, repoID *uuid.UUID, language string, limit, offset int) (*SearchResponse, error) {
	if len(query) < 3 {
		return &SearchResponse{Results: []SearchResult{}, Facets: map[string][]Facet{}}, nil
	}

	var rows pgx.Rows
	var err error

	escaped := escapeLike(query)
	args := []any{escaped, limit, offset, query}
	qb := `
		SELECT cf.id, cf.path, cf.content, cf.language,
			COALESCE(u.username, '') AS owner_name,
			r.name AS repo_name,
			similarity(cf.content, $4) AS score
		FROM code_files cf
		JOIN repositories r ON cf.repo_id = r.id
		LEFT JOIN users u ON r.owner_id = u.id AND r.owner_type = 'user'
		WHERE cf.content ILIKE '%' || $1 || '%'`

	paramIdx := 5
	if userID != nil {
		qb += fmt.Sprintf(" AND (r.visibility = 'public' OR r.owner_id = $%d)", paramIdx)
		args = append(args, *userID)
		paramIdx++
	} else {
		qb += " AND r.visibility = 'public'"
	}
	if repoID != nil {
		qb += fmt.Sprintf(" AND cf.repo_id = $%d", paramIdx)
		args = append(args, *repoID)
		paramIdx++
	}
	if language != "" {
		qb += fmt.Sprintf(" AND cf.language = $%d", paramIdx)
		args = append(args, language)
		paramIdx++
	}

	qb += ` ORDER BY score DESC LIMIT $2 OFFSET $3`

	rows, err = s.db.Query(ctx, qb, args...)
	if err != nil {
		return nil, fmt.Errorf("search code: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var id uuid.UUID
		var path, content, lang, owner, repoName string
		var score float64
		if err := rows.Scan(&id, &path, &content, &lang, &owner, &repoName, &score); err != nil {
			return nil, fmt.Errorf("scan code result: %w", err)
		}

		snippet := extractCodeSnippet(content, query, 3)
		results = append(results, SearchResult{
			Type:    "code",
			ID:      id.String(),
			Title:   path,
			Snippet: snippet,
			URL:     fmt.Sprintf("/%s/%s/blob/HEAD/%s", owner, repoName, path),
			Score:   math.Round(score*1000) / 1000,
			Meta: map[string]any{
				"language":  lang,
				"repo_name": owner + "/" + repoName,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate code rows: %w", err)
	}

	// Language facets for code scope
	facets := map[string][]Facet{}
	langFacets, err := s.codeLangFacets(ctx, query, userID, repoID)
	if err == nil && len(langFacets) > 0 {
		facets["language"] = langFacets
	}

	return &SearchResponse{
		Results: coalesce(results),
		Facets:  facets,
		Total:   len(results),
	}, nil
}

func (s *Service) codeLangFacets(ctx context.Context, query string, userID *uuid.UUID, repoID *uuid.UUID) ([]Facet, error) {
	var rows pgx.Rows
	var err error

	escaped := escapeLike(query)
	args := []any{escaped}
	qb := `
		SELECT cf.language, count(*) AS cnt
		FROM code_files cf
		JOIN repositories r ON cf.repo_id = r.id
		WHERE cf.content ILIKE '%' || $1 || '%' AND cf.language != ''`

	paramIdx := 2
	if userID != nil {
		qb += fmt.Sprintf(" AND (r.visibility = 'public' OR r.owner_id = $%d)", paramIdx)
		args = append(args, *userID)
		paramIdx++
	} else {
		qb += " AND r.visibility = 'public'"
	}
	if repoID != nil {
		qb += fmt.Sprintf(" AND cf.repo_id = $%d", paramIdx)
		args = append(args, *repoID)
		paramIdx++
	}

	qb += ` GROUP BY cf.language ORDER BY cnt DESC LIMIT 20`

	rows, err = s.db.Query(ctx, qb, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var facets []Facet
	for rows.Next() {
		var lang string
		var count int
		if err := rows.Scan(&lang, &count); err != nil {
			return nil, err
		}
		facets = append(facets, Facet{Value: lang, Count: count})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lang facet rows: %w", err)
	}
	return facets, nil
}

func (s *Service) searchAll(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	perScope := 5
	if req.Limit > 5 {
		perScope = req.Limit / 4
	}

	type scopeResult struct {
		resp *SearchResponse
		err  error
	}

	ch := make(chan scopeResult, 5)

	go func() {
		r, e := s.searchRepos(ctx, req.Query, req.UserID, perScope, 0)
		ch <- scopeResult{r, e}
	}()
	go func() {
		r, e := s.searchIssues(ctx, req.Query, req.UserID, req.RepoID, perScope, 0)
		ch <- scopeResult{r, e}
	}()
	go func() {
		r, e := s.searchPRs(ctx, req.Query, req.UserID, req.RepoID, perScope, 0)
		ch <- scopeResult{r, e}
	}()
	go func() {
		r, e := s.searchCommits(ctx, req.Query, req.UserID, req.RepoID, perScope, 0)
		ch <- scopeResult{r, e}
	}()
	go func() {
		r, e := s.searchCode(ctx, req.Query, req.UserID, req.RepoID, req.Language, perScope, 0)
		ch <- scopeResult{r, e}
	}()

	var allResults []SearchResult
	typeCounts := map[string]int{}
	totalSum := 0

	for i := 0; i < 5; i++ {
		sr := <-ch
		if sr.err != nil {
			slog.Warn("search scope failed", "error", sr.err)
			continue
		}
		totalSum += sr.resp.Total
		for _, r := range sr.resp.Results {
			typeCounts[r.Type]++
			allResults = append(allResults, r)
		}
	}

	// Sort merged results by score descending
	sortByScore(allResults)

	// Trim to requested limit
	if len(allResults) > req.Limit {
		allResults = allResults[:req.Limit]
	}

	// Type facets
	var typeFacets []Facet
	for t, c := range typeCounts {
		typeFacets = append(typeFacets, Facet{Value: t, Count: c})
	}

	facets := map[string][]Facet{}
	if len(typeFacets) > 0 {
		facets["type"] = typeFacets
	}

	return &SearchResponse{
		Results: coalesce(allResults),
		Facets:  facets,
		Total:   totalSum,
	}, nil
}

// IndexRepo indexes all files in a repository's HEAD into the code_files table.
func (s *Service) IndexRepo(ctx context.Context, repoID uuid.UUID, owner, name string) error {
	entries, err := s.listAllFiles(owner, name, "HEAD", "")
	if err != nil {
		return fmt.Errorf("list files: %w", err)
	}

	// Delete existing entries for this repo+ref
	_, err = s.db.Exec(ctx, `DELETE FROM code_files WHERE repo_id = $1 AND ref = 'HEAD'`, repoID)
	if err != nil {
		return fmt.Errorf("delete old index: %w", err)
	}

	batch := 0
	for _, entry := range entries {
		if entry.Type != "blob" {
			continue
		}
		if entry.Size > 512*1024 { // skip files >512KB
			continue
		}

		blob, err := s.gitSv.GetBlob(owner, name, "HEAD", entry.Path)
		if err != nil {
			slog.Warn("skip file in index", "path", entry.Path, "error", err)
			continue
		}
		if blob.IsBinary {
			continue
		}

		lang := detectLanguage(entry.Path)

		_, err = s.db.Exec(ctx, `
			INSERT INTO code_files (repo_id, ref, path, content, language)
			VALUES ($1, 'HEAD', $2, $3, $4)
			ON CONFLICT (repo_id, ref, path) DO UPDATE SET
				content = EXCLUDED.content,
				language = EXCLUDED.language,
				indexed_at = now()
		`, repoID, entry.Path, blob.Content, lang)
		if err != nil {
			slog.Warn("index file failed", "path", entry.Path, "error", err)
			continue
		}
		batch++
	}

	slog.Info("indexed repo", "repo", owner+"/"+name, "files", batch)
	return nil
}

// listAllFiles recursively lists all files in a repo tree.
func (s *Service) listAllFiles(owner, name, ref, treePath string) ([]fileEntry, error) {
	entries, err := s.gitSv.ListTree(owner, name, ref, treePath)
	if err != nil {
		return nil, err
	}

	var all []fileEntry
	for _, e := range entries {
		if e.Type == "blob" {
			all = append(all, fileEntry{Path: e.Path, Type: "blob", Size: e.Size})
		} else if e.Type == "tree" {
			sub, err := s.listAllFiles(owner, name, ref, e.Path)
			if err != nil {
				slog.Warn("skip subtree", "path", e.Path, "error", err)
				continue
			}
			all = append(all, sub...)
		}
	}
	return all, nil
}

type fileEntry struct {
	Path string
	Type string
	Size int64
}

// extractCodeSnippet finds the query in content and returns surrounding lines.
func extractCodeSnippet(content, query string, contextLines int) string {
	lines := strings.Split(content, "\n")
	queryLower := strings.ToLower(query)

	matchIdx := -1
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), queryLower) {
			matchIdx = i
			break
		}
	}

	if matchIdx < 0 {
		// Fallback: return first few lines
		end := len(lines)
		if end > contextLines*2+1 {
			end = contextLines*2 + 1
		}
		return strings.Join(lines[:end], "\n")
	}

	start := matchIdx - contextLines
	if start < 0 {
		start = 0
	}
	end := matchIdx + contextLines + 1
	if end > len(lines) {
		end = len(lines)
	}

	return strings.Join(lines[start:end], "\n")
}

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "Go"
	case ".js":
		return "JavaScript"
	case ".ts":
		return "TypeScript"
	case ".tsx":
		return "TypeScript"
	case ".jsx":
		return "JavaScript"
	case ".py":
		return "Python"
	case ".rs":
		return "Rust"
	case ".rb":
		return "Ruby"
	case ".java":
		return "Java"
	case ".c", ".h":
		return "C"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "C++"
	case ".cs":
		return "C#"
	case ".swift":
		return "Swift"
	case ".kt":
		return "Kotlin"
	case ".php":
		return "PHP"
	case ".sql":
		return "SQL"
	case ".html", ".htm":
		return "HTML"
	case ".css":
		return "CSS"
	case ".scss", ".sass":
		return "SCSS"
	case ".json":
		return "JSON"
	case ".yaml", ".yml":
		return "YAML"
	case ".toml":
		return "TOML"
	case ".xml":
		return "XML"
	case ".md", ".markdown":
		return "Markdown"
	case ".sh", ".bash":
		return "Shell"
	case ".dockerfile":
		return "Dockerfile"
	case ".lua":
		return "Lua"
	case ".r":
		return "R"
	case ".scala":
		return "Scala"
	case ".ex", ".exs":
		return "Elixir"
	case ".zig":
		return "Zig"
	default:
		base := strings.ToLower(filepath.Base(path))
		switch base {
		case "dockerfile":
			return "Dockerfile"
		case "makefile":
			return "Makefile"
		}
		return ""
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func coalesce(results []SearchResult) []SearchResult {
	if results == nil {
		return []SearchResult{}
	}
	return results
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func sortByScore(results []SearchResult) {
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].Score > results[j-1].Score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
}

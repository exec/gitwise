import { useState, useEffect } from "react";
import { useParams, Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { get } from "../lib/api";
import RepoHeader from "../components/RepoHeader";

interface Issue {
  id: string;
  number: number;
  author_name: string;
  title: string;
  status: string;
  labels: string[];
  priority: string;
  created_at: string;
}

export default function IssueListPage() {
  const { owner, repo } = useParams();

  const statusFilter =
    new URLSearchParams(window.location.search).get("status") ?? "open";

  const [cursor, setCursor] = useState<string>("");
  const [items, setItems] = useState<Issue[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);

  // Reset accumulated state when filters change
  useEffect(() => {
    setCursor("");
    setItems([]);
    setNextCursor(null);
  }, [owner, repo, statusFilter]);

  const issuesQuery = useQuery({
    queryKey: ["issues", owner, repo, statusFilter, cursor],
    queryFn: async () => {
      const path = `/repos/${owner}/${repo}/issues?status=${statusFilter}&limit=50${cursor ? `&cursor=${cursor}` : ""}`;
      const r = await get<Issue[]>(path);
      return { data: r.data, meta: r.meta };
    },
    enabled: !!owner && !!repo,
  });

  useEffect(() => {
    if (issuesQuery.data) {
      const newItems = issuesQuery.data.data ?? [];
      if (cursor) {
        setItems((prev) => [...prev, ...newItems]);
      } else {
        setItems(newItems);
      }
      setNextCursor(
        (issuesQuery.data.meta?.next_cursor as string) ?? null,
      );
    }
  }, [issuesQuery.data]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="repo-page">
      <RepoHeader owner={owner!} repo={repo!} activeTab="issues" />

      <div className="issue-list-controls">
        <div className="status-tabs">
          <Link
            to={`/${owner}/${repo}/issues?status=open`}
            className={`status-tab ${statusFilter === "open" ? "active" : ""}`}
          >
            Open
          </Link>
          <Link
            to={`/${owner}/${repo}/issues?status=closed`}
            className={`status-tab ${statusFilter === "closed" ? "active" : ""}`}
          >
            Closed
          </Link>
        </div>
        <Link to={`/${owner}/${repo}/issues/new`} className="btn btn-primary">
          New Issue
        </Link>
      </div>

      {issuesQuery.isLoading && <p className="muted">Loading issues...</p>}
      {issuesQuery.error && (
        <div className="error-banner">
          {issuesQuery.error instanceof Error
            ? issuesQuery.error.message
            : "Failed to load issues"}
        </div>
      )}

      {!issuesQuery.isLoading && items.length === 0 && (
        <p className="muted">No {statusFilter} issues.</p>
      )}

      {items.length > 0 && (
        <ul className="issue-list">
          {items.map((issue) => (
            <li key={issue.id} className="issue-item">
              <div className="issue-title-row">
                <span
                  className={`issue-status-dot ${issue.status === "open" ? "open" : "closed"}`}
                />
                <Link
                  to={`/${owner}/${repo}/issues/${issue.number}`}
                  className="issue-title-link"
                >
                  {issue.title}
                </Link>
                {issue.labels.map((l) => (
                  <span key={l} className="label-badge">
                    {l}
                  </span>
                ))}
              </div>
              <div className="issue-meta">
                #{issue.number} opened by <Link to={`/${issue.author_name}`} className="author-link">{issue.author_name}</Link> on{" "}
                {new Date(issue.created_at).toLocaleDateString()}
                {issue.priority !== "none" && (
                  <span className={`priority-badge priority-${issue.priority}`}>
                    {issue.priority}
                  </span>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}

      {nextCursor && (
        <div style={{ textAlign: "center", padding: "16px 0" }}>
          <button className="btn btn-secondary" onClick={() => setCursor(nextCursor)}>
            Load More
          </button>
        </div>
      )}
    </div>
  );
}

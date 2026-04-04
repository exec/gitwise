import { useState, useEffect } from "react";
import { useParams, Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { get } from "../lib/api";
import RepoHeader from "../components/RepoHeader";

interface PullRequest {
  id: string;
  number: number;
  author_name: string;
  title: string;
  source_branch: string;
  target_branch: string;
  status: string;
  created_at: string;
}

export default function PullListPage() {
  const { owner, repo } = useParams();

  const statusFilter =
    new URLSearchParams(window.location.search).get("status") ?? "open";

  const [cursor, setCursor] = useState<string>("");
  const [items, setItems] = useState<PullRequest[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);

  // Reset accumulated state when filters change
  useEffect(() => {
    setCursor("");
    setItems([]);
    setNextCursor(null);
  }, [owner, repo, statusFilter]);

  const prsQuery = useQuery({
    queryKey: ["pulls", owner, repo, statusFilter, cursor],
    queryFn: async () => {
      const path = `/repos/${owner}/${repo}/pulls?status=${statusFilter}&limit=50${cursor ? `&cursor=${cursor}` : ""}`;
      const r = await get<PullRequest[]>(path);
      return { data: r.data, meta: r.meta };
    },
    enabled: !!owner && !!repo,
  });

  useEffect(() => {
    if (prsQuery.data) {
      const newItems = prsQuery.data.data;
      if (cursor) {
        setItems((prev) => [...prev, ...newItems]);
      } else {
        setItems(newItems);
      }
      setNextCursor(
        (prsQuery.data.meta?.next_cursor as string) ?? null,
      );
    }
  }, [prsQuery.data]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="repo-page">
      <RepoHeader owner={owner!} repo={repo!} activeTab="pulls" />

      <div className="issue-list-controls">
        <div className="status-tabs">
          <Link
            to={`/${owner}/${repo}/pulls?status=open`}
            className={`status-tab ${statusFilter === "open" ? "active" : ""}`}
          >
            Open
          </Link>
          <Link
            to={`/${owner}/${repo}/pulls?status=merged`}
            className={`status-tab ${statusFilter === "merged" ? "active" : ""}`}
          >
            Merged
          </Link>
          <Link
            to={`/${owner}/${repo}/pulls?status=closed`}
            className={`status-tab ${statusFilter === "closed" ? "active" : ""}`}
          >
            Closed
          </Link>
        </div>
        <Link to={`/${owner}/${repo}/pulls/new`} className="btn btn-primary">
          New Pull Request
        </Link>
      </div>

      {prsQuery.isLoading && (
        <p className="muted">Loading pull requests...</p>
      )}
      {prsQuery.error && (
        <div className="error-banner">
          {prsQuery.error instanceof Error
            ? prsQuery.error.message
            : "Failed to load pull requests"}
        </div>
      )}

      {!prsQuery.isLoading && items.length === 0 && (
        <p className="muted">No {statusFilter} pull requests.</p>
      )}

      {items.length > 0 && (
        <ul className="issue-list">
          {items.map((pr) => (
            <li key={pr.id} className="issue-item">
              <div className="issue-title-row">
                <span
                  className={`pr-status-dot ${pr.status === "merged" ? "merged" : pr.status === "open" ? "open" : "closed"}`}
                />
                <Link
                  to={`/${owner}/${repo}/pulls/${pr.number}`}
                  className="issue-title-link"
                >
                  {pr.title}
                </Link>
              </div>
              <div className="issue-meta">
                #{pr.number} by {pr.author_name} &middot;{" "}
                {pr.source_branch} &rarr; {pr.target_branch} &middot;{" "}
                {new Date(pr.created_at).toLocaleDateString()}
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

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

  const prsQuery = useQuery({
    queryKey: ["pulls", owner, repo, statusFilter],
    queryFn: () =>
      get<PullRequest[]>(
        `/repos/${owner}/${repo}/pulls?status=${statusFilter}&limit=50`,
      ).then((r) => r.data),
    enabled: !!owner && !!repo,
  });

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

      {prsQuery.data && prsQuery.data.length === 0 && (
        <p className="muted">No {statusFilter} pull requests.</p>
      )}

      {prsQuery.data && prsQuery.data.length > 0 && (
        <ul className="issue-list">
          {prsQuery.data.map((pr) => (
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
    </div>
  );
}

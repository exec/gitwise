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

  const issuesQuery = useQuery({
    queryKey: ["issues", owner, repo, statusFilter],
    queryFn: () =>
      get<Issue[]>(
        `/repos/${owner}/${repo}/issues?status=${statusFilter}&limit=50`,
      ).then((r) => r.data),
    enabled: !!owner && !!repo,
  });

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

      {issuesQuery.data && issuesQuery.data.length === 0 && (
        <p className="muted">No {statusFilter} issues.</p>
      )}

      {issuesQuery.data && issuesQuery.data.length > 0 && (
        <ul className="issue-list">
          {issuesQuery.data.map((issue) => (
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
                #{issue.number} opened by {issue.author_name} on{" "}
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
    </div>
  );
}

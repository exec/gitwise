import { useParams } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { get, post, patch } from "../lib/api";
import { useAuthStore } from "../stores/auth";
import RepoHeader from "../components/RepoHeader";

interface Issue {
  id: string;
  number: number;
  author_id: string;
  author_name: string;
  title: string;
  body: string;
  status: string;
  labels: string[];
  priority: string;
  created_at: string;
  updated_at: string;
}

interface Comment {
  id: string;
  author_name: string;
  body: string;
  created_at: string;
}

export default function IssueDetailPage() {
  const { owner, repo, number } = useParams();
  const user = useAuthStore((s) => s.user);
  const queryClient = useQueryClient();
  const [commentBody, setCommentBody] = useState("");

  const issueQuery = useQuery({
    queryKey: ["issue", owner, repo, number],
    queryFn: () =>
      get<Issue>(`/repos/${owner}/${repo}/issues/${number}`).then(
        (r) => r.data,
      ),
    enabled: !!owner && !!repo && !!number,
  });

  const commentsQuery = useQuery({
    queryKey: ["issue-comments", owner, repo, number],
    queryFn: () =>
      get<Comment[]>(
        `/repos/${owner}/${repo}/issues/${number}/comments`,
      ).then((r) => r.data),
    enabled: !!owner && !!repo && !!number,
  });

  const addComment = useMutation({
    mutationFn: (body: string) =>
      post(`/repos/${owner}/${repo}/issues/${number}/comments`, { body }),
    onSuccess: () => {
      setCommentBody("");
      queryClient.invalidateQueries({
        queryKey: ["issue-comments", owner, repo, number],
      });
    },
  });

  const toggleStatus = useMutation({
    mutationFn: (newStatus: string) =>
      patch(`/repos/${owner}/${repo}/issues/${number}`, {
        status: newStatus,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["issue", owner, repo, number],
      });
    },
  });

  if (issueQuery.isLoading)
    return <p className="muted">Loading issue...</p>;
  if (issueQuery.error)
    return (
      <div className="error-banner">
        {issueQuery.error instanceof Error
          ? issueQuery.error.message
          : "Failed to load issue"}
      </div>
    );

  const issue = issueQuery.data;
  if (!issue) return null;

  return (
    <div className="repo-page">
      <RepoHeader owner={owner!} repo={repo!} activeTab="issues" />

      <div className="issue-detail">
        <div className="issue-header">
          <h2>
            {issue.title}{" "}
            <span className="issue-number">#{issue.number}</span>
          </h2>
          <div className="issue-status-bar">
            <span
              className={`issue-status-badge ${issue.status === "open" ? "open" : "closed"}`}
            >
              {issue.status}
            </span>
            <span className="issue-meta-info">
              {issue.author_name} opened on{" "}
              {new Date(issue.created_at).toLocaleDateString()}
            </span>
          </div>
        </div>

        {issue.labels.length > 0 && (
          <div className="issue-labels">
            {issue.labels.map((l) => (
              <span key={l} className="label-badge">
                {l}
              </span>
            ))}
          </div>
        )}

        {issue.body && (
          <div className="comment-body issue-body">
            <pre className="markdown-body">{issue.body}</pre>
          </div>
        )}

        <div className="comments-section">
          <h3>Comments</h3>
          {commentsQuery.isLoading && (
            <p className="muted">Loading comments...</p>
          )}
          {commentsQuery.data && commentsQuery.data.length === 0 && (
            <p className="muted">No comments yet.</p>
          )}
          {commentsQuery.data?.map((c) => (
            <div key={c.id} className="comment-card">
              <div className="comment-header">
                <strong>{c.author_name}</strong>
                <span className="comment-date">
                  {new Date(c.created_at).toLocaleDateString()}
                </span>
              </div>
              <div className="comment-body">
                <pre className="markdown-body">{c.body}</pre>
              </div>
            </div>
          ))}

          {user && (
            <div className="comment-form">
              <textarea
                className="comment-input"
                placeholder="Leave a comment..."
                value={commentBody}
                onChange={(e) => setCommentBody(e.target.value)}
                rows={4}
              />
              <div className="comment-actions">
                {issue.status === "open" && (
                  <button
                    className="btn btn-secondary"
                    onClick={() => toggleStatus.mutate("closed")}
                    disabled={toggleStatus.isPending}
                  >
                    Close issue
                  </button>
                )}
                {issue.status === "closed" && (
                  <button
                    className="btn btn-secondary"
                    onClick={() => toggleStatus.mutate("open")}
                    disabled={toggleStatus.isPending}
                  >
                    Reopen issue
                  </button>
                )}
                <button
                  className="btn btn-primary"
                  disabled={!commentBody.trim() || addComment.isPending}
                  onClick={() => addComment.mutate(commentBody)}
                >
                  Comment
                </button>
              </div>
              {addComment.error && (
                <div className="error-banner">
                  {addComment.error instanceof Error
                    ? addComment.error.message
                    : "Failed to add comment"}
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

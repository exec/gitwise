import { useParams, Link } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { get, post, patch } from "../lib/api";
import { useAuthStore } from "../stores/auth";
import RepoHeader from "../components/RepoHeader";
import Markdown from "../components/Markdown";

interface Issue {
  id: string;
  number: number;
  author_id: string;
  author_name: string;
  title: string;
  body: string;
  status: string;
  labels: string[];
  assignees: string[];
  priority: string;
  created_at: string;
  updated_at: string;
}

interface Comment {
  id: string;
  author_id: string;
  author_name: string;
  body: string;
  created_at: string;
}

export default function IssueDetailPage() {
  const { owner, repo, number } = useParams();
  const user = useAuthStore((s) => s.user);
  const queryClient = useQueryClient();
  const [commentBody, setCommentBody] = useState("");

  const [isEditing, setIsEditing] = useState(false);
  const [editTitle, setEditTitle] = useState("");
  const [editBody, setEditBody] = useState("");
  const [labelInput, setLabelInput] = useState("");

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

  const updateIssue = useMutation({
    mutationFn: (data: { title?: string; body?: string; labels?: string[] }) =>
      patch(`/repos/${owner}/${repo}/issues/${number}`, data),
    onSuccess: () => {
      setIsEditing(false);
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

  const canEditIssue = user && (user.id === issue.author_id || user.username === owner);

  function startEditing() {
    setEditTitle(issue!.title);
    setEditBody(issue!.body || "");
    setLabelInput((issue!.labels || []).join(", "));
    setIsEditing(true);
  }

  function saveEdit() {
    const labels = labelInput
      .split(",")
      .map((l) => l.trim())
      .filter(Boolean);
    updateIssue.mutate({ title: editTitle, body: editBody, labels });
  }

  return (
    <div className="repo-page">
      <RepoHeader owner={owner!} repo={repo!} activeTab="issues" />

      <div className="issue-detail">
        <div className="issue-header">
          {isEditing ? (
            <div className="edit-form">
              <input
                className="edit-input"
                value={editTitle}
                onChange={(e) => setEditTitle(e.target.value)}
              />
              <textarea
                className="edit-textarea"
                value={editBody}
                onChange={(e) => setEditBody(e.target.value)}
                rows={8}
              />
              <div className="label-selector">
                <label>Labels (comma-separated):</label>
                <input
                  className="edit-input"
                  value={labelInput}
                  onChange={(e) => setLabelInput(e.target.value)}
                  placeholder="bug, enhancement, help wanted"
                />
              </div>
              <div className="edit-actions">
                <button
                  className="btn btn-primary"
                  onClick={saveEdit}
                  disabled={!editTitle.trim() || updateIssue.isPending}
                >
                  Save
                </button>
                <button
                  className="btn btn-secondary"
                  onClick={() => setIsEditing(false)}
                >
                  Cancel
                </button>
              </div>
              {updateIssue.error && (
                <div className="error-banner">
                  {updateIssue.error instanceof Error
                    ? updateIssue.error.message
                    : "Failed to update issue"}
                </div>
              )}
            </div>
          ) : (
            <>
              <h2>
                {issue.title}{" "}
                <span className="issue-number">#{issue.number}</span>
                {canEditIssue && (
                  <button
                    className="btn btn-secondary btn-sm"
                    onClick={startEditing}
                    style={{ marginLeft: 8, verticalAlign: "middle" }}
                  >
                    Edit
                  </button>
                )}
              </h2>
              <div className="issue-status-bar">
                <span
                  className={`issue-status-badge ${issue.status === "open" ? "open" : "closed"}`}
                >
                  {issue.status}
                </span>
                <span className="issue-meta-info">
                  <Link to={`/${issue.author_name}`} className="author-link">{issue.author_name}</Link> opened on{" "}
                  {new Date(issue.created_at).toLocaleDateString()}
                </span>
              </div>
            </>
          )}
        </div>

        {!isEditing && issue.labels.length > 0 && (
          <div className="issue-labels">
            {issue.labels.map((l) => (
              <span key={l} className="label-badge">
                {l}
              </span>
            ))}
          </div>
        )}

        {issue.assignees && issue.assignees.length > 0 && (
          <div className="issue-assignees">
            <span className="muted">
              {issue.assignees.length} assignee
              {issue.assignees.length !== 1 ? "s" : ""}
            </span>
          </div>
        )}

        {!isEditing && issue.body && (
          <div className="comment-body issue-body">
            <Markdown content={issue.body} />
          </div>
        )}

        {user && !isEditing && (
          <div style={{ marginTop: 12, marginBottom: 12 }}>
            {issue.status === "open" && (
              <button
                className="close-reopen-btn btn btn-secondary"
                onClick={() => toggleStatus.mutate("closed")}
                disabled={toggleStatus.isPending}
              >
                Close issue
              </button>
            )}
            {issue.status === "closed" && (
              <button
                className="close-reopen-btn btn btn-secondary"
                onClick={() => toggleStatus.mutate("open")}
                disabled={toggleStatus.isPending}
              >
                Reopen issue
              </button>
            )}
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
          {/* No PATCH /comments/{id} endpoint exists — comment editing deferred */}
          {commentsQuery.data?.map((c) => (
            <div key={c.id} className="comment-card">
              <div className="comment-header">
                <strong><Link to={`/${c.author_name}`} className="author-link">{c.author_name}</Link></strong>
                <span className="comment-date">
                  {new Date(c.created_at).toLocaleDateString()}
                </span>
              </div>
              <div className="comment-body">
                <Markdown content={c.body} />
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

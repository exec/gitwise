import { useParams } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { get, post, put, patch } from "../lib/api";
import { useAuthStore } from "../stores/auth";
import RepoHeader from "../components/RepoHeader";
import DiffViewer from "../components/DiffViewer";

interface PRIntent {
  type?: string;
  scope?: string;
  components?: string[];
}

interface PullRequest {
  id: string;
  number: number;
  author_id: string;
  author_name: string;
  title: string;
  body: string;
  source_branch: string;
  target_branch: string;
  status: string;
  intent: PRIntent;
  diff_stats: {
    files_changed: number;
    insertions: number;
    deletions: number;
  };
  review_summary: {
    approved_by?: string[];
    changes_requested_by?: string[];
    reviews_count?: number;
    comments_count?: number;
    threads_resolved?: number;
    threads_unresolved?: number;
  };
  merged_by_name?: string;
  merged_at?: string;
  created_at: string;
}

interface DiffFile {
  path: string;
  old_path?: string;
  status: string;
  insertions: number;
  deletions: number;
  patch?: string;
}

interface PRDiff {
  commits: { sha: string; message: string; author: { name: string; date: string } }[];
  files: DiffFile[];
  stats: {
    total_commits: number;
    total_files: number;
    total_additions: number;
    total_deletions: number;
  };
}

interface ReviewInlineComment {
  path: string;
  line: number;
  side: string;
  body: string;
}

interface Review {
  id: string;
  author_name: string;
  type: string;
  body: string;
  comments: string | ReviewInlineComment[] | null; // JSON string or parsed array
  submitted_at: string;
}

interface Comment {
  id: string;
  author_name: string;
  body: string;
  created_at: string;
}

type PRTab = "conversation" | "files" | "commits";

export default function PullDetailPage() {
  const { owner, repo, number } = useParams();
  const user = useAuthStore((s) => s.user);
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<PRTab>("conversation");
  const [commentBody, setCommentBody] = useState("");
  const [reviewType, setReviewType] = useState("comment");
  const [reviewBody, setReviewBody] = useState("");
  const [pendingInlineComments, setPendingInlineComments] = useState<Array<{path: string, line: number, side: string, body: string}>>([]);
  const [deleteBranch, setDeleteBranch] = useState(false);

  const prQuery = useQuery({
    queryKey: ["pull", owner, repo, number],
    queryFn: () =>
      get<PullRequest>(`/repos/${owner}/${repo}/pulls/${number}`).then(
        (r) => r.data,
      ),
    enabled: !!owner && !!repo && !!number,
  });

  const diffQuery = useQuery({
    queryKey: ["pull-diff", owner, repo, number],
    queryFn: () =>
      get<PRDiff>(`/repos/${owner}/${repo}/pulls/${number}/diff`).then(
        (r) => r.data,
      ),
    enabled: !!owner && !!repo && !!number && tab === "files",
  });

  const commitsQuery = useQuery({
    queryKey: ["pull-commits", owner, repo, number],
    queryFn: () =>
      get<PRDiff>(`/repos/${owner}/${repo}/pulls/${number}/diff`).then(
        (r) => r.data,
      ),
    enabled: !!owner && !!repo && !!number && tab === "commits",
  });

  const commentsQuery = useQuery({
    queryKey: ["pull-comments", owner, repo, number],
    queryFn: () =>
      get<Comment[]>(
        `/repos/${owner}/${repo}/pulls/${number}/comments`,
      ).then((r) => r.data),
    enabled: !!owner && !!repo && !!number,
  });

  const reviewsQuery = useQuery({
    queryKey: ["pull-reviews", owner, repo, number],
    queryFn: () =>
      get<Review[]>(
        `/repos/${owner}/${repo}/pulls/${number}/reviews`,
      ).then((r) => r.data),
    enabled: !!owner && !!repo && !!number,
  });

  const addComment = useMutation({
    mutationFn: (body: string) =>
      post(`/repos/${owner}/${repo}/pulls/${number}/comments`, { body }),
    onSuccess: () => {
      setCommentBody("");
      queryClient.invalidateQueries({
        queryKey: ["pull-comments", owner, repo, number],
      });
    },
  });

  const submitReview = useMutation({
    mutationFn: (data: { type: string; body: string; comments?: Array<{path: string, line: number, side: string, body: string}> }) =>
      post(`/repos/${owner}/${repo}/pulls/${number}/reviews`, data),
    onSuccess: () => {
      setReviewBody("");
      setPendingInlineComments([]);
      queryClient.invalidateQueries({
        queryKey: ["pull-reviews", owner, repo, number],
      });
      queryClient.invalidateQueries({
        queryKey: ["pull", owner, repo, number],
      });
    },
  });

  const mergePR = useMutation({
    mutationFn: (strategy: string) =>
      put(`/repos/${owner}/${repo}/pulls/${number}/merge`, {
        strategy,
        delete_branch: deleteBranch,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["pull", owner, repo, number],
      });
    },
  });

  const closePR = useMutation({
    mutationFn: () =>
      patch(`/repos/${owner}/${repo}/pulls/${number}`, { status: "closed" }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["pull", owner, repo, number],
      });
    },
  });

  const reopenPR = useMutation({
    mutationFn: () =>
      patch(`/repos/${owner}/${repo}/pulls/${number}`, { status: "open" }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["pull", owner, repo, number],
      });
    },
  });

  function getExistingInlineComments(reviews?: Review[]): Array<{ path: string; line: number; side: string; body: string; author_name: string }> {
    if (!reviews) return [];
    const result: Array<{ path: string; line: number; side: string; body: string; author_name: string }> = [];
    for (const rev of reviews) {
      if (!rev.comments) continue;
      let parsed: ReviewInlineComment[];
      if (typeof rev.comments === "string") {
        try {
          parsed = JSON.parse(rev.comments);
        } catch {
          continue;
        }
      } else {
        parsed = rev.comments;
      }
      if (!Array.isArray(parsed)) continue;
      for (const c of parsed) {
        result.push({ path: c.path, line: c.line, side: c.side, body: c.body, author_name: rev.author_name });
      }
    }
    return result;
  }

  if (prQuery.isLoading) return <p className="muted">Loading pull request...</p>;
  if (prQuery.error)
    return (
      <div className="error-banner">
        {prQuery.error instanceof Error
          ? prQuery.error.message
          : "Failed to load pull request"}
      </div>
    );

  const pr = prQuery.data;
  if (!pr) return null;

  return (
    <div className="repo-page">
      <RepoHeader owner={owner!} repo={repo!} activeTab="pulls" />

      <div className="pr-detail">
        <div className="issue-header">
          <h2>
            {pr.title} <span className="issue-number">#{pr.number}</span>
          </h2>
          <div className="issue-status-bar">
            <span
              className={`issue-status-badge ${pr.status === "merged" ? "merged" : pr.status === "open" || pr.status === "draft" ? "open" : "closed"}`}
            >
              {pr.status}
            </span>
            <span className="issue-meta-info">
              {pr.author_name} wants to merge{" "}
              <code>{pr.source_branch}</code> into{" "}
              <code>{pr.target_branch}</code>
            </span>
          </div>
        </div>

        {pr.intent?.type && (
          <div className="pr-intent">
            <span className={`intent-type-badge ${pr.intent.type}`}>
              {pr.intent.type}
            </span>
            {pr.intent.scope && (
              <span className="intent-scope">{pr.intent.scope}</span>
            )}
            {pr.intent.components && pr.intent.components.length > 0 && (
              <span className="intent-components">
                {pr.intent.components.map((c) => (
                  <span key={c} className="intent-component-label">
                    {c}
                  </span>
                ))}
              </span>
            )}
          </div>
        )}

        {pr.review_summary.approved_by &&
          pr.review_summary.approved_by.length > 0 && (
            <div className="review-status approved">
              Approved by: {pr.review_summary.approved_by.join(", ")}
            </div>
          )}
        {pr.review_summary.changes_requested_by &&
          pr.review_summary.changes_requested_by.length > 0 && (
            <div className="review-status changes-requested">
              Changes requested by:{" "}
              {pr.review_summary.changes_requested_by.join(", ")}
            </div>
          )}
        {pr.review_summary.threads_unresolved != null && pr.review_summary.threads_unresolved > 0 && (
          <div className="review-status changes-requested">
            {pr.review_summary.threads_unresolved} unresolved thread{pr.review_summary.threads_unresolved !== 1 ? 's' : ''}
            {pr.review_summary.threads_resolved ? ` (${pr.review_summary.threads_resolved} resolved)` : ''}
          </div>
        )}
        {pr.review_summary.threads_unresolved === 0 && pr.review_summary.threads_resolved != null && pr.review_summary.threads_resolved > 0 && (
          <div className="review-status approved">
            All {pr.review_summary.threads_resolved} thread{pr.review_summary.threads_resolved !== 1 ? 's' : ''} resolved
          </div>
        )}

        <div className="tab-nav">
          <button
            className={`tab ${tab === "conversation" ? "tab-active" : ""}`}
            onClick={() => setTab("conversation")}
          >
            Conversation
          </button>
          <button
            className={`tab ${tab === "commits" ? "tab-active" : ""}`}
            onClick={() => setTab("commits")}
          >
            Commits{" "}
            {pr.diff_stats.files_changed !== undefined && (
              <span className="tab-count">
                {commitsQuery.data?.stats.total_commits ?? ""}
              </span>
            )}
          </button>
          <button
            className={`tab ${tab === "files" ? "tab-active" : ""}`}
            onClick={() => setTab("files")}
          >
            Files Changed
            <span className="tab-count">
              {pr.diff_stats.files_changed ?? 0}
            </span>
          </button>
        </div>

        {tab === "conversation" && (
          <div className="conversation-tab">
            {pr.body && (
              <div className="comment-card">
                <div className="comment-header">
                  <strong>{pr.author_name}</strong>
                  <span className="comment-date">
                    {new Date(pr.created_at).toLocaleDateString()}
                  </span>
                </div>
                <div className="comment-body">
                  <pre className="markdown-body">{pr.body}</pre>
                </div>
              </div>
            )}

            {reviewsQuery.data?.map((rev) => (
              <div key={rev.id} className={`comment-card review-card review-${rev.type}`}>
                <div className="comment-header">
                  <strong>{rev.author_name}</strong>
                  <span className={`review-type-badge ${rev.type}`}>
                    {rev.type === "approval"
                      ? "Approved"
                      : rev.type === "changes_requested"
                        ? "Changes Requested"
                        : "Commented"}
                  </span>
                  <span className="comment-date">
                    {new Date(rev.submitted_at).toLocaleDateString()}
                  </span>
                </div>
                {rev.body && (
                  <div className="comment-body">
                    <pre className="markdown-body">{rev.body}</pre>
                  </div>
                )}
              </div>
            ))}

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

            {pr.status === "merged" && (
              <div className="merge-banner">
                Merged by {pr.merged_by_name} on{" "}
                {pr.merged_at
                  ? new Date(pr.merged_at).toLocaleDateString()
                  : ""}
              </div>
            )}

            {user && pr.status === "open" && (
              <>
                <div className="review-form">
                  <h4>Submit Review</h4>
                  <textarea
                    className="comment-input"
                    placeholder="Review comment..."
                    value={reviewBody}
                    onChange={(e) => setReviewBody(e.target.value)}
                    rows={3}
                  />
                  <div className="review-actions">
                    <select
                      className="form-select"
                      value={reviewType}
                      onChange={(e) => setReviewType(e.target.value)}
                    >
                      <option value="comment">Comment</option>
                      <option value="approval">Approve</option>
                      <option value="changes_requested">
                        Request Changes
                      </option>
                    </select>
                    <button
                      className="btn btn-primary"
                      onClick={() =>
                        submitReview.mutate({
                          type: reviewType,
                          body: reviewBody,
                          comments: pendingInlineComments.length > 0 ? pendingInlineComments : undefined,
                        })
                      }
                      disabled={submitReview.isPending}
                    >
                      Submit Review
                    </button>
                  </div>
                  {submitReview.error && (
                    <div className="error-banner">
                      {submitReview.error instanceof Error
                        ? submitReview.error.message
                        : "Failed to submit review"}
                    </div>
                  )}
                </div>

                <div className="comment-form">
                  <textarea
                    className="comment-input"
                    placeholder="Leave a comment..."
                    value={commentBody}
                    onChange={(e) => setCommentBody(e.target.value)}
                    rows={3}
                  />
                  <div className="comment-actions">
                    <button
                      className="btn btn-secondary"
                      onClick={() => closePR.mutate()}
                      disabled={closePR.isPending}
                    >
                      Close PR
                    </button>
                    <div className="merge-controls">
                      <label className="checkbox-label">
                        <input
                          type="checkbox"
                          checked={deleteBranch}
                          onChange={(e) => setDeleteBranch(e.target.checked)}
                        />
                        Delete source branch
                      </label>
                      <button
                        className="btn btn-success"
                        onClick={() => mergePR.mutate("merge")}
                        disabled={mergePR.isPending}
                      >
                        Merge
                      </button>
                      <button
                        className="btn btn-success"
                        onClick={() => mergePR.mutate("squash")}
                        disabled={mergePR.isPending}
                      >
                        Squash
                      </button>
                    </div>
                    <button
                      className="btn btn-primary"
                      disabled={!commentBody.trim() || addComment.isPending}
                      onClick={() => addComment.mutate(commentBody)}
                    >
                      Comment
                    </button>
                  </div>
                  {mergePR.error && (
                    <div className="error-banner">
                      {mergePR.error instanceof Error
                        ? mergePR.error.message
                        : "Merge failed"}
                    </div>
                  )}
                </div>
              </>
            )}

            {user && pr.status === "closed" && (
              <div className="comment-actions">
                <button
                  className="btn btn-secondary"
                  onClick={() => reopenPR.mutate()}
                  disabled={reopenPR.isPending}
                >
                  Reopen PR
                </button>
              </div>
            )}
          </div>
        )}

        {tab === "files" && (
          <div className="files-tab">
            {diffQuery.isLoading && (
              <p className="muted">Loading diff...</p>
            )}
            {diffQuery.error && (
              <div className="error-banner">
                {diffQuery.error instanceof Error
                  ? diffQuery.error.message
                  : "Failed to load diff"}
              </div>
            )}
            {diffQuery.data && (
              <>
                <div className="diff-stats-bar">
                  <span>
                    {diffQuery.data.stats.total_files} files changed,{" "}
                  </span>
                  <span className="additions">
                    +{diffQuery.data.stats.total_additions}
                  </span>{" "}
                  <span className="deletions">
                    -{diffQuery.data.stats.total_deletions}
                  </span>
                </div>
                <DiffViewer
                  files={diffQuery.data.files}
                  onAddInlineComment={user && pr.status === "open" ? (path, line, side, body) => {
                    setPendingInlineComments(prev => [...prev, { path, line, side, body }]);
                  } : undefined}
                  inlineComments={[
                    ...pendingInlineComments.map(c => ({ ...c, author_name: user?.username ? `${user.username} (pending)` : "(pending)" })),
                    ...getExistingInlineComments(reviewsQuery.data),
                  ]}
                />
              </>
            )}
          </div>
        )}

        {tab === "commits" && (
          <div className="commits-tab">
            {commitsQuery.isLoading && (
              <p className="muted">Loading commits...</p>
            )}
            {commitsQuery.data?.commits.map((c) => (
              <div key={c.sha} className="commit-item">
                <div className="commit-message">
                  {c.message.split("\n")[0]}
                </div>
                <div className="commit-meta">
                  <span className="commit-author">{c.author.name}</span>
                  <span className="commit-date">
                    {new Date(c.author.date).toLocaleDateString()}
                  </span>
                  <code className="commit-sha">{c.sha.slice(0, 7)}</code>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

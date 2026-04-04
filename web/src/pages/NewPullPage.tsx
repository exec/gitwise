import { useParams, useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { get, post } from "../lib/api";
import RepoHeader from "../components/RepoHeader";

interface Repo {
  default_branch: string;
}

interface Branch {
  name: string;
}

interface PR {
  number: number;
}

export default function NewPullPage() {
  const { owner, repo } = useParams();
  const navigate = useNavigate();
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [sourceBranch, setSourceBranch] = useState("");
  const [targetBranch, setTargetBranch] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const repoQuery = useQuery({
    queryKey: ["repo", owner, repo],
    queryFn: () => get<Repo>(`/repos/${owner}/${repo}`).then((r) => r.data),
    enabled: !!owner && !!repo,
  });

  const branchesQuery = useQuery({
    queryKey: ["branches", owner, repo],
    queryFn: () =>
      get<Branch[]>(`/repos/${owner}/${repo}/branches`).then((r) => r.data),
    enabled: !!owner && !!repo,
  });

  // Set default target branch when repo loads
  if (repoQuery.data && !targetBranch) {
    setTargetBranch(repoQuery.data.default_branch);
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);

    try {
      const { data } = await post<PR>(`/repos/${owner}/${repo}/pulls`, {
        title,
        body,
        source_branch: sourceBranch,
        target_branch: targetBranch,
      });
      navigate(`/${owner}/${repo}/pulls/${data.number}`);
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Failed to create pull request",
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="repo-page">
      <RepoHeader owner={owner!} repo={repo!} activeTab="pulls" />

      <div className="form-page">
        <h2>New Pull Request</h2>

        <div className="branch-comparison">
          <div className="form-group">
            <label htmlFor="targetBranch">Base</label>
            <select
              id="targetBranch"
              className="form-select"
              value={targetBranch}
              onChange={(e) => setTargetBranch(e.target.value)}
            >
              {branchesQuery.data?.map((b) => (
                <option key={b.name} value={b.name}>
                  {b.name}
                </option>
              ))}
            </select>
          </div>
          <span className="branch-arrow">&larr;</span>
          <div className="form-group">
            <label htmlFor="sourceBranch">Compare</label>
            <select
              id="sourceBranch"
              className="form-select"
              value={sourceBranch}
              onChange={(e) => setSourceBranch(e.target.value)}
            >
              <option value="">Select branch...</option>
              {branchesQuery.data
                ?.filter((b) => b.name !== targetBranch)
                .map((b) => (
                  <option key={b.name} value={b.name}>
                    {b.name}
                  </option>
                ))}
            </select>
          </div>
        </div>

        <form onSubmit={handleSubmit}>
          {error && <div className="error-banner">{error}</div>}
          <div className="form-group">
            <label htmlFor="title">Title</label>
            <input
              id="title"
              type="text"
              className="form-input"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="Pull request title"
              required
            />
          </div>
          <div className="form-group">
            <label htmlFor="body">Description</label>
            <textarea
              id="body"
              className="form-input"
              value={body}
              onChange={(e) => setBody(e.target.value)}
              placeholder="Describe your changes..."
              rows={8}
            />
          </div>
          <button
            type="submit"
            className="btn btn-primary"
            disabled={
              !title.trim() || !sourceBranch || !targetBranch || submitting
            }
          >
            {submitting ? "Creating..." : "Create Pull Request"}
          </button>
        </form>
      </div>
    </div>
  );
}

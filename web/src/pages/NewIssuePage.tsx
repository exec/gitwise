import { useParams, useNavigate } from "react-router-dom";
import { useState } from "react";
import { post } from "../lib/api";
import RepoHeader from "../components/RepoHeader";

interface Issue {
  number: number;
}

export default function NewIssuePage() {
  const { owner, repo } = useParams();
  const navigate = useNavigate();
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [assignees, setAssignees] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);

    try {
      const parsedAssignees = assignees
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean);
      const { data } = await post<Issue>(
        `/repos/${owner}/${repo}/issues`,
        { title, body, assignees: parsedAssignees },
      );
      navigate(`/${owner}/${repo}/issues/${data.number}`);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Failed to create issue",
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="repo-page">
      <RepoHeader owner={owner!} repo={repo!} activeTab="issues" />

      <div className="form-page">
        <h2>New Issue</h2>
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
              placeholder="Issue title"
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
              placeholder="Describe the issue..."
              rows={8}
            />
          </div>
          <div className="form-group">
            <label htmlFor="assignees">Assignees</label>
            <input
              id="assignees"
              type="text"
              className="form-input"
              value={assignees}
              onChange={(e) => setAssignees(e.target.value)}
              placeholder="Comma-separated usernames"
            />
          </div>
          <button
            type="submit"
            className="btn btn-primary"
            disabled={!title.trim() || submitting}
          >
            {submitting ? "Creating..." : "Create Issue"}
          </button>
        </form>
      </div>
    </div>
  );
}

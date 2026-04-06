import { useState, useEffect, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { useMutation } from "@tanstack/react-query";
import { post, get, ApiError } from "../lib/api";
import { useAuthStore } from "../stores/auth";

type ImportSource = "github" | "gitlab";

interface ImportJob {
  id: string;
  status: string;
}

interface ImportStatus {
  id: string;
  status: "running" | "completed" | "failed";
  progress: string;
  repo_name: string;
  error?: string;
  warnings?: string[];
}

export default function ImportPage() {
  const user = useAuthStore((s) => s.user);
  const [tab, setTab] = useState<ImportSource>("github");

  // GitHub fields
  const [ghToken, setGhToken] = useState("");
  const [ghRepoURL, setGhRepoURL] = useState("");
  const [ghVisibility, setGhVisibility] = useState("private");

  // GitLab fields
  const [glToken, setGlToken] = useState("");
  const [glProjectURL, setGlProjectURL] = useState("");
  const [glInstanceURL, setGlInstanceURL] = useState("https://gitlab.com");
  const [glVisibility, setGlVisibility] = useState("private");

  // Job tracking
  const [jobID, setJobID] = useState<string | null>(null);
  const [status, setStatus] = useState<ImportStatus | null>(null);
  const [error, setError] = useState("");

  const githubMutation = useMutation({
    mutationFn: (payload: { token: string; repo_url: string; visibility: string }) =>
      post<ImportJob>("/import/github", payload).then((r) => r.data),
    onSuccess: (data) => {
      setJobID(data.id);
      setError("");
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("An unexpected error occurred");
      }
    },
  });

  const gitlabMutation = useMutation({
    mutationFn: (payload: {
      token: string;
      project_url: string;
      instance_url: string;
      visibility: string;
    }) => post<ImportJob>("/import/gitlab", payload).then((r) => r.data),
    onSuccess: (data) => {
      setJobID(data.id);
      setError("");
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("An unexpected error occurred");
      }
    },
  });

  // Poll for status while job is running
  useEffect(() => {
    if (!jobID) return;

    const interval = setInterval(async () => {
      try {
        const { data } = await get<ImportStatus>(`/import/status/${jobID}`);
        setStatus(data);
        if (data.status === "completed" || data.status === "failed") {
          clearInterval(interval);
        }
      } catch {
        // ignore polling errors
      }
    }, 2000);

    // Fetch initial status immediately
    get<ImportStatus>(`/import/status/${jobID}`)
      .then(({ data }) => setStatus(data))
      .catch(() => {});

    return () => clearInterval(interval);
  }, [jobID]);

  const handleGitHubSubmit = (e: FormEvent) => {
    e.preventDefault();
    setError("");
    setStatus(null);
    githubMutation.mutate({
      token: ghToken,
      repo_url: ghRepoURL,
      visibility: ghVisibility,
    });
  };

  const handleGitLabSubmit = (e: FormEvent) => {
    e.preventDefault();
    setError("");
    setStatus(null);
    gitlabMutation.mutate({
      token: glToken,
      project_url: glProjectURL,
      instance_url: glInstanceURL,
      visibility: glVisibility,
    });
  };

  const isPending =
    githubMutation.isPending || gitlabMutation.isPending;
  const isRunning = status?.status === "running";

  const resetForm = () => {
    setJobID(null);
    setStatus(null);
    setError("");
  };

  return (
    <div className="import-page">
      <h1>Import repository</h1>
      <p className="import-subtitle">
        Import a repository from GitHub or GitLab, including issues and pull
        requests.
      </p>

      {error && <div className="error-banner">{error}</div>}

      {/* Status display */}
      {status && (
        <div className={`import-status import-status--${status.status}`}>
          <div className="import-status-header">
            {status.status === "running" && (
              <span className="import-spinner" />
            )}
            {status.status === "completed" && (
              <span className="import-icon-success">&#10003;</span>
            )}
            {status.status === "failed" && (
              <span className="import-icon-fail">&#10007;</span>
            )}
            <strong>
              {status.status === "running"
                ? "Importing..."
                : status.status === "completed"
                  ? "Import complete"
                  : "Import failed"}
            </strong>
          </div>
          <p className="import-status-progress">{status.progress}</p>
          {status.error && (
            <p className="import-status-error">{status.error}</p>
          )}
          {status.warnings && status.warnings.length > 0 && (
            <details className="import-warnings">
              <summary>
                {status.warnings.length} warning
                {status.warnings.length !== 1 ? "s" : ""}
              </summary>
              <ul>
                {status.warnings.map((w, i) => (
                  <li key={i}>{w}</li>
                ))}
              </ul>
            </details>
          )}
          {status.status === "completed" && user && (
            <Link
              to={`/${user.username}/${status.repo_name}`}
              className="btn btn-primary import-view-repo"
            >
              View imported repository
            </Link>
          )}
          {(status.status === "completed" || status.status === "failed") && (
            <button
              type="button"
              className="btn btn-secondary import-another"
              onClick={resetForm}
            >
              Import another repository
            </button>
          )}
        </div>
      )}

      {/* Form (hidden while job is active) */}
      {!jobID && (
        <>
          <div className="import-tabs">
            <button
              type="button"
              className={`import-tab ${tab === "github" ? "import-tab--active" : ""}`}
              onClick={() => setTab("github")}
            >
              GitHub
            </button>
            <button
              type="button"
              className={`import-tab ${tab === "gitlab" ? "import-tab--active" : ""}`}
              onClick={() => setTab("gitlab")}
            >
              GitLab
            </button>
          </div>

          {tab === "github" && (
            <form onSubmit={handleGitHubSubmit} className="import-form">
              <div className="form-group">
                <label htmlFor="gh-token">Personal access token</label>
                <input
                  id="gh-token"
                  type="password"
                  value={ghToken}
                  onChange={(e) => setGhToken(e.target.value)}
                  placeholder="ghp_..."
                  required
                  autoComplete="off"
                />
                <p className="form-hint">
                  Requires <code>repo</code> scope for private repositories.{" "}
                  <a
                    href="https://github.com/settings/tokens/new?scopes=repo"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    Create a token
                  </a>
                </p>
              </div>
              <div className="form-group">
                <label htmlFor="gh-url">Repository URL</label>
                <input
                  id="gh-url"
                  type="text"
                  value={ghRepoURL}
                  onChange={(e) => setGhRepoURL(e.target.value)}
                  placeholder="https://github.com/owner/repo"
                  required
                />
              </div>
              <div className="form-group">
                <label>Visibility</label>
                <div className="radio-group">
                  <label className="radio-label">
                    <input
                      type="radio"
                      name="gh-visibility"
                      value="public"
                      checked={ghVisibility === "public"}
                      onChange={(e) => setGhVisibility(e.target.value)}
                    />
                    Public
                  </label>
                  <label className="radio-label">
                    <input
                      type="radio"
                      name="gh-visibility"
                      value="private"
                      checked={ghVisibility === "private"}
                      onChange={(e) => setGhVisibility(e.target.value)}
                    />
                    Private
                  </label>
                </div>
              </div>
              <button
                type="submit"
                className="btn btn-primary"
                disabled={isPending}
              >
                {isPending ? "Starting import..." : "Import from GitHub"}
              </button>
            </form>
          )}

          {tab === "gitlab" && (
            <form onSubmit={handleGitLabSubmit} className="import-form">
              <div className="form-group">
                <label htmlFor="gl-token">Personal access token</label>
                <input
                  id="gl-token"
                  type="password"
                  value={glToken}
                  onChange={(e) => setGlToken(e.target.value)}
                  placeholder="glpat-..."
                  required
                  autoComplete="off"
                />
                <p className="form-hint">
                  Requires <code>read_api</code> and <code>read_repository</code>{" "}
                  scopes.
                </p>
              </div>
              <div className="form-group">
                <label htmlFor="gl-instance">GitLab instance URL</label>
                <input
                  id="gl-instance"
                  type="url"
                  value={glInstanceURL}
                  onChange={(e) => setGlInstanceURL(e.target.value)}
                  placeholder="https://gitlab.com"
                />
                <p className="form-hint">
                  Change this for self-hosted GitLab instances.
                </p>
              </div>
              <div className="form-group">
                <label htmlFor="gl-url">Project URL</label>
                <input
                  id="gl-url"
                  type="text"
                  value={glProjectURL}
                  onChange={(e) => setGlProjectURL(e.target.value)}
                  placeholder="https://gitlab.com/namespace/project"
                  required
                />
              </div>
              <div className="form-group">
                <label>Visibility</label>
                <div className="radio-group">
                  <label className="radio-label">
                    <input
                      type="radio"
                      name="gl-visibility"
                      value="public"
                      checked={glVisibility === "public"}
                      onChange={(e) => setGlVisibility(e.target.value)}
                    />
                    Public
                  </label>
                  <label className="radio-label">
                    <input
                      type="radio"
                      name="gl-visibility"
                      value="private"
                      checked={glVisibility === "private"}
                      onChange={(e) => setGlVisibility(e.target.value)}
                    />
                    Private
                  </label>
                </div>
              </div>
              <button
                type="submit"
                className="btn btn-primary"
                disabled={isPending}
              >
                {isPending ? "Starting import..." : "Import from GitLab"}
              </button>
            </form>
          )}
        </>
      )}
    </div>
  );
}

import { useParams, useLocation, Link, useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { get } from "../lib/api";

interface Repo {
  id: string;
  owner_name: string;
  name: string;
  description: string;
  visibility: string;
  default_branch: string;
  created_at: string;
  updated_at: string;
}

interface TreeEntry {
  name: string;
  type: "blob" | "tree";
  size: number;
  path: string;
}

interface Blob {
  content: string;
  size: number;
  encoding: string;
}

interface Commit {
  sha: string;
  message: string;
  author: {
    name: string;
    email: string;
    date: string;
  };
}

interface Branch {
  name: string;
  sha: string;
}

type Tab = "code" | "commits";

function detectTab(pathname: string): Tab {
  if (pathname.includes("/commits")) return "commits";
  return "code";
}

function detectView(pathname: string): "tree" | "blob" | "root" {
  if (pathname.includes("/blob/")) return "blob";
  if (pathname.includes("/tree/")) return "tree";
  return "root";
}

export default function RepoPage() {
  const { owner, repo, ref: refParam, "*": splat } = useParams();
  const location = useLocation();
  const navigate = useNavigate();

  const tab = detectTab(location.pathname);
  const view = detectView(location.pathname);

  const repoQuery = useQuery({
    queryKey: ["repo", owner, repo],
    queryFn: () => get<Repo>(`/repos/${owner}/${repo}`).then((r) => r.data),
    enabled: !!owner && !!repo,
  });

  const repoLoaded = !!repoQuery.data;
  const currentRef = refParam ?? repoQuery.data?.default_branch ?? "main";

  const branchesQuery = useQuery({
    queryKey: ["branches", owner, repo],
    queryFn: () =>
      get<Branch[]>(`/repos/${owner}/${repo}/branches`).then((r) => r.data),
    enabled: !!owner && !!repo && repoLoaded,
  });

  const treePath = splat ?? "";

  const treeQuery = useQuery({
    queryKey: ["tree", owner, repo, currentRef, treePath],
    queryFn: () => {
      const path = treePath ? `/${treePath}` : "";
      return get<TreeEntry[]>(
        `/repos/${owner}/${repo}/tree/${currentRef}${path}`,
      ).then((r) => r.data);
    },
    enabled: !!owner && !!repo && repoLoaded && tab === "code" && view !== "blob",
  });

  const blobQuery = useQuery({
    queryKey: ["blob", owner, repo, currentRef, treePath],
    queryFn: () =>
      get<Blob>(
        `/repos/${owner}/${repo}/blob/${currentRef}/${treePath}`,
      ).then((r) => r.data),
    enabled: !!owner && !!repo && repoLoaded && view === "blob" && !!treePath,
  });

  const commitsQuery = useQuery({
    queryKey: ["commits", owner, repo, currentRef],
    queryFn: () =>
      get<Commit[]>(`/repos/${owner}/${repo}/commits?ref=${encodeURIComponent(currentRef)}`).then(
        (r) => r.data,
      ),
    enabled: !!owner && !!repo && repoLoaded && tab === "commits",
  });

  if (repoQuery.isLoading) {
    return <p className="muted">Loading repository...</p>;
  }

  if (repoQuery.error) {
    return <div className="error-banner">{repoQuery.error instanceof Error ? repoQuery.error.message : "Failed to load repository"}</div>;
  }

  const repoData = repoQuery.data;
  if (!repoData) return null;

  const sortedEntries = treeQuery.data
    ? [...treeQuery.data].sort((a, b) => {
        if (a.type === b.type) return a.name.localeCompare(b.name);
        return a.type === "tree" ? -1 : 1;
      })
    : [];

  const parentPath = treePath
    ? treePath.split("/").slice(0, -1).join("/")
    : null;

  return (
    <div className="repo-page">
      <div className="repo-header">
        <div className="repo-title">
          <h1>
            <Link to={`/${owner}`} className="owner-link">
              {repoData.owner_name}
            </Link>
            {" / "}
            <Link to={`/${owner}/${repo}`}>{repoData.name}</Link>
          </h1>
          <span className={`badge badge-${repoData.visibility}`}>
            {repoData.visibility}
          </span>
        </div>
        {repoData.description && (
          <p className="repo-description">{repoData.description}</p>
        )}
      </div>

      <div className="tab-nav">
        <Link
          to={`/${owner}/${repo}`}
          className={`tab ${tab === "code" ? "tab-active" : ""}`}
        >
          Code
        </Link>
        <Link
          to={`/${owner}/${repo}/commits`}
          className={`tab ${tab === "commits" ? "tab-active" : ""}`}
        >
          Commits
        </Link>
      </div>

      {tab === "code" && (
        <div className="code-tab">
          <div className="code-toolbar">
            <select
              className="branch-selector"
              value={currentRef}
              onChange={(e) => {
                const newRef = e.target.value;
                navigate(`/${owner}/${repo}/tree/${newRef}`);
              }}
            >
              {branchesQuery.data?.map((b) => (
                <option key={b.name} value={b.name}>
                  {b.name}
                </option>
              ))}
              {!branchesQuery.data && (
                <option value={currentRef}>{currentRef}</option>
              )}
            </select>
          </div>

          {view === "blob" ? (
            blobQuery.isLoading ? (
              <p className="muted">Loading file...</p>
            ) : blobQuery.error ? (
              <div className="error-banner">
                {blobQuery.error instanceof Error ? blobQuery.error.message : "Failed to load file"}
              </div>
            ) : blobQuery.data ? (
              <div className="file-view">
                <div className="file-header">
                  <span className="file-path">{treePath}</span>
                  <span className="file-size">
                    {formatSize(blobQuery.data.size)}
                  </span>
                </div>
                <pre className="file-content">
                  <code>{blobQuery.data.content}</code>
                </pre>
              </div>
            ) : null
          ) : (
            <>
              {treeQuery.isLoading && (
                <p className="muted">Loading files...</p>
              )}
              {treeQuery.error && (
                <div className="error-banner">{treeQuery.error instanceof Error ? treeQuery.error.message : "Failed to load files"}</div>
              )}
              {treeQuery.data && (
                <table className="file-table">
                  <tbody>
                    {parentPath !== null && (
                      <tr>
                        <td className="file-icon">..</td>
                        <td>
                          <Link
                            to={
                              parentPath
                                ? `/${owner}/${repo}/tree/${currentRef}/${parentPath}`
                                : `/${owner}/${repo}`
                            }
                          >
                            ..
                          </Link>
                        </td>
                        <td></td>
                      </tr>
                    )}
                    {sortedEntries.map((entry) => {
                      const entryPath = treePath
                        ? `${treePath}/${entry.name}`
                        : entry.name;
                      const linkType =
                        entry.type === "tree" ? "tree" : "blob";
                      return (
                        <tr key={entry.name}>
                          <td className="file-icon">
                            {entry.type === "tree" ? "\uD83D\uDCC1" : "\uD83D\uDCC4"}
                          </td>
                          <td>
                            <Link
                              to={`/${owner}/${repo}/${linkType}/${currentRef}/${entryPath}`}
                            >
                              {entry.name}
                            </Link>
                          </td>
                          <td className="file-size-cell">
                            {entry.type === "blob"
                              ? formatSize(entry.size)
                              : ""}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              )}
            </>
          )}
        </div>
      )}

      {tab === "commits" && (
        <div className="commits-tab">
          {commitsQuery.isLoading && (
            <p className="muted">Loading commits...</p>
          )}
          {commitsQuery.error && (
            <div className="error-banner">{commitsQuery.error instanceof Error ? commitsQuery.error.message : "Failed to load commits"}</div>
          )}
          {commitsQuery.data && (
            <ul className="commit-list">
              {commitsQuery.data.map((c) => (
                <li key={c.sha} className="commit-item">
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
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

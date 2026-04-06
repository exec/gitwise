import { useState } from "react";
import { useParams, useLocation, Link, useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { get } from "../lib/api";
import RepoHeader from "../components/RepoHeader";
import CodeView from "../components/CodeView";
import BlameView from "../components/BlameView";
import type { BlameLineData } from "../components/BlameView";
import Markdown from "../components/Markdown";

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

function detectView(pathname: string): "tree" | "blob" | "blame" | "root" {
  if (pathname.includes("/blame/")) return "blame";
  if (pathname.includes("/blob/")) return "blob";
  if (pathname.includes("/tree/")) return "tree";
  return "root";
}

const README_NAMES = ["readme.md", "readme.markdown", "readme.rst", "readme.txt", "readme"];

function findReadme(entries: TreeEntry[]): TreeEntry | undefined {
  for (const name of README_NAMES) {
    const match = entries.find(
      (e) => e.type === "blob" && e.name.toLowerCase() === name,
    );
    if (match) return match;
  }
  return undefined;
}

function isMarkdownFile(name: string): boolean {
  const lower = name.toLowerCase();
  return lower.endsWith(".md") || lower.endsWith(".markdown");
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
    enabled: !!owner && !!repo && repoLoaded && tab === "code" && view !== "blob" && view !== "blame",
  });

  const blobQuery = useQuery({
    queryKey: ["blob", owner, repo, currentRef, treePath],
    queryFn: () =>
      get<Blob>(
        `/repos/${owner}/${repo}/blob/${currentRef}/${treePath}`,
      ).then((r) => r.data),
    enabled: !!owner && !!repo && repoLoaded && (view === "blob" || view === "blame") && !!treePath,
  });

  const blameQuery = useQuery({
    queryKey: ["blame", owner, repo, currentRef, treePath],
    queryFn: () =>
      get<BlameLineData[]>(
        `/repos/${owner}/${repo}/blame/${currentRef}/${treePath}`,
      ).then((r) => r.data),
    enabled: !!owner && !!repo && repoLoaded && view === "blame" && !!treePath,
  });

  const commitsQuery = useQuery({
    queryKey: ["commits", owner, repo, currentRef],
    queryFn: () =>
      get<Commit[]>(`/repos/${owner}/${repo}/commits?ref=${encodeURIComponent(currentRef)}`).then(
        (r) => r.data,
      ),
    enabled: !!owner && !!repo && repoLoaded && tab === "commits",
  });

  const readmeEntry = treeQuery.data ? findReadme(treeQuery.data) : undefined;
  const readmePath = readmeEntry
    ? treePath
      ? `${treePath}/${readmeEntry.name}`
      : readmeEntry.name
    : "";

  const readmeQuery = useQuery({
    queryKey: ["readme", owner, repo, currentRef, readmePath],
    queryFn: () =>
      get<Blob>(
        `/repos/${owner}/${repo}/blob/${currentRef}/${readmePath}`,
      ).then((r) => r.data),
    enabled: !!readmeEntry && !!owner && !!repo && view !== "blob",
  });

  if (repoQuery.isLoading) {
    return <p className="muted">Loading repository...</p>;
  }

  if (repoQuery.error) {
    return <div className="error-banner">{repoQuery.error instanceof Error ? repoQuery.error.message : "Failed to load repository"}</div>;
  }

  const repoData = repoQuery.data;
  if (!repoData) return null;

  const isEmptyRepo = branchesQuery.data !== undefined && branchesQuery.data.length === 0;

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
      <RepoHeader
        owner={owner!}
        repo={repo!}
        activeTab={tab === "commits" ? "commits" : "code"}
      />

      {tab === "code" && isEmptyRepo && (
        <SetupInstructions owner={owner!} repo={repo!} />
      )}

      {tab === "code" && !isEmptyRepo && (
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
            <input
              className="clone-url-input"
              type="text"
              readOnly
              value={`${window.location.origin}/${owner}/${repo}.git`}
              onFocus={(e) => e.target.select()}
            />
          </div>

          {view === "blob" || view === "blame" ? (
            blobQuery.isLoading || (view === "blame" && blameQuery.isLoading) ? (
              <p className="muted">Loading file...</p>
            ) : blobQuery.error ? (
              <div className="error-banner">
                {blobQuery.error instanceof Error ? blobQuery.error.message : "Failed to load file"}
              </div>
            ) : blameQuery.error && view === "blame" ? (
              <div className="error-banner">
                {blameQuery.error instanceof Error ? blameQuery.error.message : "Failed to load blame data"}
              </div>
            ) : blobQuery.data ? (
              <div className="file-view">
                <div className="file-header">
                  <span className="file-path">{treePath}</span>
                  <div className="file-header-actions">
                    <div className="file-view-toggle">
                      <Link
                        to={`/${owner}/${repo}/blob/${currentRef}/${treePath}`}
                        className={`file-view-btn${view === "blob" ? " active" : ""}`}
                      >
                        Code
                      </Link>
                      <Link
                        to={`/${owner}/${repo}/blame/${currentRef}/${treePath}`}
                        className={`file-view-btn${view === "blame" ? " active" : ""}`}
                      >
                        Blame
                      </Link>
                    </div>
                    <span className="file-size">
                      {formatSize(blobQuery.data.size)}
                    </span>
                  </div>
                </div>
                <div className="file-content">
                  {view === "blame" && blameQuery.data ? (
                    <BlameView lines={blameQuery.data} filename={treePath} />
                  ) : (
                    <CodeView code={blobQuery.data.content} filename={treePath} />
                  )}
                </div>
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
              {readmeQuery.data && readmeEntry && (
                <div className="readme-container">
                  <div className="readme-header">
                    <svg className="readme-icon" viewBox="0 0 16 16" width="16" height="16" fill="currentColor">
                      <path d="M0 1.75A.75.75 0 0 1 .75 1h4.253c1.227 0 2.317.59 3 1.501A3.744 3.744 0 0 1 11.006 1h4.245a.75.75 0 0 1 .75.75v10.5a.75.75 0 0 1-.75.75h-4.507a2.25 2.25 0 0 0-1.591.659l-.622.621a.75.75 0 0 1-1.06 0l-.622-.621A2.25 2.25 0 0 0 5.258 13H.75a.75.75 0 0 1-.75-.75Zm7.251 10.324.004-5.073-.002-2.253A2.25 2.25 0 0 0 5.003 2.5H1.5v9h3.757a3.75 3.75 0 0 1 1.994.574ZM8.755 4.75l-.004 7.322a3.752 3.752 0 0 1 1.992-.572H14.5v-9h-3.495a2.25 2.25 0 0 0-2.25 2.25Z"></path>
                    </svg>
                    <span>{readmeEntry.name}</span>
                  </div>
                  <div className="readme-content">
                    {isMarkdownFile(readmeEntry.name) ? (
                      <Markdown content={readmeQuery.data.content} />
                    ) : (
                      <pre className="readme-plain">{readmeQuery.data.content}</pre>
                    )}
                  </div>
                </div>
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

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  };

  return (
    <button className="btn btn-sm btn-secondary" onClick={handleCopy}>
      {copied ? "Copied!" : "Copy"}
    </button>
  );
}

function SetupInstructions({ owner, repo }: { owner: string; repo: string }) {
  const cloneUrl = `${window.location.origin}/${owner}/${repo}.git`;

  const newRepoCommands = [
    `echo "# ${repo}" >> README.md`,
    "git init",
    "git add README.md",
    'git commit -m "first commit"',
    "git branch -M main",
    `git remote add origin ${cloneUrl}`,
    "git push -u origin main",
  ].join("\n");

  const existingRepoCommands = [
    `git remote add origin ${cloneUrl}`,
    "git branch -M main",
    "git push -u origin main",
  ].join("\n");

  return (
    <div className="setup-instructions">
      <div className="setup-quick">
        <h2>Quick setup</h2>
        <div className="setup-clone-url">
          <input
            type="text"
            readOnly
            value={cloneUrl}
            className="setup-url-input"
          />
          <CopyButton text={cloneUrl} />
        </div>
      </div>

      <div className="setup-section">
        <h3>...or create a new repository on the command line</h3>
        <div className="setup-code-block">
          <pre><code>{newRepoCommands}</code></pre>
          <div className="setup-code-copy">
            <CopyButton text={newRepoCommands} />
          </div>
        </div>
      </div>

      <div className="setup-section">
        <h3>...or push an existing repository from the command line</h3>
        <div className="setup-code-block">
          <pre><code>{existingRepoCommands}</code></pre>
          <div className="setup-code-copy">
            <CopyButton text={existingRepoCommands} />
          </div>
        </div>
      </div>
    </div>
  );
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

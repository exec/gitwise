import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { get } from "../lib/api";

interface Repo {
  owner_name: string;
  name: string;
  description: string;
  visibility: string;
}

interface RepoHeaderProps {
  owner: string;
  repo: string;
  activeTab: "code" | "issues" | "pulls" | "commits";
}

export default function RepoHeader({ owner, repo, activeTab }: RepoHeaderProps) {
  const repoQuery = useQuery({
    queryKey: ["repo", owner, repo],
    queryFn: () => get<Repo>(`/repos/${owner}/${repo}`).then((r) => r.data),
    enabled: !!owner && !!repo,
  });

  const repoData = repoQuery.data;

  return (
    <>
      <div className="repo-header">
        <div className="repo-title">
          <h1>
            <Link to={`/${owner}`} className="owner-link">
              {repoData?.owner_name ?? owner}
            </Link>
            {" / "}
            <Link to={`/${owner}/${repo}`}>{repoData?.name ?? repo}</Link>
          </h1>
          {repoData && (
            <span className={`badge badge-${repoData.visibility}`}>
              {repoData.visibility}
            </span>
          )}
        </div>
        {repoData?.description && (
          <p className="repo-description">{repoData.description}</p>
        )}
      </div>

      <div className="tab-nav">
        <Link
          to={`/${owner}/${repo}`}
          className={`tab ${activeTab === "code" ? "tab-active" : ""}`}
        >
          Code
        </Link>
        <Link
          to={`/${owner}/${repo}/issues`}
          className={`tab ${activeTab === "issues" ? "tab-active" : ""}`}
        >
          Issues
        </Link>
        <Link
          to={`/${owner}/${repo}/pulls`}
          className={`tab ${activeTab === "pulls" ? "tab-active" : ""}`}
        >
          Pull Requests
        </Link>
        <Link
          to={`/${owner}/${repo}/commits`}
          className={`tab ${activeTab === "commits" ? "tab-active" : ""}`}
        >
          Commits
        </Link>
      </div>
    </>
  );
}

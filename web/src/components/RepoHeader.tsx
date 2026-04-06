import { Link } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { get, put, del } from "../lib/api";
import { useAuthStore } from "../stores/auth";

interface Repo {
  owner_name: string;
  name: string;
  description: string;
  visibility: string;
}

interface WatchStatus {
  watching: boolean;
  count: number;
}

interface RepoHeaderProps {
  owner: string;
  repo: string;
  activeTab: "code" | "issues" | "pulls" | "commits" | "settings";
}

export default function RepoHeader({ owner, repo, activeTab }: RepoHeaderProps) {
  const currentUser = useAuthStore((s) => s.user);
  const isOwner = currentUser?.username === owner;
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const queryClient = useQueryClient();

  const repoQuery = useQuery({
    queryKey: ["repo", owner, repo],
    queryFn: () => get<Repo>(`/repos/${owner}/${repo}`).then((r) => r.data),
    enabled: !!owner && !!repo,
  });

  const watchQuery = useQuery({
    queryKey: ["watch-status", owner, repo],
    queryFn: () => get<WatchStatus>(`/repos/${owner}/${repo}/watchers`).then((r) => r.data),
    enabled: !!owner && !!repo,
  });

  const watchMutation = useMutation({
    mutationFn: async () => {
      if (watchQuery.data?.watching) {
        await del(`/repos/${owner}/${repo}/watch`);
      } else {
        await put(`/repos/${owner}/${repo}/watch`);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["watch-status", owner, repo] });
    },
  });

  const repoData = repoQuery.data;
  const watchData = watchQuery.data;

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
          {isAuthenticated && (
            <button
              className={`watch-btn ${watchData?.watching ? "watching" : ""}`}
              onClick={() => watchMutation.mutate()}
              disabled={watchMutation.isPending}
            >
              <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
                {watchData?.watching ? (
                  <path fillRule="evenodd" d="M1.679 7.932c.412-.621 1.242-1.75 2.366-2.717C5.175 4.242 6.527 3.5 8 3.5c1.473 0 2.824.742 3.955 1.715 1.124.967 1.954 2.096 2.366 2.717a.119.119 0 010 .136c-.412.621-1.242 1.75-2.366 2.717C10.825 11.758 9.473 12.5 8 12.5c-1.473 0-2.825-.742-3.955-1.715C2.92 9.818 2.09 8.69 1.679 8.068a.119.119 0 010-.136zM8 2c-1.981 0-3.67.992-4.933 2.078C1.797 5.169.88 6.423.43 7.1a1.619 1.619 0 000 1.798c.45.678 1.367 1.932 2.637 3.024C4.329 13.008 6.019 14 8 14c1.981 0 3.67-.992 4.933-2.078 1.27-1.091 2.187-2.345 2.637-3.023a1.619 1.619 0 000-1.798c-.45-.678-1.367-1.932-2.637-3.023C11.671 2.992 9.981 2 8 2zm0 8a2 2 0 100-4 2 2 0 000 4z" />
                ) : (
                  <path fillRule="evenodd" d="M1.679 7.932c.412-.621 1.242-1.75 2.366-2.717C5.175 4.242 6.527 3.5 8 3.5c1.473 0 2.824.742 3.955 1.715 1.124.967 1.954 2.096 2.366 2.717a.119.119 0 010 .136c-.412.621-1.242 1.75-2.366 2.717C10.825 11.758 9.473 12.5 8 12.5c-1.473 0-2.825-.742-3.955-1.715C2.92 9.818 2.09 8.69 1.679 8.068a.119.119 0 010-.136zM8 2c-1.981 0-3.67.992-4.933 2.078C1.797 5.169.88 6.423.43 7.1a1.619 1.619 0 000 1.798c.45.678 1.367 1.932 2.637 3.024C4.329 13.008 6.019 14 8 14c1.981 0 3.67-.992 4.933-2.078 1.27-1.091 2.187-2.345 2.637-3.023a1.619 1.619 0 000-1.798c-.45-.678-1.367-1.932-2.637-3.023C11.671 2.992 9.981 2 8 2zm0 8a2 2 0 100-4 2 2 0 000 4z" />
                )}
              </svg>
              {watchData?.watching ? "Unwatch" : "Watch"}
              {watchData !== undefined && (
                <span className="watch-count">{watchData.count}</span>
              )}
            </button>
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
        {isOwner && (
          <Link
            to={`/${owner}/${repo}/settings`}
            className={`tab ${activeTab === "settings" ? "tab-active" : ""}`}
          >
            Settings
          </Link>
        )}
      </div>
    </>
  );
}

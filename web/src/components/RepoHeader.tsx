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
  activeTab: "code" | "issues" | "pulls" | "commits" | "agents" | "settings";
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
    onMutate: async () => {
      await queryClient.cancelQueries({ queryKey: ["watch-status", owner, repo] });
      const previous = queryClient.getQueryData<WatchStatus>(["watch-status", owner, repo]);
      if (previous) {
        queryClient.setQueryData<WatchStatus>(["watch-status", owner, repo], {
          watching: !previous.watching,
          count: previous.watching ? previous.count - 1 : previous.count + 1,
        });
      }
      return { previous };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.previous) {
        queryClient.setQueryData(["watch-status", owner, repo], ctx.previous);
      }
    },
    onSettled: () => {
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
                  <path fillRule="evenodd" d="M.143 2.31a.75.75 0 011.047-.167l14 10a.75.75 0 11-.88 1.214l-2.248-1.606C11.058 12.517 9.592 13 8 13c-1.981 0-3.67-.992-4.933-2.078C1.797 9.83.88 8.577.43 7.9a1.619 1.619 0 010-1.798c.322-.486.903-1.28 1.706-2.09L.31 3.357A.75.75 0 01.143 2.31zm3.386 3.378a3.5 3.5 0 004.753 4.753L7.28 9.768A2 2 0 016.232 8.72L3.53 5.689zM8 3c.692 0 1.357.104 1.983.291l-1.14.814A3.5 3.5 0 005.258 7.72l-1.166.833c-.459-.58-.862-1.14-1.152-1.595C3.352 6.335 4.184 5.204 5.31 4.236 6.437 3.268 7.614 2.75 8.93 2.53 8.62 2.51 8.31 2.5 8 2.5V3zm4.934 3.96a10.927 10.927 0 00-1.07-1.275l1.094-.782c.57.565 1.038 1.14 1.379 1.596l.016.024c-.45.678-1.367 1.932-2.637 3.023-.29.25-.595.483-.91.695l-1.073-.767A2 2 0 008.005 6a2 2 0 00.535.074l.006-.004.003-.002z" />
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
        <Link
          to={`/${owner}/${repo}/agents`}
          className={`tab ${activeTab === "agents" ? "tab-active" : ""}`}
        >
          Agents
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

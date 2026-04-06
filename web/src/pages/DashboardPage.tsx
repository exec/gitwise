import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { get } from "../lib/api";
import { avatarUrl } from "../lib/avatar";

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

interface OrgMembership {
  id: string;
  name: string;
  display_name: string;
  avatar_url: string;
  role: string;
}

export default function DashboardPage() {
  const { data: repos, isLoading, error } = useQuery({
    queryKey: ["user-repos"],
    queryFn: async () => {
      const { data } = await get<Repo[]>("/user/repos");
      return data;
    },
  });

  const orgsQuery = useQuery({
    queryKey: ["my-orgs"],
    queryFn: async () => {
      const { data } = await get<OrgMembership[]>("/user/orgs");
      return data;
    },
  });

  return (
    <div className="dashboard-layout">
      <aside className="dashboard-sidebar">
        {/* Your organizations */}
        {orgsQuery.data && orgsQuery.data.length > 0 && (
          <div className="dashboard-orgs">
            <h3 className="dashboard-orgs-title">Your organizations</h3>
            <ul className="dashboard-orgs-list">
              {orgsQuery.data.map((org) => (
                <li key={org.id}>
                  <Link to={`/${org.name}`} className="dashboard-org-item">
                    {org.avatar_url ? (
                      <img
                        src={avatarUrl(org.avatar_url, 40)}
                        alt={org.name}
                        className="dashboard-org-avatar"
                      />
                    ) : (
                      <div className="dashboard-org-avatar dashboard-org-avatar-placeholder">
                        {(org.display_name || org.name).charAt(0).toUpperCase()}
                      </div>
                    )}
                    <span className="dashboard-org-name">{org.display_name || org.name}</span>
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        )}
      </aside>

      <div className="dashboard">
        <div className="dashboard-header">
          <h1>Your repositories</h1>
          <Link to="/new" className="btn btn-primary">
            New repository
          </Link>
        </div>

        {isLoading && <p className="muted">Loading repositories...</p>}
        {error && <div className="error-banner">{error instanceof Error ? error.message : "Failed to load repositories"}</div>}

        {repos && repos.length === 0 && (
          <div className="empty-state">
            <p>You don't have any repositories yet.</p>
            <Link to="/new" className="btn btn-primary">
              Create your first repository
            </Link>
          </div>
        )}

        {repos && repos.length > 0 && (
          <ul className="repo-list">
            {repos.map((repo) => (
              <li key={repo.id} className="repo-list-item">
                <div className="repo-info">
                  <Link to={`/${repo.owner_name}/${repo.name}`} className="repo-name">
                    {repo.owner_name}/{repo.name}
                  </Link>
                  <span className={`badge badge-${repo.visibility}`}>{repo.visibility}</span>
                </div>
                {repo.description && (
                  <p className="repo-description">{repo.description}</p>
                )}
                <span className="repo-meta">
                  Updated {new Date(repo.updated_at).toLocaleDateString()}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

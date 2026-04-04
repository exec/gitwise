import { useParams, Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { get } from "../lib/api";

interface Organization {
  id: string;
  name: string;
  display_name: string;
  description: string;
  avatar_url: string;
  created_at: string;
}

interface OrgMember {
  user_id: string;
  username: string;
  full_name: string;
  avatar_url: string;
  role: string;
}

interface Repo {
  id: string;
  name: string;
  owner_name: string;
  description: string;
  visibility: string;
  language_stats: Record<string, number> | null;
  stars_count: number;
  updated_at: string;
}

function topLanguage(stats: Record<string, number> | null): string | null {
  if (!stats) return null;
  const entries = Object.entries(stats);
  if (entries.length === 0) return null;
  entries.sort((a, b) => b[1] - a[1]);
  return entries[0][0];
}

export default function OrgPage() {
  const { name } = useParams();

  const orgQuery = useQuery({
    queryKey: ["org", name],
    queryFn: () => get<Organization>(`/orgs/${name}`).then((r) => r.data),
    enabled: !!name,
  });

  const membersQuery = useQuery({
    queryKey: ["org-members", name],
    queryFn: () =>
      get<OrgMember[]>(`/orgs/${name}/members`).then((r) => r.data),
    enabled: !!name,
  });

  const reposQuery = useQuery({
    queryKey: ["org-repos", name],
    queryFn: () => get<Repo[]>(`/orgs/${name}/repos`).then((r) => r.data),
    enabled: !!name,
  });

  if (orgQuery.isLoading) {
    return <p className="muted">Loading organization...</p>;
  }

  if (orgQuery.error) {
    return (
      <div className="error-banner">
        {orgQuery.error instanceof Error
          ? orgQuery.error.message
          : "Failed to load organization"}
      </div>
    );
  }

  const org = orgQuery.data;
  if (!org) return null;

  return (
    <div className="org-page">
      <div className="org-header">
        {org.avatar_url ? (
          <img src={org.avatar_url} alt={org.name} className="org-avatar" />
        ) : (
          <div className="org-avatar org-avatar-placeholder">
            {(org.display_name || org.name).charAt(0).toUpperCase()}
          </div>
        )}
        <div className="org-header-info">
          <h1 className="org-display-name">
            {org.display_name || org.name}
          </h1>
          {org.display_name && org.display_name !== org.name && (
            <p className="org-name muted">@{org.name}</p>
          )}
          {org.description && (
            <p className="org-description">{org.description}</p>
          )}
        </div>
      </div>

      {/* Members */}
      <section className="org-section">
        <h2>Members</h2>
        {membersQuery.isLoading && (
          <p className="muted">Loading members...</p>
        )}
        {membersQuery.data && membersQuery.data.length === 0 && (
          <p className="muted">No members.</p>
        )}
        {membersQuery.data && membersQuery.data.length > 0 && (
          <div className="org-members">
            {membersQuery.data.map((m) => (
              <Link
                key={m.user_id}
                to={`/users/${m.username}`}
                className="member-card"
              >
                {m.avatar_url ? (
                  <img
                    src={m.avatar_url}
                    alt={m.username}
                    className="member-avatar"
                  />
                ) : (
                  <div className="member-avatar member-avatar-placeholder">
                    {(m.full_name || m.username).charAt(0).toUpperCase()}
                  </div>
                )}
                <div className="member-info">
                  <span className="member-username">{m.username}</span>
                  {m.full_name && (
                    <span className="member-fullname muted">
                      {m.full_name}
                    </span>
                  )}
                </div>
                <span className={`badge badge-role badge-role-${m.role}`}>
                  {m.role}
                </span>
              </Link>
            ))}
          </div>
        )}
      </section>

      {/* Repos */}
      <section className="org-section">
        <h2>Repositories</h2>
        {reposQuery.isLoading && (
          <p className="muted">Loading repositories...</p>
        )}
        {reposQuery.data && reposQuery.data.length === 0 && (
          <p className="muted">No repositories yet.</p>
        )}
        {reposQuery.data && reposQuery.data.length > 0 && (
          <ul className="org-repos">
            {reposQuery.data.map((repo) => {
              const lang = topLanguage(repo.language_stats);
              return (
                <li key={repo.id} className="org-repo-item">
                  <div className="repo-info">
                    <Link
                      to={`/${repo.owner_name}/${repo.name}`}
                      className="repo-name"
                    >
                      {repo.name}
                    </Link>
                    <span className={`badge badge-${repo.visibility}`}>
                      {repo.visibility}
                    </span>
                  </div>
                  {repo.description && (
                    <p className="repo-description">{repo.description}</p>
                  )}
                  <div className="repo-meta">
                    {lang && <span>{lang}</span>}
                    {repo.stars_count > 0 && (
                      <span>&#9733; {repo.stars_count}</span>
                    )}
                    <span>
                      Updated{" "}
                      {new Date(repo.updated_at).toLocaleDateString()}
                    </span>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </section>
    </div>
  );
}

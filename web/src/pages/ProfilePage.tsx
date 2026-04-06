import { useParams, Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { get } from "../lib/api";
import { avatarUrl } from "../lib/avatar";
import { useAuthStore } from "../stores/auth";
import ContributionGraph from "../components/ContributionGraph";

interface UserProfile {
  id: string;
  username: string;
  full_name: string;
  bio: string;
  avatar_url: string;
  created_at: string;
}

interface PinnedRepo {
  id: string;
  name: string;
  owner_name: string;
  description: string;
  language: string;
  stars: number;
  visibility: string;
}

interface ContributionDay {
  date: string;
  count: number;
}

interface Repo {
  id: string;
  name: string;
  owner_name: string;
  description: string;
  visibility: string;
  language: string;
  stars: number;
  updated_at: string;
}

interface OrgMembership {
  id: string;
  name: string;
  display_name: string;
  avatar_url: string;
  role: string;
}

export default function ProfilePage() {
  const params = useParams();
  const username = params.username || params.owner;
  const currentUser = useAuthStore((s) => s.user);
  const isOwnProfile = currentUser?.username === username;

  const profileQuery = useQuery({
    queryKey: ["profile", username],
    queryFn: () =>
      get<UserProfile>(`/users/${username}`).then((r) => r.data),
    enabled: !!username,
  });

  const pinnedQuery = useQuery({
    queryKey: ["pinned-repos", username],
    queryFn: () =>
      get<PinnedRepo[]>(`/users/${username}/pinned-repos`).then((r) => r.data),
    enabled: !!username,
  });

  const contributionsQuery = useQuery({
    queryKey: ["contributions", username],
    queryFn: () =>
      get<ContributionDay[]>(`/users/${username}/contributions`).then(
        (r) => r.data,
      ),
    enabled: !!username,
  });

  const reposQuery = useQuery({
    queryKey: ["user-repos", username],
    queryFn: () =>
      get<Repo[]>(`/users/${username}/repos`).then((r) => r.data),
    enabled: !!username,
  });

  const orgsQuery = useQuery({
    queryKey: ["user-orgs", username],
    queryFn: () =>
      get<OrgMembership[]>(`/users/${username}/orgs`).then((r) => r.data),
    enabled: !!username,
  });

  if (profileQuery.isLoading) {
    return <p className="muted">Loading profile...</p>;
  }

  if (profileQuery.error) {
    return (
      <div className="error-banner">
        {profileQuery.error instanceof Error
          ? profileQuery.error.message
          : "Failed to load profile"}
      </div>
    );
  }

  const profile = profileQuery.data;
  if (!profile) return null;

  return (
    <div className="profile-page">
      <aside className="profile-sidebar">
        <div className="profile-header">
          {profile.avatar_url ? (
            <img
              src={avatarUrl(profile.avatar_url)}
              alt={profile.username}
              className="profile-avatar"
            />
          ) : (
            <div className="profile-avatar profile-avatar-placeholder">
              {(profile.full_name || profile.username).charAt(0).toUpperCase()}
            </div>
          )}
          {profile.full_name && (
            <h1 className="profile-fullname">{profile.full_name}</h1>
          )}
          <p className="profile-username">{profile.username}</p>
          {profile.bio && <p className="profile-bio">{profile.bio}</p>}
          <p className="profile-joined muted">
            Joined {new Date(profile.created_at).toLocaleDateString("en-US", {
              month: "long",
              year: "numeric",
            })}
          </p>
          {isOwnProfile && (
            <Link to="/settings/profile" className="btn btn-secondary btn-block profile-edit-btn">
              Edit profile
            </Link>
          )}
        </div>

        {/* Organizations */}
        {orgsQuery.data && orgsQuery.data.length > 0 && (
          <div className="profile-orgs">
            <h3 className="profile-orgs-title">Organizations</h3>
            <div className="profile-orgs-list">
              {orgsQuery.data.map((org) => (
                <Link key={org.id} to={`/${org.name}`} className="profile-org-badge" title={org.display_name || org.name}>
                  {org.avatar_url ? (
                    <img
                      src={avatarUrl(org.avatar_url, 64)}
                      alt={org.name}
                      className="profile-org-avatar"
                    />
                  ) : (
                    <div className="profile-org-avatar profile-org-avatar-placeholder">
                      {(org.display_name || org.name).charAt(0).toUpperCase()}
                    </div>
                  )}
                </Link>
              ))}
            </div>
          </div>
        )}
      </aside>

      <div className="profile-main">
        {/* Pinned repos */}
        {pinnedQuery.data && pinnedQuery.data.length > 0 && (
          <section className="profile-section">
            <div className="profile-section-header">
              <h2>Pinned</h2>
              {isOwnProfile && (
                <Link to="/settings/profile" className="muted">
                  Customize pinned repos
                </Link>
              )}
            </div>
            <div className="pinned-repos">
              {pinnedQuery.data.map((repo) => (
                <Link
                  key={repo.id}
                  to={`/${repo.owner_name}/${repo.name}`}
                  className="pinned-repo-card"
                >
                  <div className="pinned-repo-name">{repo.name}</div>
                  {repo.description && (
                    <p className="pinned-repo-desc">
                      {repo.description.length > 100
                        ? repo.description.slice(0, 100) + "..."
                        : repo.description}
                    </p>
                  )}
                  <div className="pinned-repo-meta">
                    {repo.language && (
                      <span className="pinned-repo-lang">{repo.language}</span>
                    )}
                    {repo.stars > 0 && (
                      <span className="pinned-repo-stars">
                        &#9733; {repo.stars}
                      </span>
                    )}
                  </div>
                </Link>
              ))}
            </div>
          </section>
        )}

        {/* Contribution graph */}
        {contributionsQuery.data && (
          <section className="profile-section">
            <ContributionGraph data={contributionsQuery.data} />
          </section>
        )}

        {/* Public repos */}
        <section className="profile-section">
          <h2>Repositories</h2>
          {reposQuery.isLoading && (
            <p className="muted">Loading repositories...</p>
          )}
          {reposQuery.data && reposQuery.data.length === 0 && (
            <p className="muted">No public repositories yet.</p>
          )}
          {reposQuery.data && reposQuery.data.length > 0 && (
            <ul className="profile-repos">
              {reposQuery.data.map((repo) => (
                <li key={repo.id} className="profile-repo-item">
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
                    {repo.language && <span>{repo.language}</span>}
                    {repo.stars > 0 && <span>&#9733; {repo.stars}</span>}
                    <span>
                      Updated{" "}
                      {new Date(repo.updated_at).toLocaleDateString()}
                    </span>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </div>
  );
}

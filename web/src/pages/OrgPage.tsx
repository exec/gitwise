import { useState } from "react";
import { useParams, Link, useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { get, post, put, del, patch } from "../lib/api";
import { avatarUrl } from "../lib/avatar";
import { useAuthStore } from "../stores/auth";

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

interface Team {
  id: string;
  org_id: string;
  name: string;
  description: string;
  permission: string;
  member_count: number;
  repo_count: number;
  created_at: string;
  updated_at: string;
}

interface TeamMember {
  user_id: string;
  username: string;
  full_name: string;
  avatar_url: string;
}

interface TeamRepo {
  repo_id: string;
  name: string;
  description: string;
  visibility: string;
}

function topLanguage(stats: Record<string, number> | null): string | null {
  if (!stats) return null;
  const entries = Object.entries(stats);
  if (entries.length === 0) return null;
  entries.sort((a, b) => b[1] - a[1]);
  return entries[0][0];
}

const PERM_LABELS: Record<string, string> = {
  read: "Read",
  triage: "Triage",
  write: "Write",
  admin: "Admin",
};

type OrgTab = "repos" | "members" | "teams" | "settings";

export default function OrgPage() {
  const params = useParams();
  const name = params.name || params.owner;
  const [activeTab, setActiveTab] = useState<OrgTab>("repos");
  const user = useAuthStore((s) => s.user);

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

  const teamsQuery = useQuery({
    queryKey: ["org-teams", name],
    queryFn: () => get<Team[]>(`/orgs/${name}/teams`).then((r) => r.data),
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

  const isOwner = membersQuery.data?.some(
    (m) => m.user_id === user?.id && m.role === "owner"
  );

  return (
    <div className="org-page">
      <div className="org-header">
        {org.avatar_url ? (
          <img
            src={avatarUrl(org.avatar_url)}
            alt={org.name}
            className="org-avatar"
          />
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

      {/* Tabs */}
      <div className="org-tabs">
        <button
          className={`org-tab ${activeTab === "repos" ? "org-tab-active" : ""}`}
          onClick={() => setActiveTab("repos")}
        >
          Repositories
          {reposQuery.data && (
            <span className="org-tab-count">{reposQuery.data.length}</span>
          )}
        </button>
        <button
          className={`org-tab ${activeTab === "members" ? "org-tab-active" : ""}`}
          onClick={() => setActiveTab("members")}
        >
          Members
          {membersQuery.data && (
            <span className="org-tab-count">{membersQuery.data.length}</span>
          )}
        </button>
        <button
          className={`org-tab ${activeTab === "teams" ? "org-tab-active" : ""}`}
          onClick={() => setActiveTab("teams")}
        >
          Teams
          {teamsQuery.data && (
            <span className="org-tab-count">{teamsQuery.data.length}</span>
          )}
        </button>
        {isOwner && (
          <button
            className={`org-tab ${activeTab === "settings" ? "org-tab-active" : ""}`}
            onClick={() => setActiveTab("settings")}
          >
            Settings
          </button>
        )}
      </div>

      {/* Tab content */}
      {activeTab === "repos" && (
        <ReposTab repos={reposQuery.data} isLoading={reposQuery.isLoading} />
      )}
      {activeTab === "members" && (
        <MembersTab
          members={membersQuery.data}
          isLoading={membersQuery.isLoading}
        />
      )}
      {activeTab === "teams" && (
        <TeamsTab
          orgName={org.name}
          teams={teamsQuery.data}
          isLoading={teamsQuery.isLoading}
          isOwner={!!isOwner}
          orgMembers={membersQuery.data ?? []}
          orgRepos={reposQuery.data ?? []}
        />
      )}
      {activeTab === "settings" && isOwner && (
        <SettingsTab
          org={org}
          members={membersQuery.data ?? []}
        />
      )}
    </div>
  );
}

function ReposTab({
  repos,
  isLoading,
}: {
  repos: Repo[] | undefined;
  isLoading: boolean;
}) {
  if (isLoading) return <p className="muted">Loading repositories...</p>;
  if (!repos || repos.length === 0)
    return <p className="muted">No repositories yet.</p>;

  return (
    <ul className="org-repos">
      {repos.map((repo) => {
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
                Updated {new Date(repo.updated_at).toLocaleDateString()}
              </span>
            </div>
          </li>
        );
      })}
    </ul>
  );
}

function MembersTab({
  members,
  isLoading,
}: {
  members: OrgMember[] | undefined;
  isLoading: boolean;
}) {
  if (isLoading) return <p className="muted">Loading members...</p>;
  if (!members || members.length === 0)
    return <p className="muted">No members.</p>;

  return (
    <div className="org-members">
      {members.map((m) => (
        <Link key={m.user_id} to={`/${m.username}`} className="member-card">
          {m.avatar_url ? (
            <img
              src={avatarUrl(m.avatar_url)}
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
              <span className="member-fullname muted">{m.full_name}</span>
            )}
          </div>
          <span className={`badge badge-role badge-role-${m.role}`}>
            {m.role}
          </span>
        </Link>
      ))}
    </div>
  );
}

function TeamsTab({
  orgName,
  teams,
  isLoading,
  isOwner,
  orgMembers,
  orgRepos,
}: {
  orgName: string;
  teams: Team[] | undefined;
  isLoading: boolean;
  isOwner: boolean;
  orgMembers: OrgMember[];
  orgRepos: Repo[];
}) {
  const [showCreate, setShowCreate] = useState(false);
  const [expandedTeam, setExpandedTeam] = useState<string | null>(null);

  if (isLoading) return <p className="muted">Loading teams...</p>;

  return (
    <div className="teams-section">
      {isOwner && (
        <div className="teams-actions">
          <button
            className="btn btn-primary btn-sm"
            onClick={() => setShowCreate(!showCreate)}
          >
            {showCreate ? "Cancel" : "New team"}
          </button>
        </div>
      )}

      {showCreate && (
        <CreateTeamForm
          orgName={orgName}
          onDone={() => setShowCreate(false)}
        />
      )}

      {(!teams || teams.length === 0) && !showCreate && (
        <p className="muted">No teams yet.</p>
      )}

      {teams && teams.length > 0 && (
        <div className="teams-list">
          {teams.map((t) => (
            <div key={t.id} className="team-card">
              <div
                className="team-card-header"
                onClick={() =>
                  setExpandedTeam(expandedTeam === t.name ? null : t.name)
                }
              >
                <div className="team-card-info">
                  <span className="team-name">{t.name}</span>
                  {t.description && (
                    <span className="team-description muted">
                      {t.description}
                    </span>
                  )}
                </div>
                <div className="team-card-meta">
                  <span
                    className={`badge badge-perm badge-perm-${t.permission}`}
                  >
                    {PERM_LABELS[t.permission] || t.permission}
                  </span>
                  <span className="team-stat">{t.member_count} members</span>
                  <span className="team-stat">{t.repo_count} repos</span>
                  <span className="team-expand-icon">
                    {expandedTeam === t.name ? "\u25B2" : "\u25BC"}
                  </span>
                </div>
              </div>
              {expandedTeam === t.name && (
                <TeamDetail
                  orgName={orgName}
                  team={t}
                  isOwner={isOwner}
                  orgMembers={orgMembers}
                  orgRepos={orgRepos}
                />
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function CreateTeamForm({
  orgName,
  onDone,
}: {
  orgName: string;
  onDone: () => void;
}) {
  const queryClient = useQueryClient();
  const [formName, setFormName] = useState("");
  const [description, setDescription] = useState("");
  const [permission, setPermission] = useState("read");
  const [error, setError] = useState("");

  const createMutation = useMutation({
    mutationFn: () =>
      post(`/orgs/${orgName}/teams`, {
        name: formName,
        description,
        permission,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["org-teams", orgName] });
      onDone();
    },
    onError: (err) => {
      setError(err instanceof Error ? err.message : "Failed to create team");
    },
  });

  return (
    <form
      className="team-form"
      onSubmit={(e) => {
        e.preventDefault();
        setError("");
        createMutation.mutate();
      }}
    >
      <div className="form-group">
        <label htmlFor="team-name">Team name</label>
        <input
          id="team-name"
          type="text"
          value={formName}
          onChange={(e) => setFormName(e.target.value)}
          placeholder="e.g. backend-devs"
          maxLength={100}
          required
        />
      </div>
      <div className="form-group">
        <label htmlFor="team-desc">Description</label>
        <input
          id="team-desc"
          type="text"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Optional description"
        />
      </div>
      <div className="form-group">
        <label htmlFor="team-perm">Permission</label>
        <select
          id="team-perm"
          value={permission}
          onChange={(e) => setPermission(e.target.value)}
        >
          <option value="read">Read</option>
          <option value="triage">Triage</option>
          <option value="write">Write</option>
          <option value="admin">Admin</option>
        </select>
      </div>
      {error && <p className="error-text">{error}</p>}
      <button
        type="submit"
        className="btn btn-primary btn-sm"
        disabled={createMutation.isPending || !formName.trim()}
      >
        {createMutation.isPending ? "Creating..." : "Create team"}
      </button>
    </form>
  );
}

function TeamDetail({
  orgName,
  team,
  isOwner,
  orgMembers,
  orgRepos,
}: {
  orgName: string;
  team: Team;
  isOwner: boolean;
  orgMembers: OrgMember[];
  orgRepos: Repo[];
}) {
  const queryClient = useQueryClient();
  const [addMember, setAddMember] = useState("");
  const [addRepo, setAddRepo] = useState("");

  const membersQuery = useQuery({
    queryKey: ["org-team-members", orgName, team.name],
    queryFn: () =>
      get<TeamMember[]>(`/orgs/${orgName}/teams/${team.name}/members`).then(
        (r) => r.data
      ),
  });

  const reposQuery = useQuery({
    queryKey: ["org-team-repos", orgName, team.name],
    queryFn: () =>
      get<TeamRepo[]>(`/orgs/${orgName}/teams/${team.name}/repos`).then(
        (r) => r.data
      ),
  });

  const addMemberMut = useMutation({
    mutationFn: (username: string) =>
      put(`/orgs/${orgName}/teams/${team.name}/members/${username}`),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["org-team-members", orgName, team.name],
      });
      queryClient.invalidateQueries({ queryKey: ["org-teams", orgName] });
      setAddMember("");
    },
  });

  const removeMemberMut = useMutation({
    mutationFn: (username: string) =>
      del(`/orgs/${orgName}/teams/${team.name}/members/${username}`),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["org-team-members", orgName, team.name],
      });
      queryClient.invalidateQueries({ queryKey: ["org-teams", orgName] });
    },
  });

  const addRepoMut = useMutation({
    mutationFn: (repoName: string) =>
      put(`/orgs/${orgName}/teams/${team.name}/repos/${repoName}`),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["org-team-repos", orgName, team.name],
      });
      queryClient.invalidateQueries({ queryKey: ["org-teams", orgName] });
      setAddRepo("");
    },
  });

  const removeRepoMut = useMutation({
    mutationFn: (repoName: string) =>
      del(`/orgs/${orgName}/teams/${team.name}/repos/${repoName}`),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["org-team-repos", orgName, team.name],
      });
      queryClient.invalidateQueries({ queryKey: ["org-teams", orgName] });
    },
  });

  const deleteTeamMut = useMutation({
    mutationFn: () => del(`/orgs/${orgName}/teams/${team.name}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["org-teams", orgName] });
    },
  });

  // Members available to add (org members not already in the team)
  const currentMemberIds = new Set(
    membersQuery.data?.map((m) => m.user_id) ?? []
  );
  const availableMembers = orgMembers.filter(
    (m) => !currentMemberIds.has(m.user_id)
  );

  // Repos available to add (org repos not already in the team)
  const currentRepoIds = new Set(
    reposQuery.data?.map((r) => r.repo_id) ?? []
  );
  const availableRepos = orgRepos.filter(
    (r) => !currentRepoIds.has(r.id)
  );

  return (
    <div className="team-detail">
      {/* Members */}
      <div className="team-detail-section">
        <h4>Members</h4>
        {membersQuery.isLoading && (
          <p className="muted">Loading members...</p>
        )}
        {membersQuery.data && membersQuery.data.length === 0 && (
          <p className="muted">No members in this team.</p>
        )}
        {membersQuery.data && membersQuery.data.length > 0 && (
          <ul className="team-detail-list">
            {membersQuery.data.map((m) => (
              <li key={m.user_id} className="team-detail-item">
                <Link to={`/${m.username}`} className="team-detail-item-name">
                  {m.username}
                  {m.full_name && (
                    <span className="muted"> ({m.full_name})</span>
                  )}
                </Link>
                {isOwner && (
                  <button
                    className="btn btn-danger btn-xs"
                    onClick={() => removeMemberMut.mutate(m.username)}
                    disabled={removeMemberMut.isPending}
                  >
                    Remove
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
        {isOwner && availableMembers.length > 0 && (
          <div className="team-add-row">
            <select
              value={addMember}
              onChange={(e) => setAddMember(e.target.value)}
            >
              <option value="">Add member...</option>
              {availableMembers.map((m) => (
                <option key={m.user_id} value={m.username}>
                  {m.username}
                </option>
              ))}
            </select>
            <button
              className="btn btn-primary btn-xs"
              disabled={!addMember || addMemberMut.isPending}
              onClick={() => addMember && addMemberMut.mutate(addMember)}
            >
              Add
            </button>
          </div>
        )}
      </div>

      {/* Repos */}
      <div className="team-detail-section">
        <h4>Repositories</h4>
        {reposQuery.isLoading && <p className="muted">Loading repos...</p>}
        {reposQuery.data && reposQuery.data.length === 0 && (
          <p className="muted">No repositories assigned to this team.</p>
        )}
        {reposQuery.data && reposQuery.data.length > 0 && (
          <ul className="team-detail-list">
            {reposQuery.data.map((r) => (
              <li key={r.repo_id} className="team-detail-item">
                <Link
                  to={`/${orgName}/${r.name}`}
                  className="team-detail-item-name"
                >
                  {r.name}
                </Link>
                <span className={`badge badge-${r.visibility}`}>
                  {r.visibility}
                </span>
                {isOwner && (
                  <button
                    className="btn btn-danger btn-xs"
                    onClick={() => removeRepoMut.mutate(r.name)}
                    disabled={removeRepoMut.isPending}
                  >
                    Remove
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
        {isOwner && availableRepos.length > 0 && (
          <div className="team-add-row">
            <select
              value={addRepo}
              onChange={(e) => setAddRepo(e.target.value)}
            >
              <option value="">Add repository...</option>
              {availableRepos.map((r) => (
                <option key={r.id} value={r.name}>
                  {r.name}
                </option>
              ))}
            </select>
            <button
              className="btn btn-primary btn-xs"
              disabled={!addRepo || addRepoMut.isPending}
              onClick={() => addRepo && addRepoMut.mutate(addRepo)}
            >
              Add
            </button>
          </div>
        )}
      </div>

      {/* Delete team */}
      {isOwner && (
        <div className="team-detail-section team-danger-zone">
          <button
            className="btn btn-danger btn-sm"
            onClick={() => {
              if (window.confirm(`Delete team "${team.name}"?`)) {
                deleteTeamMut.mutate();
              }
            }}
            disabled={deleteTeamMut.isPending}
          >
            Delete team
          </button>
        </div>
      )}
    </div>
  );
}

function SettingsTab({
  org,
  members,
}: {
  org: Organization;
  members: OrgMember[];
}) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  // General settings
  const [displayName, setDisplayName] = useState(org.display_name || "");
  const [description, setDescription] = useState(org.description || "");
  const [avatarUrlVal, setAvatarUrlVal] = useState(org.avatar_url || "");
  const [generalError, setGeneralError] = useState("");
  const [generalSuccess, setGeneralSuccess] = useState("");

  const updateMut = useMutation({
    mutationFn: () =>
      put(`/orgs/${org.name}`, {
        display_name: displayName,
        description,
        avatar_url: avatarUrlVal,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["org", org.name] });
      queryClient.invalidateQueries({ queryKey: ["resolve", org.name] });
      setGeneralSuccess("Organization updated.");
      setGeneralError("");
    },
    onError: (err) => {
      setGeneralError(
        err instanceof Error ? err.message : "Failed to update organization"
      );
      setGeneralSuccess("");
    },
  });

  // Member management
  const [newMember, setNewMember] = useState("");
  const [newMemberRole, setNewMemberRole] = useState("member");
  const [memberError, setMemberError] = useState("");

  const addMemberMut = useMutation({
    mutationFn: () =>
      put(`/orgs/${org.name}/members/${newMember}`, { role: newMemberRole }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["org-members", org.name] });
      setNewMember("");
      setNewMemberRole("member");
      setMemberError("");
    },
    onError: (err) => {
      setMemberError(
        err instanceof Error ? err.message : "Failed to add member"
      );
    },
  });

  const removeMemberMut = useMutation({
    mutationFn: (username: string) =>
      del(`/orgs/${org.name}/members/${username}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["org-members", org.name] });
      setMemberError("");
    },
    onError: (err) => {
      setMemberError(
        err instanceof Error ? err.message : "Failed to remove member"
      );
    },
  });

  const changeRoleMut = useMutation({
    mutationFn: ({
      username,
      role,
    }: {
      username: string;
      role: string;
    }) => put(`/orgs/${org.name}/members/${username}`, { role }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["org-members", org.name] });
      setMemberError("");
    },
    onError: (err) => {
      setMemberError(
        err instanceof Error ? err.message : "Failed to change role"
      );
    },
  });

  // Delete org
  const deleteMut = useMutation({
    mutationFn: () => del(`/orgs/${org.name}`),
    onSuccess: () => {
      navigate("/");
    },
  });

  return (
    <div className="org-settings">
      {/* General */}
      <section className="settings-section">
        <h3>General</h3>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            updateMut.mutate();
          }}
        >
          <div className="form-group">
            <label htmlFor="org-display-name">Display name</label>
            <input
              id="org-display-name"
              type="text"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
            />
          </div>
          <div className="form-group">
            <label htmlFor="org-description">Description</label>
            <textarea
              id="org-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
            />
          </div>
          <div className="form-group">
            <label htmlFor="org-avatar-url">Avatar URL</label>
            <input
              id="org-avatar-url"
              type="text"
              value={avatarUrlVal}
              onChange={(e) => setAvatarUrlVal(e.target.value)}
              placeholder="https://..."
            />
          </div>
          {generalError && <p className="error-text">{generalError}</p>}
          {generalSuccess && <p className="success-text">{generalSuccess}</p>}
          <button
            type="submit"
            className="btn btn-primary btn-sm"
            disabled={updateMut.isPending}
          >
            {updateMut.isPending ? "Saving..." : "Save changes"}
          </button>
        </form>
      </section>

      {/* Members */}
      <section className="settings-section">
        <h3>Members</h3>
        {memberError && <p className="error-text">{memberError}</p>}

        <div className="settings-members-list">
          {members.map((m) => (
            <div key={m.user_id} className="settings-member-row">
              <Link to={`/${m.username}`} className="settings-member-name">
                {m.username}
                {m.full_name && (
                  <span className="muted"> ({m.full_name})</span>
                )}
              </Link>
              <select
                value={m.role}
                onChange={(e) =>
                  changeRoleMut.mutate({
                    username: m.username,
                    role: e.target.value,
                  })
                }
                disabled={changeRoleMut.isPending}
              >
                <option value="owner">Owner</option>
                <option value="member">Member</option>
              </select>
              <button
                className="btn btn-danger btn-xs"
                onClick={() => {
                  if (
                    window.confirm(
                      `Remove ${m.username} from ${org.name}?`
                    )
                  ) {
                    removeMemberMut.mutate(m.username);
                  }
                }}
                disabled={removeMemberMut.isPending}
              >
                Remove
              </button>
            </div>
          ))}
        </div>

        <div className="settings-add-member">
          <input
            type="text"
            value={newMember}
            onChange={(e) => setNewMember(e.target.value)}
            placeholder="Username to add"
          />
          <select
            value={newMemberRole}
            onChange={(e) => setNewMemberRole(e.target.value)}
          >
            <option value="member">Member</option>
            <option value="owner">Owner</option>
          </select>
          <button
            className="btn btn-primary btn-xs"
            onClick={() => {
              if (newMember.trim()) {
                addMemberMut.mutate();
              }
            }}
            disabled={addMemberMut.isPending || !newMember.trim()}
          >
            Add member
          </button>
        </div>
      </section>

      {/* Danger zone */}
      <section className="settings-section settings-danger-zone">
        <h3>Danger zone</h3>
        <p className="muted">
          Deleting an organization is permanent. All repositories, teams, and
          members will be removed.
        </p>
        <button
          className="btn btn-danger btn-sm"
          onClick={() => {
            if (
              window.confirm(
                `Are you sure you want to delete "${org.name}"? This cannot be undone.`
              )
            ) {
              deleteMut.mutate();
            }
          }}
          disabled={deleteMut.isPending}
        >
          {deleteMut.isPending ? "Deleting..." : "Delete organization"}
        </button>
      </section>
    </div>
  );
}

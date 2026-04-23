import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { get, post, put, del } from "../lib/api";

const ADMIN_PREFIX = "/admin-8bc6d1f";

interface AdminUser {
  id: string;
  username: string;
  email: string;
  full_name: string;
  is_admin: boolean;
  created_at: string;
  updated_at: string;
  repo_count: number;
}

interface SystemStats {
  user_count: number;
  repo_count: number;
  commit_count: number;
  issue_count: number;
  pr_count: number;
  disk_usage: string;
  active_sessions: number;
}

interface WebhookDelivery {
  id: string;
  webhook_id: string;
  event_type: string;
  response_status: number | null;
  success: boolean;
  attempts: number;
  duration_ms: number;
  delivered_at: string;
  webhook_url: string;
  repo_name: string;
  owner_name: string;
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function StatsCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="admin-stat-card">
      <div className="admin-stat-value">{value}</div>
      <div className="admin-stat-label">{label}</div>
    </div>
  );
}

function DashboardTab() {
  const { data: stats, isLoading, error } = useQuery({
    queryKey: ["admin-stats"],
    queryFn: async () => {
      const { data } = await get<SystemStats>(`${ADMIN_PREFIX}/stats`);
      return data;
    },
  });

  if (isLoading) return <p className="muted">Loading stats...</p>;
  if (error) {
    return (
      <div className="error-banner">
        {error instanceof Error ? error.message : "Failed to load system stats"}
      </div>
    );
  }

  return (
    <div>
      <h2>System Overview</h2>
      {stats && (
        <div className="admin-stats-grid">
          <StatsCard label="Users" value={stats.user_count} />
          <StatsCard label="Repositories" value={stats.repo_count} />
          <StatsCard label="Commits" value={stats.commit_count.toLocaleString()} />
          <StatsCard label="Issues" value={stats.issue_count} />
          <StatsCard label="Pull Requests" value={stats.pr_count} />
          <StatsCard label="Database Size" value={stats.disk_usage} />
        </div>
      )}
    </div>
  );
}

function UsersTab() {
  const [search, setSearch] = useState("");
  const [editingUser, setEditingUser] = useState<string | null>(null);
  const queryClient = useQueryClient();

  const { data, isLoading } = useQuery({
    queryKey: ["admin-users", search],
    queryFn: async () => {
      const params = new URLSearchParams({ limit: "50" });
      if (search) params.set("q", search);
      const res = await get<AdminUser[]>(`${ADMIN_PREFIX}/users?${params}`);
      return res;
    },
  });

  const users = data?.data ?? [];
  const total = (data?.meta as Record<string, number> | undefined)?.total ?? 0;

  const toggleAdmin = useMutation({
    mutationFn: async ({ id, isAdmin }: { id: string; isAdmin: boolean }) => {
      await put(`${ADMIN_PREFIX}/users/${id}`, { is_admin: isAdmin });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin-users"] });
      setEditingUser(null);
    },
  });

  const deleteUser = useMutation({
    mutationFn: async (id: string) => {
      await del(`${ADMIN_PREFIX}/users/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin-users"] });
      queryClient.invalidateQueries({ queryKey: ["admin-stats"] });
    },
  });

  const handleDelete = (user: AdminUser) => {
    if (window.confirm(`Delete user "${user.username}" and all their data? This cannot be undone.`)) {
      deleteUser.mutate(user.id);
    }
  };

  return (
    <div>
      <div className="admin-section-header">
        <h2>Users ({total})</h2>
        <input
          type="text"
          placeholder="Search users..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="admin-search-input"
        />
      </div>

      {isLoading && <p className="muted">Loading users...</p>}

      {!isLoading && users.length === 0 && (
        <p className="muted">No users found.</p>
      )}

      {users.length > 0 && (
        <div className="admin-table-wrap">
          <table className="admin-table">
            <thead>
              <tr>
                <th>Username</th>
                <th>Email</th>
                <th>Repos</th>
                <th>Admin</th>
                <th>Joined</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {users.map((user) => (
                <tr key={user.id}>
                  <td>
                    <a href={`/${user.username}`}>{user.username}</a>
                    {user.full_name && (
                      <span className="admin-user-fullname"> ({user.full_name})</span>
                    )}
                  </td>
                  <td className="admin-email">{user.email}</td>
                  <td>{user.repo_count}</td>
                  <td>
                    {editingUser === user.id ? (
                      <div className="admin-toggle-group">
                        <button
                          className="btn btn-sm btn-primary"
                          onClick={() => toggleAdmin.mutate({ id: user.id, isAdmin: !user.is_admin })}
                          disabled={toggleAdmin.isPending}
                        >
                          {user.is_admin ? "Revoke" : "Grant"}
                        </button>
                        <button
                          className="btn btn-sm btn-secondary"
                          onClick={() => setEditingUser(null)}
                        >
                          Cancel
                        </button>
                      </div>
                    ) : (
                      <span
                        className={`admin-badge ${user.is_admin ? "admin-badge-yes" : "admin-badge-no"}`}
                        onClick={() => setEditingUser(user.id)}
                        style={{ cursor: "pointer" }}
                        title="Click to toggle"
                      >
                        {user.is_admin ? "Yes" : "No"}
                      </span>
                    )}
                  </td>
                  <td className="admin-date">{formatDate(user.created_at)}</td>
                  <td>
                    <button
                      className="btn btn-sm btn-danger"
                      onClick={() => handleDelete(user)}
                      disabled={deleteUser.isPending}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function JobsTab() {
  const queryClient = useQueryClient();

  const { data: deliveries, isLoading } = useQuery({
    queryKey: ["admin-jobs"],
    queryFn: async () => {
      const { data } = await get<WebhookDelivery[]>(`${ADMIN_PREFIX}/jobs`);
      return data;
    },
  });

  const reindex = useMutation({
    mutationFn: async () => {
      await post(`${ADMIN_PREFIX}/reindex-commits`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin-stats"] });
    },
  });

  return (
    <div>
      <div className="admin-section-header">
        <h2>Background Jobs</h2>
        <button
          className="btn btn-primary btn-sm"
          onClick={() => reindex.mutate()}
          disabled={reindex.isPending}
        >
          {reindex.isPending ? "Reindexing..." : "Reindex All Commits"}
        </button>
      </div>

      {reindex.isSuccess && (
        <div className="admin-success-banner">Commit reindexing started in background.</div>
      )}

      <h3 className="admin-subsection-title">Recent Webhook Deliveries</h3>

      {isLoading && <p className="muted">Loading deliveries...</p>}

      {!isLoading && (!deliveries || deliveries.length === 0) && (
        <p className="muted">No webhook deliveries yet.</p>
      )}

      {deliveries && deliveries.length > 0 && (
        <div className="admin-table-wrap">
          <table className="admin-table">
            <thead>
              <tr>
                <th>Event</th>
                <th>Repository</th>
                <th>Status</th>
                <th>Duration</th>
                <th>Attempts</th>
                <th>Delivered</th>
              </tr>
            </thead>
            <tbody>
              {deliveries.map((d) => (
                <tr key={d.id}>
                  <td>
                    <span className="admin-event-badge">{d.event_type}</span>
                  </td>
                  <td>
                    {d.owner_name && d.repo_name
                      ? <a href={`/${d.owner_name}/${d.repo_name}`}>{d.owner_name}/{d.repo_name}</a>
                      : <span className="muted">-</span>
                    }
                  </td>
                  <td>
                    <span className={`admin-status ${d.success ? "admin-status-ok" : "admin-status-fail"}`}>
                      {d.success ? "OK" : "Failed"}
                      {d.response_status !== null && ` (${d.response_status})`}
                    </span>
                  </td>
                  <td>{d.duration_ms}ms</td>
                  <td>{d.attempts}</td>
                  <td className="admin-date">{formatDate(d.delivered_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

export default function AdminPage() {
  const [activeTab, setActiveTab] = useState<"dashboard" | "users" | "jobs">("dashboard");

  return (
    <div className="settings-page">
      <nav className="settings-sidebar">
        <div className="admin-sidebar-title">Admin Panel</div>
        <button
          className={`settings-tab ${activeTab === "dashboard" ? "active" : ""}`}
          onClick={() => setActiveTab("dashboard")}
        >
          Dashboard
        </button>
        <button
          className={`settings-tab ${activeTab === "users" ? "active" : ""}`}
          onClick={() => setActiveTab("users")}
        >
          Users
        </button>
        <button
          className={`settings-tab ${activeTab === "jobs" ? "active" : ""}`}
          onClick={() => setActiveTab("jobs")}
        >
          Jobs
        </button>
      </nav>
      <div className="settings-content">
        {activeTab === "dashboard" && <DashboardTab />}
        {activeTab === "users" && <UsersTab />}
        {activeTab === "jobs" && <JobsTab />}
      </div>
    </div>
  );
}

import { useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { get, post, patch, del } from "../lib/api";
import RepoHeader from "../components/RepoHeader";

type SettingsTab = "general" | "webhooks" | "branch-protection" | "labels" | "milestones";

interface Repo {
  id: string;
  name: string;
  description: string;
  default_branch: string;
  visibility: string;
}

interface Webhook {
  id: string;
  url: string;
  secret: string;
  events: string[];
  active: boolean;
  created_at: string;
}

interface WebhookDelivery {
  id: string;
  event: string;
  status_code: number;
  success: boolean;
  delivered_at: string;
}

interface BranchProtection {
  id: string;
  branch_pattern: string;
  required_reviews: number;
  require_linear_history: boolean;
}

interface Label {
  id: string;
  name: string;
  color: string;
  description: string;
}

interface Milestone {
  id: string;
  title: string;
  description: string;
  due_date: string | null;
  status: string;
  open_issues: number;
  closed_issues: number;
}

const WEBHOOK_EVENTS = [
  "push",
  "ping",
  "issue.opened",
  "issue.closed",
  "pr.opened",
  "pr.merged",
  "pr.closed",
  "review.submitted",
  "comment.created",
];

const DEFAULT_COLORS = [
  "#e11d48", "#f97316", "#eab308", "#22c55e", "#06b6d4",
  "#3b82f6", "#8b5cf6", "#ec4899", "#6b7280", "#1d4ed8",
];

export default function RepoSettingsPage() {
  const { owner, repo } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [activeTab, setActiveTab] = useState<SettingsTab>("general");

  return (
    <div className="repo-page">
      <RepoHeader owner={owner!} repo={repo!} activeTab="settings" />
      <div className="settings-page">
        <nav className="settings-sidebar">
          {(["general", "webhooks", "branch-protection", "labels", "milestones"] as SettingsTab[]).map((tab) => (
            <button
              key={tab}
              className={`settings-tab ${activeTab === tab ? "active" : ""}`}
              onClick={() => setActiveTab(tab)}
            >
              {tab === "branch-protection" ? "Branch Protection" : tab.charAt(0).toUpperCase() + tab.slice(1)}
            </button>
          ))}
        </nav>
        <div className="settings-content">
          {activeTab === "general" && (
            <GeneralTab owner={owner!} repo={repo!} navigate={navigate} queryClient={queryClient} />
          )}
          {activeTab === "webhooks" && (
            <WebhooksTab owner={owner!} repo={repo!} queryClient={queryClient} />
          )}
          {activeTab === "branch-protection" && (
            <BranchProtectionTab owner={owner!} repo={repo!} queryClient={queryClient} />
          )}
          {activeTab === "labels" && (
            <LabelsTab owner={owner!} repo={repo!} queryClient={queryClient} />
          )}
          {activeTab === "milestones" && (
            <MilestonesTab owner={owner!} repo={repo!} queryClient={queryClient} />
          )}
        </div>
      </div>
    </div>
  );
}

/* ---- General Tab ---- */

function GeneralTab({
  owner,
  repo,
  navigate,
  queryClient,
}: {
  owner: string;
  repo: string;
  navigate: ReturnType<typeof useNavigate>;
  queryClient: ReturnType<typeof useQueryClient>;
}) {
  const repoQuery = useQuery({
    queryKey: ["repo", owner, repo],
    queryFn: () => get<Repo>(`/repos/${owner}/${repo}`).then((r) => r.data),
  });

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [defaultBranch, setDefaultBranch] = useState("");
  const [visibility, setVisibility] = useState("public");
  const [initialized, setInitialized] = useState(false);
  const [error, setError] = useState("");
  const [deleteConfirm, setDeleteConfirm] = useState(false);
  const [deleteInput, setDeleteInput] = useState("");

  if (repoQuery.data && !initialized) {
    setName(repoQuery.data.name);
    setDescription(repoQuery.data.description ?? "");
    setDefaultBranch(repoQuery.data.default_branch ?? "main");
    setVisibility(repoQuery.data.visibility ?? "public");
    setInitialized(true);
  }

  const updateMutation = useMutation({
    mutationFn: () =>
      patch(`/repos/${owner}/${repo}`, {
        name,
        description,
        default_branch: defaultBranch,
        visibility,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["repo", owner, repo] });
      setError("");
    },
    onError: (err: Error) => setError(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: () => del(`/repos/${owner}/${repo}`),
    onSuccess: () => navigate(`/${owner}`),
    onError: (err: Error) => setError(err.message),
  });

  if (repoQuery.isLoading) return <p className="muted">Loading...</p>;

  return (
    <div>
      <h2>General</h2>
      {error && <div className="error-banner">{error}</div>}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          updateMutation.mutate();
        }}
      >
        <div className="form-group">
          <label htmlFor="repo-name">Repository name</label>
          <input
            id="repo-name"
            type="text"
            className="form-input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
          />
        </div>
        <div className="form-group">
          <label htmlFor="repo-desc">Description</label>
          <input
            id="repo-desc"
            type="text"
            className="form-input"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </div>
        <div className="form-group">
          <label htmlFor="default-branch">Default branch</label>
          <input
            id="default-branch"
            type="text"
            className="form-input"
            value={defaultBranch}
            onChange={(e) => setDefaultBranch(e.target.value)}
            required
          />
        </div>
        <div className="form-group">
          <label htmlFor="visibility">Visibility</label>
          <select
            id="visibility"
            className="form-input"
            value={visibility}
            onChange={(e) => setVisibility(e.target.value)}
          >
            <option value="public">Public</option>
            <option value="private">Private</option>
          </select>
        </div>
        <button type="submit" className="btn btn-primary" disabled={updateMutation.isPending}>
          {updateMutation.isPending ? "Saving..." : "Save changes"}
        </button>
      </form>

      <div className="danger-zone">
        <h3>Danger Zone</h3>
        <div className="danger-zone-item">
          <div>
            <strong>Delete this repository</strong>
            <p className="muted">Once deleted, there is no going back.</p>
          </div>
          <button className="btn btn-danger" onClick={() => setDeleteConfirm(true)}>
            Delete repository
          </button>
        </div>
      </div>

      {deleteConfirm && (
        <div className="confirm-overlay" onClick={() => setDeleteConfirm(false)}>
          <div className="confirm-dialog" onClick={(e) => e.stopPropagation()}>
            <h3>Are you sure?</h3>
            <p>
              This will permanently delete <strong>{owner}/{repo}</strong> and all its data.
            </p>
            <p className="muted">
              Type <strong>{owner}/{repo}</strong> to confirm:
            </p>
            <input
              type="text"
              className="form-input"
              value={deleteInput}
              onChange={(e) => setDeleteInput(e.target.value)}
              placeholder={`${owner}/${repo}`}
            />
            <div className="confirm-actions">
              <button className="btn btn-secondary" onClick={() => setDeleteConfirm(false)}>
                Cancel
              </button>
              <button
                className="btn btn-danger"
                disabled={deleteInput !== `${owner}/${repo}` || deleteMutation.isPending}
                onClick={() => deleteMutation.mutate()}
              >
                {deleteMutation.isPending ? "Deleting..." : "Delete this repository"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

/* ---- Webhooks Tab ---- */

function WebhooksTab({
  owner,
  repo,
  queryClient,
}: {
  owner: string;
  repo: string;
  queryClient: ReturnType<typeof useQueryClient>;
}) {
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [url, setUrl] = useState("");
  const [secret, setSecret] = useState("");
  const [events, setEvents] = useState<string[]>(["push"]);
  const [active, setActive] = useState(true);
  const [error, setError] = useState("");
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const webhooksQuery = useQuery({
    queryKey: ["webhooks", owner, repo],
    queryFn: () => get<Webhook[]>(`/repos/${owner}/${repo}/webhooks`).then((r) => r.data),
  });

  const createMutation = useMutation({
    mutationFn: () => post(`/repos/${owner}/${repo}/webhooks`, { url, secret, events, active }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["webhooks", owner, repo] });
      resetForm();
    },
    onError: (err: Error) => setError(err.message),
  });

  const updateMutation = useMutation({
    mutationFn: (id: string) =>
      patch(`/repos/${owner}/${repo}/webhooks/${id}`, { url, secret: secret || undefined, events, active }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["webhooks", owner, repo] });
      resetForm();
    },
    onError: (err: Error) => setError(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => del(`/repos/${owner}/${repo}/webhooks/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["webhooks", owner, repo] }),
    onError: (err: Error) => setError(err.message),
  });

  const testMutation = useMutation({
    mutationFn: (id: string) => post(`/repos/${owner}/${repo}/webhooks/${id}/test`),
    onSuccess: (_data, id) =>
      queryClient.invalidateQueries({ queryKey: ["webhook-deliveries", owner, repo, id] }),
    onError: (err: Error) => setError(err.message),
  });

  function resetForm() {
    setShowForm(false);
    setEditId(null);
    setUrl("");
    setSecret("");
    setEvents(["push"]);
    setActive(true);
    setError("");
  }

  function startEdit(wh: Webhook) {
    setEditId(wh.id);
    setUrl(wh.url);
    setSecret("");
    setEvents(wh.events);
    setActive(wh.active);
    setShowForm(true);
  }

  function toggleEvent(ev: string) {
    setEvents((prev) => (prev.includes(ev) ? prev.filter((e) => e !== ev) : [...prev, ev]));
  }

  return (
    <div>
      <div className="settings-header">
        <h2>Webhooks</h2>
        {!showForm && (
          <button className="btn btn-primary" onClick={() => { resetForm(); setShowForm(true); }}>
            Add webhook
          </button>
        )}
      </div>
      {error && <div className="error-banner">{error}</div>}

      {showForm && (
        <div className="settings-form-card">
          <h3>{editId ? "Edit webhook" : "New webhook"}</h3>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              editId ? updateMutation.mutate(editId) : createMutation.mutate();
            }}
          >
            <div className="form-group">
              <label htmlFor="wh-url">Payload URL</label>
              <input
                id="wh-url"
                type="url"
                className="form-input"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="https://example.com/webhook"
                required
              />
            </div>
            <div className="form-group">
              <label htmlFor="wh-secret">Secret {editId && "(leave blank to keep current)"}</label>
              <input
                id="wh-secret"
                type="text"
                className="form-input"
                value={secret}
                onChange={(e) => setSecret(e.target.value)}
                placeholder="webhook-secret"
              />
            </div>
            <div className="form-group">
              <label>Events</label>
              <div className="event-checkboxes">
                {WEBHOOK_EVENTS.map((ev) => (
                  <label key={ev} className="checkbox-label">
                    <input
                      type="checkbox"
                      checked={events.includes(ev)}
                      onChange={() => toggleEvent(ev)}
                    />
                    {ev}
                  </label>
                ))}
              </div>
            </div>
            <div className="form-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  checked={active}
                  onChange={(e) => setActive(e.target.checked)}
                />
                Active
              </label>
            </div>
            <div className="form-actions">
              <button type="submit" className="btn btn-primary" disabled={createMutation.isPending || updateMutation.isPending}>
                {editId ? "Update webhook" : "Create webhook"}
              </button>
              <button type="button" className="btn btn-secondary" onClick={resetForm}>
                Cancel
              </button>
            </div>
          </form>
        </div>
      )}

      {webhooksQuery.isLoading && <p className="muted">Loading webhooks...</p>}

      {webhooksQuery.data && webhooksQuery.data.length === 0 && !showForm && (
        <p className="muted">No webhooks configured.</p>
      )}

      {webhooksQuery.data && webhooksQuery.data.length > 0 && (
        <ul className="webhook-list">
          {webhooksQuery.data.map((wh) => (
            <li key={wh.id} className="webhook-item">
              <div className="webhook-info">
                <span className={`delivery-status ${wh.active ? "success" : "inactive"}`} />
                <code className="webhook-url">{wh.url}</code>
                <span className="muted">{wh.events.join(", ")}</span>
              </div>
              <div className="webhook-actions">
                <button
                  className="btn btn-sm btn-secondary"
                  onClick={() => testMutation.mutate(wh.id)}
                  disabled={testMutation.isPending}
                >
                  Test
                </button>
                <button className="btn btn-sm btn-secondary" onClick={() => startEdit(wh)}>
                  Edit
                </button>
                <button
                  className="btn btn-sm btn-danger"
                  onClick={() => deleteMutation.mutate(wh.id)}
                >
                  Delete
                </button>
                <button
                  className="btn btn-sm btn-secondary"
                  onClick={() => setExpandedId(expandedId === wh.id ? null : wh.id)}
                >
                  {expandedId === wh.id ? "Hide deliveries" : "Deliveries"}
                </button>
              </div>
              {expandedId === wh.id && (
                <WebhookDeliveries owner={owner} repo={repo} webhookId={wh.id} />
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function WebhookDeliveries({
  owner,
  repo,
  webhookId,
}: {
  owner: string;
  repo: string;
  webhookId: string;
}) {
  const deliveriesQuery = useQuery({
    queryKey: ["webhook-deliveries", owner, repo, webhookId],
    queryFn: () =>
      get<WebhookDelivery[]>(`/repos/${owner}/${repo}/webhooks/${webhookId}/deliveries`).then(
        (r) => r.data,
      ),
  });

  if (deliveriesQuery.isLoading) return <p className="muted">Loading deliveries...</p>;
  if (!deliveriesQuery.data?.length) return <p className="muted">No recent deliveries.</p>;

  return (
    <div className="deliveries-list">
      {deliveriesQuery.data.map((d) => (
        <div key={d.id} className="delivery-item">
          <span className={`delivery-status ${d.success ? "success" : "failure"}`} />
          <span className="delivery-event">{d.event}</span>
          <span className="muted">HTTP {d.status_code}</span>
          <span className="muted">{new Date(d.delivered_at).toLocaleString()}</span>
        </div>
      ))}
    </div>
  );
}

/* ---- Branch Protection Tab ---- */

function BranchProtectionTab({
  owner,
  repo,
  queryClient,
}: {
  owner: string;
  repo: string;
  queryClient: ReturnType<typeof useQueryClient>;
}) {
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [branchPattern, setBranchPattern] = useState("");
  const [requiredReviews, setRequiredReviews] = useState(1);
  const [requireLinearHistory, setRequireLinearHistory] = useState(false);
  const [error, setError] = useState("");

  const rulesQuery = useQuery({
    queryKey: ["branch-protection", owner, repo],
    queryFn: () =>
      get<BranchProtection[]>(`/repos/${owner}/${repo}/branch-protection`).then((r) => r.data),
  });

  const createMutation = useMutation({
    mutationFn: () =>
      post(`/repos/${owner}/${repo}/branch-protection`, {
        branch_pattern: branchPattern,
        required_reviews: requiredReviews,
        require_linear_history: requireLinearHistory,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["branch-protection", owner, repo] });
      resetForm();
    },
    onError: (err: Error) => setError(err.message),
  });

  const updateMutation = useMutation({
    mutationFn: (id: string) =>
      patch(`/repos/${owner}/${repo}/branch-protection/${id}`, {
        branch_pattern: branchPattern,
        required_reviews: requiredReviews,
        require_linear_history: requireLinearHistory,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["branch-protection", owner, repo] });
      resetForm();
    },
    onError: (err: Error) => setError(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => del(`/repos/${owner}/${repo}/branch-protection/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["branch-protection", owner, repo] }),
    onError: (err: Error) => setError(err.message),
  });

  function resetForm() {
    setShowForm(false);
    setEditId(null);
    setBranchPattern("");
    setRequiredReviews(1);
    setRequireLinearHistory(false);
    setError("");
  }

  function startEdit(rule: BranchProtection) {
    setEditId(rule.id);
    setBranchPattern(rule.branch_pattern);
    setRequiredReviews(rule.required_reviews);
    setRequireLinearHistory(rule.require_linear_history);
    setShowForm(true);
  }

  return (
    <div>
      <div className="settings-header">
        <h2>Branch Protection Rules</h2>
        {!showForm && (
          <button className="btn btn-primary" onClick={() => { resetForm(); setShowForm(true); }}>
            Add rule
          </button>
        )}
      </div>
      {error && <div className="error-banner">{error}</div>}

      {showForm && (
        <div className="settings-form-card">
          <h3>{editId ? "Edit rule" : "New rule"}</h3>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              editId ? updateMutation.mutate(editId) : createMutation.mutate();
            }}
          >
            <div className="form-group">
              <label htmlFor="bp-pattern">Branch name pattern</label>
              <input
                id="bp-pattern"
                type="text"
                className="form-input"
                value={branchPattern}
                onChange={(e) => setBranchPattern(e.target.value)}
                placeholder="main, release/*, feature/*"
                required
              />
            </div>
            <div className="form-group">
              <label htmlFor="bp-reviews">Required approving reviews</label>
              <input
                id="bp-reviews"
                type="number"
                className="form-input"
                value={requiredReviews}
                onChange={(e) => setRequiredReviews(parseInt(e.target.value, 10) || 0)}
                min={0}
                max={10}
              />
            </div>
            <div className="form-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  checked={requireLinearHistory}
                  onChange={(e) => setRequireLinearHistory(e.target.checked)}
                />
                Require linear history
              </label>
            </div>
            <div className="form-actions">
              <button type="submit" className="btn btn-primary" disabled={createMutation.isPending || updateMutation.isPending}>
                {editId ? "Update rule" : "Create rule"}
              </button>
              <button type="button" className="btn btn-secondary" onClick={resetForm}>
                Cancel
              </button>
            </div>
          </form>
        </div>
      )}

      {rulesQuery.isLoading && <p className="muted">Loading rules...</p>}

      {rulesQuery.data && rulesQuery.data.length === 0 && !showForm && (
        <p className="muted">No branch protection rules.</p>
      )}

      {rulesQuery.data && rulesQuery.data.length > 0 && (
        <ul className="protection-list">
          {rulesQuery.data.map((rule) => (
            <li key={rule.id} className="protection-item">
              <div className="protection-info">
                <code>{rule.branch_pattern}</code>
                <span className="muted">
                  {rule.required_reviews} review{rule.required_reviews !== 1 ? "s" : ""} required
                  {rule.require_linear_history ? " | linear history" : ""}
                </span>
              </div>
              <div className="protection-actions">
                <button className="btn btn-sm btn-secondary" onClick={() => startEdit(rule)}>
                  Edit
                </button>
                <button
                  className="btn btn-sm btn-danger"
                  onClick={() => deleteMutation.mutate(rule.id)}
                >
                  Delete
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/* ---- Labels Tab ---- */

function LabelsTab({
  owner,
  repo,
  queryClient,
}: {
  owner: string;
  repo: string;
  queryClient: ReturnType<typeof useQueryClient>;
}) {
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [labelName, setLabelName] = useState("");
  const [labelColor, setLabelColor] = useState(DEFAULT_COLORS[0]);
  const [labelDesc, setLabelDesc] = useState("");
  const [error, setError] = useState("");

  const labelsQuery = useQuery({
    queryKey: ["labels", owner, repo],
    queryFn: () => get<Label[]>(`/repos/${owner}/${repo}/labels`).then((r) => r.data),
  });

  const createMutation = useMutation({
    mutationFn: () =>
      post(`/repos/${owner}/${repo}/labels`, { name: labelName, color: labelColor, description: labelDesc }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["labels", owner, repo] });
      resetForm();
    },
    onError: (err: Error) => setError(err.message),
  });

  const updateMutation = useMutation({
    mutationFn: (id: string) =>
      patch(`/repos/${owner}/${repo}/labels/${id}`, { name: labelName, color: labelColor, description: labelDesc }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["labels", owner, repo] });
      resetForm();
    },
    onError: (err: Error) => setError(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => del(`/repos/${owner}/${repo}/labels/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["labels", owner, repo] }),
    onError: (err: Error) => setError(err.message),
  });

  function resetForm() {
    setShowForm(false);
    setEditId(null);
    setLabelName("");
    setLabelColor(DEFAULT_COLORS[0]);
    setLabelDesc("");
    setError("");
  }

  function startEdit(label: Label) {
    setEditId(label.id);
    setLabelName(label.name);
    setLabelColor(label.color);
    setLabelDesc(label.description ?? "");
    setShowForm(true);
  }

  return (
    <div>
      <div className="settings-header">
        <h2>Labels</h2>
        {!showForm && (
          <button className="btn btn-primary" onClick={() => { resetForm(); setShowForm(true); }}>
            New label
          </button>
        )}
      </div>
      {error && <div className="error-banner">{error}</div>}

      {showForm && (
        <div className="settings-form-card">
          <h3>{editId ? "Edit label" : "New label"}</h3>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              editId ? updateMutation.mutate(editId) : createMutation.mutate();
            }}
          >
            <div className="form-group">
              <label htmlFor="label-name">Label name</label>
              <input
                id="label-name"
                type="text"
                className="form-input"
                value={labelName}
                onChange={(e) => setLabelName(e.target.value)}
                placeholder="bug"
                required
              />
            </div>
            <div className="form-group">
              <label>Color</label>
              <div className="color-picker">
                {DEFAULT_COLORS.map((c) => (
                  <button
                    key={c}
                    type="button"
                    className={`color-swatch ${labelColor === c ? "selected" : ""}`}
                    style={{ backgroundColor: c }}
                    onClick={() => setLabelColor(c)}
                  />
                ))}
                <input
                  type="color"
                  value={labelColor}
                  onChange={(e) => setLabelColor(e.target.value)}
                  className="color-input"
                />
              </div>
            </div>
            <div className="form-group">
              <label htmlFor="label-desc">Description</label>
              <input
                id="label-desc"
                type="text"
                className="form-input"
                value={labelDesc}
                onChange={(e) => setLabelDesc(e.target.value)}
                placeholder="Optional description"
              />
            </div>
            <div className="form-actions">
              <button type="submit" className="btn btn-primary" disabled={createMutation.isPending || updateMutation.isPending}>
                {editId ? "Save changes" : "Create label"}
              </button>
              <button type="button" className="btn btn-secondary" onClick={resetForm}>
                Cancel
              </button>
            </div>
          </form>
        </div>
      )}

      {labelsQuery.isLoading && <p className="muted">Loading labels...</p>}

      {labelsQuery.data && labelsQuery.data.length === 0 && !showForm && (
        <p className="muted">No labels yet.</p>
      )}

      {labelsQuery.data && labelsQuery.data.length > 0 && (
        <ul className="label-list">
          {labelsQuery.data.map((label) => (
            <li key={label.id} className="label-list-item">
              <div className="label-info">
                <span
                  className="label-color-dot"
                  style={{ backgroundColor: label.color }}
                />
                <strong>{label.name}</strong>
                {label.description && <span className="muted">{label.description}</span>}
              </div>
              <div className="label-actions">
                <button className="btn btn-sm btn-secondary" onClick={() => startEdit(label)}>
                  Edit
                </button>
                <button
                  className="btn btn-sm btn-danger"
                  onClick={() => deleteMutation.mutate(label.id)}
                >
                  Delete
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/* ---- Milestones Tab ---- */

function MilestonesTab({
  owner,
  repo,
  queryClient,
}: {
  owner: string;
  repo: string;
  queryClient: ReturnType<typeof useQueryClient>;
}) {
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [title, setTitle] = useState("");
  const [msDescription, setMsDescription] = useState("");
  const [dueDate, setDueDate] = useState("");
  const [error, setError] = useState("");

  const milestonesQuery = useQuery({
    queryKey: ["milestones", owner, repo],
    queryFn: () => get<Milestone[]>(`/repos/${owner}/${repo}/milestones`).then((r) => r.data),
  });

  const createMutation = useMutation({
    mutationFn: () =>
      post(`/repos/${owner}/${repo}/milestones`, {
        title,
        description: msDescription,
        due_date: dueDate || null,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["milestones", owner, repo] });
      resetForm();
    },
    onError: (err: Error) => setError(err.message),
  });

  const updateMutation = useMutation({
    mutationFn: (id: string) =>
      patch(`/repos/${owner}/${repo}/milestones/${id}`, {
        title,
        description: msDescription,
        due_date: dueDate || null,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["milestones", owner, repo] });
      resetForm();
    },
    onError: (err: Error) => setError(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => del(`/repos/${owner}/${repo}/milestones/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["milestones", owner, repo] }),
    onError: (err: Error) => setError(err.message),
  });

  function resetForm() {
    setShowForm(false);
    setEditId(null);
    setTitle("");
    setMsDescription("");
    setDueDate("");
    setError("");
  }

  function startEdit(ms: Milestone) {
    setEditId(ms.id);
    setTitle(ms.title);
    setMsDescription(ms.description ?? "");
    setDueDate(ms.due_date ? ms.due_date.slice(0, 10) : "");
    setShowForm(true);
  }

  return (
    <div>
      <div className="settings-header">
        <h2>Milestones</h2>
        {!showForm && (
          <button className="btn btn-primary" onClick={() => { resetForm(); setShowForm(true); }}>
            New milestone
          </button>
        )}
      </div>
      {error && <div className="error-banner">{error}</div>}

      {showForm && (
        <div className="settings-form-card">
          <h3>{editId ? "Edit milestone" : "New milestone"}</h3>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              editId ? updateMutation.mutate(editId) : createMutation.mutate();
            }}
          >
            <div className="form-group">
              <label htmlFor="ms-title">Title</label>
              <input
                id="ms-title"
                type="text"
                className="form-input"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder="v1.0"
                required
              />
            </div>
            <div className="form-group">
              <label htmlFor="ms-desc">Description</label>
              <textarea
                id="ms-desc"
                className="form-input"
                value={msDescription}
                onChange={(e) => setMsDescription(e.target.value)}
                placeholder="Optional description"
                rows={3}
              />
            </div>
            <div className="form-group">
              <label htmlFor="ms-due">Due date</label>
              <input
                id="ms-due"
                type="date"
                className="form-input"
                value={dueDate}
                onChange={(e) => setDueDate(e.target.value)}
              />
            </div>
            <div className="form-actions">
              <button type="submit" className="btn btn-primary" disabled={createMutation.isPending || updateMutation.isPending}>
                {editId ? "Save changes" : "Create milestone"}
              </button>
              <button type="button" className="btn btn-secondary" onClick={resetForm}>
                Cancel
              </button>
            </div>
          </form>
        </div>
      )}

      {milestonesQuery.isLoading && <p className="muted">Loading milestones...</p>}

      {milestonesQuery.data && milestonesQuery.data.length === 0 && !showForm && (
        <p className="muted">No milestones yet.</p>
      )}

      {milestonesQuery.data && milestonesQuery.data.length > 0 && (
        <ul className="milestone-list">
          {milestonesQuery.data.map((ms) => {
            const total = (ms.open_issues ?? 0) + (ms.closed_issues ?? 0);
            const pct = total > 0 ? Math.round(((ms.closed_issues ?? 0) / total) * 100) : 0;
            return (
              <li key={ms.id} className="milestone-item">
                <div className="milestone-info">
                  <div className="milestone-title-row">
                    <strong>{ms.title}</strong>
                    <span className={`badge badge-${ms.status === "closed" ? "closed" : "open"}`}>
                      {ms.status ?? "open"}
                    </span>
                  </div>
                  {ms.description && <p className="muted">{ms.description}</p>}
                  <div className="milestone-meta">
                    {ms.due_date && (
                      <span className="muted">
                        Due {new Date(ms.due_date).toLocaleDateString()}
                      </span>
                    )}
                    {total > 0 && (
                      <span className="muted">{pct}% complete</span>
                    )}
                  </div>
                  {total > 0 && (
                    <div className="milestone-progress">
                      <div className="milestone-progress-bar" style={{ width: `${pct}%` }} />
                    </div>
                  )}
                </div>
                <div className="milestone-actions">
                  <button className="btn btn-sm btn-secondary" onClick={() => startEdit(ms)}>
                    Edit
                  </button>
                  <button
                    className="btn btn-sm btn-danger"
                    onClick={() => deleteMutation.mutate(ms.id)}
                  >
                    Delete
                  </button>
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

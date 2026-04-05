import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { get, post, del } from "../lib/api";
import { useAuthStore } from "../stores/auth";

interface ApiToken {
  id: string;
  name: string;
  scopes: string[];
  expires_at: string | null;
  last_used: string | null;
  created_at: string;
}

interface CreateTokenResponse {
  id: string;
  name: string;
  token: string;
}

const AVAILABLE_SCOPES = [
  { value: "repo:read", label: "repo:read" },
  { value: "repo:write", label: "repo:write" },
  { value: "user:read", label: "user:read" },
  { value: "user:write", label: "user:write" },
  { value: "admin", label: "admin" },
];

function formatDate(dateStr: string | null): string {
  if (!dateStr) return "Never";
  return new Date(dateStr).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

export default function UserSettingsPage() {
  const [activeTab, setActiveTab] = useState<"tokens" | "account">("tokens");
  const navigate = useNavigate();

  return (
    <div className="settings-page">
      <nav className="settings-sidebar">
        <a href="/settings/profile" className="settings-tab settings-tab-link" onClick={(e) => { e.preventDefault(); navigate("/settings/profile"); }}>
          Profile
          <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor" style={{ marginLeft: 4, opacity: 0.5, flexShrink: 0 }}>
            <path d="M3.75 2a.75.75 0 0 0 0 1.5h6.69L2.72 11.22a.75.75 0 1 0 1.06 1.06L11.5 4.56v6.69a.75.75 0 0 0 1.5 0V2.75a.75.75 0 0 0-.75-.75H3.75Z" />
          </svg>
        </a>
        <button
          className={`settings-tab ${activeTab === "tokens" ? "active" : ""}`}
          onClick={() => setActiveTab("tokens")}
        >
          API Tokens
        </button>
        <button
          className={`settings-tab ${activeTab === "account" ? "active" : ""}`}
          onClick={() => setActiveTab("account")}
        >
          Account
        </button>
      </nav>

      <div className="settings-content">
        {activeTab === "tokens" && <TokensTab />}
        {activeTab === "account" && <AccountTab />}
      </div>
    </div>
  );
}

function TokensTab() {
  const queryClient = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [newTokenValue, setNewTokenValue] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);

  const [tokenName, setTokenName] = useState("");
  const [selectedScopes, setSelectedScopes] = useState<string[]>([]);
  const [expiresAt, setExpiresAt] = useState("");
  const [createError, setCreateError] = useState("");

  const { data: tokens, isLoading } = useQuery({
    queryKey: ["auth-tokens"],
    queryFn: async () => {
      const { data } = await get<ApiToken[]>("/auth/tokens");
      return data;
    },
  });

  const createMutation = useMutation({
    mutationFn: async () => {
      const body: Record<string, unknown> = {
        name: tokenName,
        scopes: selectedScopes,
      };
      if (expiresAt) {
        body.expires_at = new Date(expiresAt).toISOString();
      }
      const { data } = await post<CreateTokenResponse>("/auth/tokens", body);
      return data;
    },
    onSuccess: (data) => {
      setNewTokenValue(data.token);
      setShowCreate(false);
      setTokenName("");
      setSelectedScopes([]);
      setExpiresAt("");
      setCreateError("");
      queryClient.invalidateQueries({ queryKey: ["auth-tokens"] });
    },
    onError: (err: Error) => {
      setCreateError(err.message);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: string) => {
      await del(`/auth/tokens/${id}`);
    },
    onSuccess: () => {
      setDeleteConfirm(null);
      queryClient.invalidateQueries({ queryKey: ["auth-tokens"] });
    },
  });

  const handleScopeToggle = (scope: string) => {
    setSelectedScopes((prev) =>
      prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope],
    );
  };

  const handleCopy = async () => {
    if (!newTokenValue) return;
    await navigator.clipboard.writeText(newTokenValue);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleCreateSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!tokenName.trim()) {
      setCreateError("Token name is required");
      return;
    }
    if (selectedScopes.length === 0) {
      setCreateError("Select at least one scope");
      return;
    }
    setCreateError("");
    createMutation.mutate();
  };

  return (
    <div>
      <div className="settings-header">
        <h2>API Tokens</h2>
        <button className="btn btn-primary btn-sm" onClick={() => { setShowCreate(true); setNewTokenValue(null); }}>
          New token
        </button>
      </div>

      {newTokenValue && (
        <div className="token-warning">
          <strong>Copy your new token now.</strong> You won't be able to see it again.
          <div className="token-value">
            <code>{newTokenValue}</code>
            <button className="btn btn-secondary btn-sm" onClick={handleCopy}>
              {copied ? "Copied!" : "Copy"}
            </button>
          </div>
        </div>
      )}

      {showCreate && (
        <div className="settings-form-card">
          <h3>Create new token</h3>
          {createError && <div className="error-banner">{createError}</div>}
          <form onSubmit={handleCreateSubmit}>
            <div className="form-group">
              <label htmlFor="token-name">Token name</label>
              <input
                id="token-name"
                type="text"
                value={tokenName}
                onChange={(e) => setTokenName(e.target.value)}
                placeholder="My CI token"
              />
            </div>
            <div className="form-group">
              <label>Scopes</label>
              <div className="scope-checkboxes">
                {AVAILABLE_SCOPES.map((scope) => (
                  <label key={scope.value} className="checkbox-label">
                    <input
                      type="checkbox"
                      checked={selectedScopes.includes(scope.value)}
                      onChange={() => handleScopeToggle(scope.value)}
                    />
                    {scope.label}
                  </label>
                ))}
              </div>
            </div>
            <div className="form-group">
              <label htmlFor="token-expiry">Expiration (optional)</label>
              <input
                id="token-expiry"
                type="date"
                value={expiresAt}
                onChange={(e) => setExpiresAt(e.target.value)}
                style={{ width: "auto" }}
              />
            </div>
            <div style={{ display: "flex", gap: 8 }}>
              <button type="submit" className="btn btn-primary btn-sm" disabled={createMutation.isPending}>
                {createMutation.isPending ? "Creating..." : "Create token"}
              </button>
              <button type="button" className="btn btn-secondary btn-sm" onClick={() => { setShowCreate(false); setCreateError(""); }}>
                Cancel
              </button>
            </div>
          </form>
        </div>
      )}

      {isLoading ? (
        <p className="muted">Loading tokens...</p>
      ) : tokens && tokens.length > 0 ? (
        <div className="settings-form-card" style={{ padding: 0 }}>
          <table className="token-list">
            <thead>
              <tr>
                <th>Name</th>
                <th>Scopes</th>
                <th>Created</th>
                <th>Last used</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {tokens.map((token) => (
                <tr key={token.id}>
                  <td><strong>{token.name}</strong></td>
                  <td>
                    <span className="token-scopes">
                      {token.scopes?.join(", ") || "none"}
                    </span>
                  </td>
                  <td>{formatDate(token.created_at)}</td>
                  <td>{formatDate(token.last_used)}</td>
                  <td>
                    {deleteConfirm === token.id ? (
                      <span style={{ display: "flex", gap: 4 }}>
                        <button
                          className="btn btn-sm"
                          style={{ background: "var(--danger)", color: "#fff", borderColor: "var(--danger)" }}
                          onClick={() => deleteMutation.mutate(token.id)}
                          disabled={deleteMutation.isPending}
                        >
                          Confirm
                        </button>
                        <button
                          className="btn btn-secondary btn-sm"
                          onClick={() => setDeleteConfirm(null)}
                        >
                          Cancel
                        </button>
                      </span>
                    ) : (
                      <button
                        className="btn btn-secondary btn-sm"
                        onClick={() => setDeleteConfirm(token.id)}
                      >
                        Delete
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="settings-form-card">
          <p className="muted" style={{ margin: 0 }}>No API tokens yet. Create one to authenticate with the API.</p>
        </div>
      )}
    </div>
  );
}

function AccountTab() {
  const user = useAuthStore((s) => s.user);

  return (
    <div>
      <h2>Account</h2>
      <div className="settings-form-card">
        <div className="form-group">
          <label>Username</label>
          <input type="text" value={user?.username || ""} disabled />
        </div>
        <div className="form-group">
          <label>Email</label>
          <input type="text" value={user?.email || ""} disabled />
        </div>
        <div className="form-group" style={{ marginBottom: 0 }}>
          <label>Account created</label>
          <input type="text" value={user?.created_at ? formatDate(user.created_at) : ""} disabled />
        </div>
      </div>

      <div className="danger-zone">
        <h3>Danger zone</h3>
        <div className="danger-zone-item">
          <div>
            <strong>Delete account</strong>
            <p className="muted">
              Permanently delete your account and all associated data.
            </p>
          </div>
          <button className="btn btn-sm" style={{ background: "var(--danger)", color: "#fff", borderColor: "var(--danger)" }} disabled>
            Coming soon
          </button>
        </div>
      </div>
    </div>
  );
}

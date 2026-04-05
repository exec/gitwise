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
  const [activeTab, setActiveTab] = useState<"profile" | "tokens" | "account">("tokens");
  const navigate = useNavigate();

  if (activeTab === "profile") {
    navigate("/settings/profile");
  }

  return (
    <div className="settings-page">
      <h2>Settings</h2>
      <div className="tab-nav">
        <button
          className={`tab ${activeTab === "profile" ? "tab-active" : ""}`}
          onClick={() => setActiveTab("profile")}
        >
          Profile
        </button>
        <button
          className={`tab ${activeTab === "tokens" ? "tab-active" : ""}`}
          onClick={() => setActiveTab("tokens")}
        >
          API Tokens
        </button>
        <button
          className={`tab ${activeTab === "account" ? "tab-active" : ""}`}
          onClick={() => setActiveTab("account")}
        >
          Account
        </button>
      </div>

      {activeTab === "tokens" && <TokensTab />}
      {activeTab === "account" && <AccountTab />}
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
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 16 }}>
        <h3>API Tokens</h3>
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
        <div className="settings-card" style={{ marginBottom: 16 }}>
          <h4 style={{ marginBottom: 12 }}>Create new token</h4>
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
                <td>{token.name}</td>
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
      ) : (
        <p className="muted">No API tokens yet.</p>
      )}
    </div>
  );
}

function AccountTab() {
  const user = useAuthStore((s) => s.user);

  return (
    <div>
      <h3 style={{ marginBottom: 16 }}>Account</h3>
      <div className="settings-card">
        <div className="form-group">
          <label>Username</label>
          <input type="text" value={user?.username || ""} disabled />
        </div>
        <div className="form-group">
          <label>Email</label>
          <input type="text" value={user?.email || ""} disabled />
        </div>
        <div className="form-group">
          <label>Account created</label>
          <input type="text" value={user?.created_at ? formatDate(user.created_at) : ""} disabled />
        </div>
      </div>

      <div className="danger-zone" style={{ marginTop: 24 }}>
        <h4>Danger zone</h4>
        <div className="settings-card" style={{ borderColor: "var(--danger)" }}>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
            <div>
              <strong>Delete account</strong>
              <p className="muted" style={{ fontSize: "0.8125rem" }}>
                Permanently delete your account and all associated data.
              </p>
            </div>
            <button className="btn btn-sm" style={{ background: "var(--danger)", color: "#fff", borderColor: "var(--danger)" }} disabled>
              Coming soon
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

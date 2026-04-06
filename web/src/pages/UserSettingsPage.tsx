import { useState, useRef } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { get, post, put, del } from "../lib/api";
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

interface SSHKey {
  id: string;
  name: string;
  fingerprint: string;
  key_type: string;
  created_at: string;
}

interface NotificationPreferences {
  user_id: string;
  pr_review: boolean;
  pr_merged: boolean;
  pr_comment: boolean;
  issue_comment: boolean;
  mention: boolean;
  updated_at: string;
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
  const [activeTab, setActiveTab] = useState<"tokens" | "ssh-keys" | "2fa" | "notifications" | "account">("tokens");
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
          className={`settings-tab ${activeTab === "ssh-keys" ? "active" : ""}`}
          onClick={() => setActiveTab("ssh-keys")}
        >
          SSH Keys
        </button>
        <button
          className={`settings-tab ${activeTab === "2fa" ? "active" : ""}`}
          onClick={() => setActiveTab("2fa")}
        >
          Two-Factor Auth
        </button>
        <button
          className={`settings-tab ${activeTab === "notifications" ? "active" : ""}`}
          onClick={() => setActiveTab("notifications")}
        >
          Notifications
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
        {activeTab === "ssh-keys" && <SSHKeysTab />}
        {activeTab === "2fa" && <TwoFactorTab />}
        {activeTab === "notifications" && <NotificationsTab />}
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

function SSHKeysTab() {
  const queryClient = useQueryClient();
  const [showAdd, setShowAdd] = useState(false);
  const [keyTitle, setKeyTitle] = useState("");
  const [keyContent, setKeyContent] = useState("");
  const [addError, setAddError] = useState("");
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);

  const { data: keys, isLoading } = useQuery({
    queryKey: ["ssh-keys"],
    queryFn: async () => {
      const { data } = await get<SSHKey[]>("/user/ssh-keys");
      return data;
    },
  });

  const addMutation = useMutation({
    mutationFn: async () => {
      const { data } = await post<SSHKey>("/user/ssh-keys", {
        title: keyTitle,
        public_key: keyContent,
      });
      return data;
    },
    onSuccess: () => {
      setShowAdd(false);
      setKeyTitle("");
      setKeyContent("");
      setAddError("");
      queryClient.invalidateQueries({ queryKey: ["ssh-keys"] });
    },
    onError: (err: Error) => {
      setAddError(err.message);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: string) => {
      await del(`/user/ssh-keys/${id}`);
    },
    onSuccess: () => {
      setDeleteConfirm(null);
      queryClient.invalidateQueries({ queryKey: ["ssh-keys"] });
    },
  });

  const handleAddSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!keyTitle.trim()) {
      setAddError("Title is required");
      return;
    }
    if (!keyContent.trim()) {
      setAddError("Public key is required");
      return;
    }
    setAddError("");
    addMutation.mutate();
  };

  return (
    <div>
      <div className="settings-header">
        <h2>SSH Keys</h2>
        <button className="btn btn-primary btn-sm" onClick={() => setShowAdd(true)}>
          Add SSH key
        </button>
      </div>

      {showAdd && (
        <div className="settings-form-card">
          <h3>Add new SSH key</h3>
          {addError && <div className="error-banner">{addError}</div>}
          <form onSubmit={handleAddSubmit}>
            <div className="form-group">
              <label htmlFor="ssh-key-title">Title</label>
              <input
                id="ssh-key-title"
                type="text"
                value={keyTitle}
                onChange={(e) => setKeyTitle(e.target.value)}
                placeholder="My Laptop"
              />
            </div>
            <div className="form-group">
              <label htmlFor="ssh-key-content">Public key</label>
              <textarea
                id="ssh-key-content"
                value={keyContent}
                onChange={(e) => setKeyContent(e.target.value)}
                placeholder="ssh-ed25519 AAAA... user@host"
                rows={4}
                style={{ fontFamily: "var(--font-mono)", fontSize: "0.8125rem" }}
              />
            </div>
            <div style={{ display: "flex", gap: 8 }}>
              <button type="submit" className="btn btn-primary btn-sm" disabled={addMutation.isPending}>
                {addMutation.isPending ? "Adding..." : "Add key"}
              </button>
              <button type="button" className="btn btn-secondary btn-sm" onClick={() => { setShowAdd(false); setAddError(""); }}>
                Cancel
              </button>
            </div>
          </form>
        </div>
      )}

      {isLoading ? (
        <p className="muted">Loading SSH keys...</p>
      ) : keys && keys.length > 0 ? (
        <div className="settings-form-card" style={{ padding: 0 }}>
          <table className="token-list">
            <thead>
              <tr>
                <th>Title</th>
                <th>Fingerprint</th>
                <th>Type</th>
                <th>Added</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {keys.map((key) => (
                <tr key={key.id}>
                  <td><strong>{key.name}</strong></td>
                  <td>
                    <code style={{ fontSize: "0.8125rem" }}>{key.fingerprint}</code>
                  </td>
                  <td>{key.key_type}</td>
                  <td>{formatDate(key.created_at)}</td>
                  <td>
                    {deleteConfirm === key.id ? (
                      <span style={{ display: "flex", gap: 4 }}>
                        <button
                          className="btn btn-sm"
                          style={{ background: "var(--danger)", color: "#fff", borderColor: "var(--danger)" }}
                          onClick={() => deleteMutation.mutate(key.id)}
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
                        onClick={() => setDeleteConfirm(key.id)}
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
          <p className="muted" style={{ margin: 0 }}>No SSH keys yet. Add one to push and pull via SSH.</p>
        </div>
      )}
    </div>
  );
}

const NOTIFICATION_TYPES = [
  { key: "pr_review" as const, label: "Pull request reviews", description: "When someone reviews your pull request" },
  { key: "pr_merged" as const, label: "Pull request merged", description: "When a pull request you're involved in is merged" },
  { key: "pr_comment" as const, label: "Pull request comments", description: "When someone comments on your pull request" },
  { key: "issue_comment" as const, label: "Issue comments", description: "When someone comments on an issue you're involved in" },
  { key: "mention" as const, label: "Mentions", description: "When someone mentions you in a comment or description" },
];

function NotificationsTab() {
  const queryClient = useQueryClient();
  const [saveStatus, setSaveStatus] = useState<"idle" | "saving" | "saved">("idle");

  const { data: prefs, isLoading } = useQuery({
    queryKey: ["notification-preferences"],
    queryFn: async () => {
      const { data } = await get<NotificationPreferences>("/user/notification-preferences");
      return data;
    },
  });

  const updateMutation = useMutation({
    mutationFn: async (update: Partial<Record<string, boolean>>) => {
      const { data } = await put<NotificationPreferences>("/user/notification-preferences", update);
      return data;
    },
    onMutate: async (update) => {
      await queryClient.cancelQueries({ queryKey: ["notification-preferences"] });
      const previous = queryClient.getQueryData<NotificationPreferences>(["notification-preferences"]);
      if (previous) {
        queryClient.setQueryData<NotificationPreferences>(["notification-preferences"], {
          ...previous,
          ...update,
        });
      }
      return { previous };
    },
    onError: (_err, _vars, context) => {
      if (context?.previous) {
        queryClient.setQueryData(["notification-preferences"], context.previous);
      }
    },
    onSuccess: () => {
      setSaveStatus("saved");
      setTimeout(() => setSaveStatus("idle"), 2000);
      queryClient.invalidateQueries({ queryKey: ["notification-preferences"] });
    },
  });

  const handleToggle = (key: string, currentValue: boolean) => {
    setSaveStatus("saving");
    updateMutation.mutate({ [key]: !currentValue });
  };

  if (isLoading) {
    return (
      <div>
        <h2>Notifications</h2>
        <p className="muted">Loading preferences...</p>
      </div>
    );
  }

  return (
    <div>
      <div className="settings-header">
        <h2>Notifications</h2>
        {saveStatus === "saving" && <span className="muted">Saving...</span>}
        {saveStatus === "saved" && <span style={{ color: "var(--success)" }}>Saved</span>}
      </div>
      <p className="muted" style={{ marginBottom: 16 }}>
        Choose which notifications you'd like to receive.
      </p>

      <div className="settings-form-card">
        {NOTIFICATION_TYPES.map((type) => {
          const value = prefs ? prefs[type.key] : true;
          return (
            <div key={type.key} className="notification-pref-row">
              <div className="notification-pref-info">
                <strong>{type.label}</strong>
                <span className="muted">{type.description}</span>
              </div>
              <button
                className={`toggle-switch ${value ? "active" : ""}`}
                onClick={() => handleToggle(type.key, value)}
                role="switch"
                aria-checked={value}
                aria-label={type.label}
              >
                <span className="toggle-knob" />
              </button>
            </div>
          );
        })}
      </div>
    </div>
  );
}

interface TwoFactorStatus {
  enabled: boolean;
}

interface TwoFactorSetupData {
  secret: string;
  uri: string;
  qr_code: string;
  recovery_codes: string[];
}

function TwoFactorTab() {
  const queryClient = useQueryClient();
  const [setupData, setSetupData] = useState<TwoFactorSetupData | null>(null);
  const [setupPassword, setSetupPassword] = useState("");
  const [verifyCode, setVerifyCode] = useState("");
  const [disableCode, setDisableCode] = useState("");
  const [error, setError] = useState("");
  const [showRecoveryCodes, setShowRecoveryCodes] = useState(false);
  const [showDisable, setShowDisable] = useState(false);
  const [copiedCodes, setCopiedCodes] = useState(false);
  const recoveryRef = useRef<HTMLPreElement>(null);

  const { data: status, isLoading } = useQuery({
    queryKey: ["2fa-status"],
    queryFn: async () => {
      const { data } = await get<TwoFactorStatus>("/user/2fa/status");
      return data;
    },
  });

  const setupMutation = useMutation({
    mutationFn: async () => {
      const { data } = await post<TwoFactorSetupData>("/user/2fa/setup", {
        current_password: setupPassword,
      });
      return data;
    },
    onSuccess: (data) => {
      setSetupData(data);
      setSetupPassword("");
      setError("");
    },
    onError: (err: Error) => {
      setError(err.message);
    },
  });

  const enableMutation = useMutation({
    mutationFn: async () => {
      await post("/user/2fa/enable", { code: verifyCode });
    },
    onSuccess: () => {
      setVerifyCode("");
      setError("");
      setShowRecoveryCodes(true);
      queryClient.invalidateQueries({ queryKey: ["2fa-status"] });
    },
    onError: (err: Error) => {
      setError(err.message);
    },
  });

  const disableMutation = useMutation({
    mutationFn: async () => {
      await post("/user/2fa/disable", { code: disableCode });
    },
    onSuccess: () => {
      setDisableCode("");
      setShowDisable(false);
      setSetupData(null);
      setError("");
      queryClient.invalidateQueries({ queryKey: ["2fa-status"] });
    },
    onError: (err: Error) => {
      setError(err.message);
    },
  });

  const handleCopyRecoveryCodes = async () => {
    if (!setupData?.recovery_codes) return;
    await navigator.clipboard.writeText(setupData.recovery_codes.join("\n"));
    setCopiedCodes(true);
    setTimeout(() => setCopiedCodes(false), 2000);
  };

  if (isLoading) {
    return (
      <div>
        <h2>Two-Factor Authentication</h2>
        <p className="muted">Loading...</p>
      </div>
    );
  }

  const isEnabled = status?.enabled ?? false;

  // Show recovery codes after enabling
  if (showRecoveryCodes && setupData) {
    return (
      <div>
        <h2>Two-Factor Authentication</h2>
        <div className="settings-form-card">
          <h3>Save your recovery codes</h3>
          <p style={{ marginBottom: 12 }}>
            Store these recovery codes in a safe place. Each code can only be used once.
            If you lose access to your authenticator app, you can use these codes to sign in.
          </p>
          <pre
            ref={recoveryRef}
            style={{
              background: "var(--bg-secondary)",
              padding: 16,
              borderRadius: 6,
              fontFamily: "var(--font-mono)",
              fontSize: "0.875rem",
              lineHeight: 1.6,
              marginBottom: 12,
            }}
          >
            {setupData.recovery_codes.join("\n")}
          </pre>
          <div style={{ display: "flex", gap: 8 }}>
            <button className="btn btn-primary btn-sm" onClick={handleCopyRecoveryCodes}>
              {copiedCodes ? "Copied!" : "Copy codes"}
            </button>
            <button
              className="btn btn-secondary btn-sm"
              onClick={() => {
                setShowRecoveryCodes(false);
                setSetupData(null);
              }}
            >
              Done
            </button>
          </div>
        </div>
      </div>
    );
  }

  // 2FA is enabled - show status and disable option
  if (isEnabled) {
    return (
      <div>
        <h2>Two-Factor Authentication</h2>
        <div className="settings-form-card">
          <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 12 }}>
            <span style={{
              display: "inline-block",
              width: 10,
              height: 10,
              borderRadius: "50%",
              background: "var(--success, #2ea043)",
            }} />
            <strong>Two-factor authentication is enabled</strong>
          </div>
          <p className="muted" style={{ marginBottom: 16 }}>
            Your account is protected with TOTP-based two-factor authentication.
          </p>

          {showDisable ? (
            <>
              {error && <div className="error-banner">{error}</div>}
              <form onSubmit={(e) => { e.preventDefault(); disableMutation.mutate(); }}>
                <div className="form-group">
                  <label htmlFor="disable-code">Enter your TOTP code or a recovery code to disable 2FA</label>
                  <input
                    id="disable-code"
                    type="text"
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    value={disableCode}
                    onChange={(e) => setDisableCode(e.target.value)}
                    placeholder="123456"
                    autoFocus
                    style={{ fontFamily: "var(--font-mono)" }}
                  />
                </div>
                <div style={{ display: "flex", gap: 8 }}>
                  <button
                    type="submit"
                    className="btn btn-sm"
                    style={{ background: "var(--danger)", color: "#fff", borderColor: "var(--danger)" }}
                    disabled={disableMutation.isPending || !disableCode.trim()}
                  >
                    {disableMutation.isPending ? "Disabling..." : "Disable 2FA"}
                  </button>
                  <button
                    type="button"
                    className="btn btn-secondary btn-sm"
                    onClick={() => { setShowDisable(false); setError(""); }}
                  >
                    Cancel
                  </button>
                </div>
              </form>
            </>
          ) : (
            <button
              className="btn btn-sm"
              style={{ background: "var(--danger)", color: "#fff", borderColor: "var(--danger)" }}
              onClick={() => setShowDisable(true)}
            >
              Disable two-factor authentication
            </button>
          )}
        </div>
      </div>
    );
  }

  // 2FA not enabled - show setup flow
  return (
    <div>
      <h2>Two-Factor Authentication</h2>
      {!setupData ? (
        <div className="settings-form-card">
          <p style={{ marginBottom: 16 }}>
            Add an extra layer of security to your account by enabling two-factor authentication
            using a TOTP authenticator app (like Google Authenticator, Authy, or 1Password).
          </p>
          {error && <div className="error-banner" style={{ marginBottom: 12 }}>{error}</div>}
          <form onSubmit={(e) => { e.preventDefault(); setupMutation.mutate(); }}>
            <div className="form-group">
              <label htmlFor="setup-password">Confirm your password to begin setup</label>
              <input
                id="setup-password"
                type="password"
                autoComplete="current-password"
                value={setupPassword}
                onChange={(e) => setSetupPassword(e.target.value)}
                placeholder="Enter your current password"
                required
              />
            </div>
            <button
              type="submit"
              className="btn btn-primary btn-sm"
              disabled={setupMutation.isPending || !setupPassword.trim()}
            >
              {setupMutation.isPending ? "Setting up..." : "Set up two-factor authentication"}
            </button>
          </form>
        </div>
      ) : (
        <div className="settings-form-card">
          <h3>Scan QR code</h3>
          <p style={{ marginBottom: 16 }}>
            Scan this QR code with your authenticator app, then enter the 6-digit code below to verify.
          </p>
          <div style={{ textAlign: "center", marginBottom: 16 }}>
            <img
              src={setupData.qr_code}
              alt="TOTP QR Code"
              style={{ width: 256, height: 256, imageRendering: "pixelated" }}
            />
          </div>
          <details style={{ marginBottom: 16 }}>
            <summary style={{ cursor: "pointer", color: "var(--fg-muted)", fontSize: "0.875rem" }}>
              Can't scan? Enter this key manually
            </summary>
            <code style={{
              display: "block",
              marginTop: 8,
              padding: 12,
              background: "var(--bg-secondary)",
              borderRadius: 6,
              fontFamily: "var(--font-mono)",
              fontSize: "0.875rem",
              wordBreak: "break-all",
            }}>
              {setupData.secret}
            </code>
          </details>

          {error && <div className="error-banner">{error}</div>}
          <form onSubmit={(e) => { e.preventDefault(); enableMutation.mutate(); }}>
            <div className="form-group">
              <label htmlFor="verify-code">Verification code</label>
              <input
                id="verify-code"
                type="text"
                inputMode="numeric"
                autoComplete="one-time-code"
                value={verifyCode}
                onChange={(e) => setVerifyCode(e.target.value)}
                placeholder="123456"
                maxLength={6}
                autoFocus
                style={{ fontFamily: "var(--font-mono)", fontSize: "1.125rem", letterSpacing: "0.1em" }}
              />
            </div>
            <div style={{ display: "flex", gap: 8 }}>
              <button
                type="submit"
                className="btn btn-primary btn-sm"
                disabled={enableMutation.isPending || verifyCode.length < 6}
              >
                {enableMutation.isPending ? "Verifying..." : "Enable 2FA"}
              </button>
              <button
                type="button"
                className="btn btn-secondary btn-sm"
                onClick={() => { setSetupData(null); setError(""); }}
              >
                Cancel
              </button>
            </div>
          </form>
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

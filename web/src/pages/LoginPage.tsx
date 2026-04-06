import { useState, useEffect, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useAuthStore } from "../stores/auth";
import { get, ApiError } from "../lib/api";

function GitHubIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 16 16" fill="currentColor">
      <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z" />
    </svg>
  );
}

export default function LoginPage() {
  const login = useAuthStore((s) => s.login);
  const navigate = useNavigate();

  const [loginField, setLoginField] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [providers, setProviders] = useState<string[]>([]);

  useEffect(() => {
    get<string[]>("/auth/providers")
      .then(({ data }) => setProviders(data ?? []))
      .catch(() => {});
  }, []);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      await login(loginField, password);
      navigate("/");
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("An unexpected error occurred");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="auth-page">
      <div className="auth-card">
        <h1>Sign in to Gitwise</h1>
        {error && <div className="error-banner">{error}</div>}
        {providers.includes("github") && (
          <>
            <a href="/api/v1/auth/github" className="btn btn-github btn-block">
              <GitHubIcon /> Sign in with GitHub
            </a>
            <div className="auth-divider">
              <span>or</span>
            </div>
          </>
        )}
        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label htmlFor="login">Username or email</label>
            <input
              id="login"
              type="text"
              value={loginField}
              onChange={(e) => setLoginField(e.target.value)}
              required
              autoFocus
              autoComplete="username"
            />
          </div>
          <div className="form-group">
            <label htmlFor="password">Password</label>
            <input
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              autoComplete="current-password"
            />
          </div>
          <button type="submit" className="btn btn-primary btn-block" disabled={submitting}>
            {submitting ? "Signing in..." : "Sign in"}
          </button>
        </form>
        <p className="auth-footer">
          New to Gitwise? <Link to="/register">Create an account</Link>
        </p>
      </div>
    </div>
  );
}

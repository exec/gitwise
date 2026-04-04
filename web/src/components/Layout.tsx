import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "../stores/auth";
import NotificationBell from "./NotificationBell";

export default function Layout({ children }: { children: React.ReactNode }) {
  const user = useAuthStore((s) => s.user);
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const logout = useAuthStore((s) => s.logout);
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [searchValue, setSearchValue] = useState("");

  const handleLogout = async () => {
    try {
      await logout();
    } catch {
      // Clear local state even if server call fails
      useAuthStore.setState({ user: null, isAuthenticated: false });
    }
    queryClient.clear();
    navigate("/login");
  };

  const handleSearchSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const q = searchValue.trim();
    if (q) {
      navigate(`/search?q=${encodeURIComponent(q)}`);
    }
  };

  return (
    <div className="app">
      <nav className="top-nav">
        <div className="nav-left">
          <Link to="/" className="logo">
            Gitwise
          </Link>
        </div>
        <form className="search-bar" onSubmit={handleSearchSubmit}>
          <svg className="search-bar-icon" width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
            <path fillRule="evenodd" d="M11.5 7a4.5 4.5 0 1 1-9 0 4.5 4.5 0 0 1 9 0Zm-.82 4.74a6 6 0 1 1 1.06-1.06l3.04 3.04a.75.75 0 1 1-1.06 1.06l-3.04-3.04Z" />
          </svg>
          <input
            type="text"
            className="search-bar-input"
            placeholder="Search repos, issues, code..."
            value={searchValue}
            onChange={(e) => setSearchValue(e.target.value)}
          />
        </form>
        <div className="nav-right">
          {isAuthenticated && user ? (
            <div className="user-menu">
              <NotificationBell />
              <span className="username">{user.username}</span>
              <button className="btn btn-secondary btn-sm" onClick={handleLogout}>
                Sign out
              </button>
            </div>
          ) : (
            <div className="auth-links">
              <Link to="/login" className="btn btn-secondary btn-sm">
                Sign in
              </Link>
              <Link to="/register" className="btn btn-primary btn-sm">
                Sign up
              </Link>
            </div>
          )}
        </div>
      </nav>
      <main className="main-content">{children}</main>
    </div>
  );
}

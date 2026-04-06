import { useState, useRef, useEffect, useCallback } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { get } from "../lib/api";
import { useAuthStore } from "../stores/auth";
import { useThemeStore } from "../stores/theme";
import { useKeyboardShortcuts } from "../hooks/useKeyboardShortcuts";
import NotificationBell from "./NotificationBell";
import KeyboardShortcutsHelp from "./KeyboardShortcutsHelp";

interface OrgMembership {
  id: string;
  name: string;
  display_name: string;
  avatar_url: string;
  role: string;
}

export default function Layout({ children }: { children: React.ReactNode }) {
  const user = useAuthStore((s) => s.user);
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const logout = useAuthStore((s) => s.logout);
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const theme = useThemeStore((s) => s.theme);
  const toggleTheme = useThemeStore((s) => s.toggle);
  const [searchValue, setSearchValue] = useState("");
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);

  const handleToggleHelp = useCallback(() => {
    setShortcutsOpen((v) => !v);
  }, []);

  const handleFocusSearch = useCallback(() => {
    searchInputRef.current?.focus();
  }, []);

  const handleCloseShortcuts = useCallback(() => {
    setShortcutsOpen(false);
  }, []);

  useKeyboardShortcuts({
    onToggleHelp: handleToggleHelp,
    onFocusSearch: handleFocusSearch,
  });

  const orgsQuery = useQuery({
    queryKey: ["my-orgs"],
    queryFn: () => get<OrgMembership[]>("/user/orgs").then((r) => r.data),
    enabled: isAuthenticated,
  });

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setDropdownOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  const handleLogout = async () => {
    setDropdownOpen(false);
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
            ref={searchInputRef}
            type="text"
            className="search-bar-input"
            placeholder="Search repos, issues, code..."
            value={searchValue}
            onChange={(e) => setSearchValue(e.target.value)}
          />
        </form>
        <div className="nav-right">
          <button
            className="theme-toggle"
            onClick={toggleTheme}
            aria-label={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
            title={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
          >
            {theme === "dark" ? (
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="12" r="5" />
                <line x1="12" y1="1" x2="12" y2="3" />
                <line x1="12" y1="21" x2="12" y2="23" />
                <line x1="4.22" y1="4.22" x2="5.64" y2="5.64" />
                <line x1="18.36" y1="18.36" x2="19.78" y2="19.78" />
                <line x1="1" y1="12" x2="3" y2="12" />
                <line x1="21" y1="12" x2="23" y2="12" />
                <line x1="4.22" y1="19.78" x2="5.64" y2="18.36" />
                <line x1="18.36" y1="5.64" x2="19.78" y2="4.22" />
              </svg>
            ) : (
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
              </svg>
            )}
          </button>
          {isAuthenticated && user ? (
            <div className="user-menu">
              <NotificationBell />
              <div className="user-dropdown-wrapper" ref={dropdownRef}>
                <button
                  className="username user-dropdown-trigger"
                  onClick={() => setDropdownOpen((v) => !v)}
                >
                  {user.username}
                </button>
                {dropdownOpen && (
                  <div className="user-dropdown">
                    <Link to={`/${user.username}`} className="user-dropdown-item" onClick={() => setDropdownOpen(false)}>
                      Your profile
                    </Link>
                    <Link to="/settings" className="user-dropdown-item" onClick={() => setDropdownOpen(false)}>
                      Settings
                    </Link>
                    <Link to="/" className="user-dropdown-item" onClick={() => setDropdownOpen(false)}>
                      Your repos
                    </Link>
                    <div className="user-dropdown-divider" />
                    <Link to="/new" className="user-dropdown-item" onClick={() => setDropdownOpen(false)}>
                      New repository
                    </Link>
                    <Link to="/new/org" className="user-dropdown-item" onClick={() => setDropdownOpen(false)}>
                      New organization
                    </Link>
                    {orgsQuery.data && orgsQuery.data.length > 0 && (
                      <>
                        <div className="user-dropdown-divider" />
                        <div className="user-dropdown-label">Your organizations</div>
                        {orgsQuery.data.map((org) => (
                          <Link
                            key={org.id}
                            to={`/${org.name}`}
                            className="user-dropdown-item"
                            onClick={() => setDropdownOpen(false)}
                          >
                            {org.display_name || org.name}
                          </Link>
                        ))}
                      </>
                    )}
                    {user.is_admin && (
                      <Link to="/admin-8bc6d1f" className="user-dropdown-item" onClick={() => setDropdownOpen(false)}>
                        Admin
                      </Link>
                    )}
                    <div className="user-dropdown-divider" />
                    <button className="user-dropdown-item user-dropdown-signout" onClick={handleLogout}>
                      Sign out
                    </button>
                  </div>
                )}
              </div>
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
      <KeyboardShortcutsHelp
        isOpen={shortcutsOpen}
        onClose={handleCloseShortcuts}
      />
    </div>
  );
}

import { Link, useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "../stores/auth";

export default function Layout({ children }: { children: React.ReactNode }) {
  const user = useAuthStore((s) => s.user);
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const logout = useAuthStore((s) => s.logout);
  const navigate = useNavigate();
  const queryClient = useQueryClient();

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

  return (
    <div className="app">
      <nav className="top-nav">
        <div className="nav-left">
          <Link to="/" className="logo">
            Gitwise
          </Link>
        </div>
        <div className="nav-right">
          {isAuthenticated && user ? (
            <div className="user-menu">
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

import { Routes, Route, Navigate, useParams } from "react-router-dom";
import { useAuthStore } from "./stores/auth";
import Layout from "./components/Layout";
import LandingPage from "./pages/LandingPage";
import LoginPage from "./pages/LoginPage";
import RegisterPage from "./pages/RegisterPage";
import DashboardPage from "./pages/DashboardPage";
import NewRepoPage from "./pages/NewRepoPage";
import NewOrgPage from "./pages/NewOrgPage";
import RepoPage from "./pages/RepoPage";
import IssueListPage from "./pages/IssueListPage";
import IssueDetailPage from "./pages/IssueDetailPage";
import NewIssuePage from "./pages/NewIssuePage";
import PullListPage from "./pages/PullListPage";
import PullDetailPage from "./pages/PullDetailPage";
import NewPullPage from "./pages/NewPullPage";
import SearchPage from "./pages/SearchPage";
import EditProfilePage from "./pages/EditProfilePage";
import OwnerPage from "./pages/OwnerPage";
import UserSettingsPage from "./pages/UserSettingsPage";
import RepoSettingsPage from "./pages/RepoSettingsPage";
import AdminPage from "./pages/AdminPage";
import ImportPage from "./pages/ImportPage";
import NotFoundPage from "./pages/NotFoundPage";

function RequireAuth({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}

function RequireAdminGuard({ children }: { children: React.ReactNode }) {
  const user = useAuthStore((s) => s.user);
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  if (!isAuthenticated || !user?.is_admin) {
    return <Navigate to="/" replace />;
  }
  return <>{children}</>;
}

function HomePage() {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  return isAuthenticated ? <DashboardPage /> : <LandingPage />;
}

function OrgRedirect() {
  const { name } = useParams();
  return <Navigate to={`/${name}`} replace />;
}

export default function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route path="/" element={<HomePage />} />
        <Route
          path="/new"
          element={
            <RequireAuth>
              <NewRepoPage />
            </RequireAuth>
          }
        />
        <Route
          path="/new/org"
          element={
            <RequireAuth>
              <NewOrgPage />
            </RequireAuth>
          }
        />
        <Route
          path="/new/import"
          element={
            <RequireAuth>
              <ImportPage />
            </RequireAuth>
          }
        />

        {/* Admin panel (secret path, requires admin) */}
        <Route
          path="/admin-8bc6d1f"
          element={
            <RequireAdminGuard>
              <AdminPage />
            </RequireAdminGuard>
          }
        />

        {/* Legacy org route redirect */}
        <Route path="/orgs/:name" element={<OrgRedirect />} />

        {/* Search */}
        <Route path="/search" element={<SearchPage />} />

        {/* Settings */}
        <Route
          path="/settings"
          element={
            <RequireAuth>
              <UserSettingsPage />
            </RequireAuth>
          }
        />
        <Route
          path="/settings/profile"
          element={
            <RequireAuth>
              <EditProfilePage />
            </RequireAuth>
          }
        />

        {/* Repo settings (before catch-all repo route) */}
        <Route
          path="/:owner/:repo/settings"
          element={
            <RequireAuth>
              <RepoSettingsPage />
            </RequireAuth>
          }
        />

        {/* Repo pages */}
        <Route path="/:owner/:repo" element={<RepoPage />} />

        {/* Shared /:owner namespace — resolves to ProfilePage or OrgPage */}
        <Route path="/:owner" element={<OwnerPage />} />
        <Route path="/:owner/:repo/tree/:ref/*" element={<RepoPage />} />
        <Route path="/:owner/:repo/blob/:ref/*" element={<RepoPage />} />
        <Route path="/:owner/:repo/blame/:ref/*" element={<RepoPage />} />
        <Route path="/:owner/:repo/commits" element={<RepoPage />} />

        {/* Issues */}
        <Route path="/:owner/:repo/issues" element={<IssueListPage />} />
        <Route
          path="/:owner/:repo/issues/new"
          element={
            <RequireAuth>
              <NewIssuePage />
            </RequireAuth>
          }
        />
        <Route
          path="/:owner/:repo/issues/:number"
          element={<IssueDetailPage />}
        />

        {/* Pull Requests */}
        <Route path="/:owner/:repo/pulls" element={<PullListPage />} />
        <Route
          path="/:owner/:repo/pulls/new"
          element={
            <RequireAuth>
              <NewPullPage />
            </RequireAuth>
          }
        />
        <Route
          path="/:owner/:repo/pulls/:number"
          element={<PullDetailPage />}
        />

        {/* 404 catch-all */}
        <Route path="*" element={<NotFoundPage />} />
      </Routes>
    </Layout>
  );
}

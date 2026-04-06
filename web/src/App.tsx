import { Routes, Route, Navigate } from "react-router-dom";
import { useAuthStore } from "./stores/auth";
import Layout from "./components/Layout";
import LandingPage from "./pages/LandingPage";
import LoginPage from "./pages/LoginPage";
import RegisterPage from "./pages/RegisterPage";
import DashboardPage from "./pages/DashboardPage";
import NewRepoPage from "./pages/NewRepoPage";
import RepoPage from "./pages/RepoPage";
import IssueListPage from "./pages/IssueListPage";
import IssueDetailPage from "./pages/IssueDetailPage";
import NewIssuePage from "./pages/NewIssuePage";
import PullListPage from "./pages/PullListPage";
import PullDetailPage from "./pages/PullDetailPage";
import NewPullPage from "./pages/NewPullPage";
import SearchPage from "./pages/SearchPage";
import ProfilePage from "./pages/ProfilePage";
import EditProfilePage from "./pages/EditProfilePage";
import OrgPage from "./pages/OrgPage";
import UserSettingsPage from "./pages/UserSettingsPage";
import RepoSettingsPage from "./pages/RepoSettingsPage";
import NotFoundPage from "./pages/NotFoundPage";

function RequireAuth({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}

function HomePage() {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  return isAuthenticated ? <DashboardPage /> : <LandingPage />;
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

        {/* Organizations */}
        <Route path="/orgs/:name" element={<OrgPage />} />

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

        {/* Profile (must be after /:owner/:repo so two-segment paths match first) */}
        <Route path="/:username" element={<ProfilePage />} />
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

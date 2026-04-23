import { Suspense, lazy } from "react";
import { Routes, Route, Navigate, useParams } from "react-router-dom";
import { useAuthStore } from "./stores/auth";
// Layout and auth guards are eagerly imported (tiny, needed on every render)
import Layout from "./components/Layout";

// Page chunks — loaded only when their route is visited
const LandingPage = lazy(() => import("./pages/LandingPage"));
const LoginPage = lazy(() => import("./pages/LoginPage"));
const RegisterPage = lazy(() => import("./pages/RegisterPage"));
const DashboardPage = lazy(() => import("./pages/DashboardPage"));
const NewRepoPage = lazy(() => import("./pages/NewRepoPage"));
const NewOrgPage = lazy(() => import("./pages/NewOrgPage"));
const RepoPage = lazy(() => import("./pages/RepoPage"));
const IssueListPage = lazy(() => import("./pages/IssueListPage"));
const IssueDetailPage = lazy(() => import("./pages/IssueDetailPage"));
const NewIssuePage = lazy(() => import("./pages/NewIssuePage"));
const PullListPage = lazy(() => import("./pages/PullListPage"));
const PullDetailPage = lazy(() => import("./pages/PullDetailPage"));
const NewPullPage = lazy(() => import("./pages/NewPullPage"));
const SearchPage = lazy(() => import("./pages/SearchPage"));
const EditProfilePage = lazy(() => import("./pages/EditProfilePage"));
const OwnerPage = lazy(() => import("./pages/OwnerPage"));
const UserSettingsPage = lazy(() => import("./pages/UserSettingsPage"));
const RepoSettingsPage = lazy(() => import("./pages/RepoSettingsPage"));
const AdminPage = lazy(() => import("./pages/AdminPage"));
const ImportPage = lazy(() => import("./pages/ImportPage"));
const AgentsTab = lazy(() => import("./pages/AgentsTab"));
const NotFoundPage = lazy(() => import("./pages/NotFoundPage"));

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
      <Suspense fallback={<div style={{ padding: "2rem", textAlign: "center" }}>Loading...</div>}>
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

        {/* Agents tab */}
        <Route path="/:owner/:repo/agents" element={<AgentsTab />} />

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
      </Suspense>
    </Layout>
  );
}

import { Routes, Route, Navigate } from "react-router-dom";
import { useAuthStore } from "./stores/auth";
import Layout from "./components/Layout";
import LoginPage from "./pages/LoginPage";
import RegisterPage from "./pages/RegisterPage";
import DashboardPage from "./pages/DashboardPage";
import NewRepoPage from "./pages/NewRepoPage";
import RepoPage from "./pages/RepoPage";

function RequireAuth({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}

export default function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route
          path="/"
          element={
            <RequireAuth>
              <DashboardPage />
            </RequireAuth>
          }
        />
        <Route
          path="/new"
          element={
            <RequireAuth>
              <NewRepoPage />
            </RequireAuth>
          }
        />
        <Route path="/:owner/:repo" element={<RepoPage />} />
        <Route path="/:owner/:repo/tree/:ref/*" element={<RepoPage />} />
        <Route path="/:owner/:repo/blob/:ref/*" element={<RepoPage />} />
        <Route path="/:owner/:repo/commits" element={<RepoPage />} />
      </Routes>
    </Layout>
  );
}

import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { get } from "../lib/api";
import ProfilePage from "./ProfilePage";
import OrgPage from "./OrgPage";
import NotFoundPage from "./NotFoundPage";

interface NamespaceResult {
  type: "user" | "org";
  data: unknown;
}

export default function OwnerPage() {
  const { owner } = useParams();

  const resolveQuery = useQuery({
    queryKey: ["resolve", owner],
    queryFn: () =>
      get<NamespaceResult>(`/resolve/${owner}`).then((r) => r.data),
    enabled: !!owner,
    retry: false,
  });

  if (resolveQuery.isLoading) {
    return <p className="muted">Loading...</p>;
  }

  if (resolveQuery.error || !resolveQuery.data) {
    return <NotFoundPage />;
  }

  if (resolveQuery.data.type === "user") {
    return <ProfilePage />;
  }

  if (resolveQuery.data.type === "org") {
    return <OrgPage />;
  }

  return <NotFoundPage />;
}

import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation, useQuery } from "@tanstack/react-query";
import { post, get, ApiError } from "../lib/api";
import { useAuthStore } from "../stores/auth";

interface CreateRepoPayload {
  name: string;
  description: string;
  visibility: string;
  default_branch: string;
  auto_init: boolean;
  org_name?: string;
}

interface Repo {
  id: string;
  owner_name: string;
  name: string;
}

interface OrgMembership {
  id: string;
  name: string;
  display_name: string;
  avatar_url: string;
  role: string;
}

export default function NewRepoPage() {
  const user = useAuthStore((s) => s.user);
  const navigate = useNavigate();

  const [owner, setOwner] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [visibility, setVisibility] = useState("private");
  const [autoInit, setAutoInit] = useState(true);
  const [error, setError] = useState("");

  const orgsQuery = useQuery({
    queryKey: ["user-orgs"],
    queryFn: () =>
      get<OrgMembership[]>("/user/orgs").then((r) => r.data),
    enabled: !!user,
  });

  const mutation = useMutation({
    mutationFn: (payload: CreateRepoPayload) =>
      post<Repo>("/repos", payload).then((r) => r.data),
    onSuccess: (repo) => {
      navigate(`/${repo.owner_name}/${repo.name}`);
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("An unexpected error occurred");
      }
    },
  });

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    setError("");
    const payload: CreateRepoPayload = {
      name,
      description,
      visibility,
      default_branch: "main",
      auto_init: autoInit,
    };
    if (owner && owner !== user?.username) {
      payload.org_name = owner;
    }
    mutation.mutate(payload);
  };

  return (
    <div className="new-repo-page">
      <h1>Create a new repository</h1>
      {error && <div className="error-banner">{error}</div>}
      <form onSubmit={handleSubmit}>
        <div className="form-group">
          <label htmlFor="owner">Owner</label>
          <select
            id="owner"
            value={owner}
            onChange={(e) => setOwner(e.target.value)}
          >
            <option value="">{user?.username ?? ""}</option>
            {orgsQuery.data?.map((o) => (
              <option key={o.id} value={o.name}>
                {o.display_name || o.name}
              </option>
            ))}
          </select>
        </div>
        <div className="form-group">
          <label htmlFor="name">Repository name</label>
          <input
            id="name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            autoFocus
            pattern="[a-zA-Z0-9._-]+"
            title="Letters, numbers, hyphens, dots, and underscores only"
          />
        </div>
        <div className="form-group">
          <label htmlFor="description">Description (optional)</label>
          <input
            id="description"
            type="text"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </div>
        <div className="form-group">
          <label>Visibility</label>
          <div className="radio-group">
            <label className="radio-label">
              <input
                type="radio"
                name="visibility"
                value="public"
                checked={visibility === "public"}
                onChange={(e) => setVisibility(e.target.value)}
              />
              Public
            </label>
            <label className="radio-label">
              <input
                type="radio"
                name="visibility"
                value="private"
                checked={visibility === "private"}
                onChange={(e) => setVisibility(e.target.value)}
              />
              Private
            </label>
          </div>
        </div>
        <div className="form-group">
          <label className="checkbox-label">
            <input
              type="checkbox"
              checked={autoInit}
              onChange={(e) => setAutoInit(e.target.checked)}
            />
            Initialize this repository with a README
          </label>
        </div>
        <button
          type="submit"
          className="btn btn-primary"
          disabled={mutation.isPending}
        >
          {mutation.isPending ? "Creating..." : "Create repository"}
        </button>
      </form>
    </div>
  );
}

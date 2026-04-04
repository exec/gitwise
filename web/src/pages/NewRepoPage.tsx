import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation } from "@tanstack/react-query";
import { post, ApiError } from "../lib/api";
import { useAuthStore } from "../stores/auth";

interface CreateRepoPayload {
  name: string;
  description: string;
  visibility: string;
  default_branch: string;
  auto_init: boolean;
}

interface Repo {
  id: string;
  owner_name: string;
  name: string;
}

export default function NewRepoPage() {
  const user = useAuthStore((s) => s.user);
  const navigate = useNavigate();

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [visibility, setVisibility] = useState("private");
  const [autoInit, setAutoInit] = useState(true);
  const [error, setError] = useState("");

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
    mutation.mutate({
      name,
      description,
      visibility,
      default_branch: "main",
      auto_init: autoInit,
    });
  };

  return (
    <div className="new-repo-page">
      <h1>Create a new repository</h1>
      {error && <div className="error-banner">{error}</div>}
      <form onSubmit={handleSubmit}>
        <div className="form-group">
          <label htmlFor="owner">Owner</label>
          <input id="owner" type="text" value={user?.username ?? ""} disabled />
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

import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation } from "@tanstack/react-query";
import { post, ApiError } from "../lib/api";

interface Organization {
  id: string;
  name: string;
  display_name: string;
}

export default function NewOrgPage() {
  const navigate = useNavigate();

  const [name, setName] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [description, setDescription] = useState("");
  const [error, setError] = useState("");

  const mutation = useMutation({
    mutationFn: () =>
      post<Organization>("/orgs", { name, display_name: displayName, description }).then(
        (r) => r.data
      ),
    onSuccess: (org) => {
      navigate(`/${org.name}`);
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
    mutation.mutate();
  };

  return (
    <div className="new-repo-page">
      <h1>Create a new organization</h1>
      <p className="muted">
        Organizations let you collaborate with others on repositories.
      </p>
      {error && <div className="error-banner">{error}</div>}
      <form onSubmit={handleSubmit}>
        <div className="form-group">
          <label htmlFor="org-name">Organization name</label>
          <input
            id="org-name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            autoFocus
            pattern="[a-zA-Z0-9][a-zA-Z0-9._-]*[a-zA-Z0-9]?"
            title="Letters, numbers, hyphens, dots, and underscores. Must start and end with alphanumeric."
            placeholder="my-org"
          />
          <p className="form-hint">
            This will be your organization's URL: /{name || "..."}
          </p>
        </div>
        <div className="form-group">
          <label htmlFor="org-display-name">Display name (optional)</label>
          <input
            id="org-display-name"
            type="text"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            placeholder="My Organization"
          />
        </div>
        <div className="form-group">
          <label htmlFor="org-description">Description (optional)</label>
          <textarea
            id="org-description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={3}
            placeholder="What does this organization do?"
          />
        </div>
        <button
          type="submit"
          className="btn btn-primary"
          disabled={mutation.isPending || !name.trim()}
        >
          {mutation.isPending ? "Creating..." : "Create organization"}
        </button>
      </form>
    </div>
  );
}

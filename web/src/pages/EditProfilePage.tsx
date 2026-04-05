import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { patch, get } from "../lib/api";
import { useAuthStore } from "../stores/auth";
import type { User } from "../stores/auth";

interface ProfileForm {
  full_name: string;
  bio: string;
  avatar_url: string;
}

export default function EditProfilePage() {
  const user = useAuthStore((s) => s.user);
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const [form, setForm] = useState<ProfileForm>({
    full_name: "",
    bio: "",
    avatar_url: "",
  });
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!user) return;
    get<{ full_name: string; bio: string; avatar_url: string }>(
      `/users/${user.username}`,
    ).then(({ data }) => {
      setForm({
        full_name: data.full_name || "",
        bio: data.bio || "",
        avatar_url: data.avatar_url || "",
      });
      setLoading(false);
    }).catch(() => {
      setForm({
        full_name: user.full_name || "",
        bio: "",
        avatar_url: "",
      });
      setLoading(false);
    });
  }, [user]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setSubmitting(true);

    try {
      await patch<User>("/user/profile", {
        full_name: form.full_name,
        bio: form.bio,
        avatar_url: form.avatar_url,
      });
      queryClient.invalidateQueries({ queryKey: ["profile", user?.username] });
      navigate(`/${user?.username}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to update profile");
      setSubmitting(false);
    }
  };

  if (loading) {
    return <p className="muted">Loading profile...</p>;
  }

  return (
    <div className="form-page">
      <h2>Edit profile</h2>

      {error && <div className="error-banner">{error}</div>}

      <form onSubmit={handleSubmit}>
        <div className="form-group">
          <label htmlFor="full_name">Name</label>
          <input
            id="full_name"
            type="text"
            value={form.full_name}
            onChange={(e) =>
              setForm((f) => ({ ...f, full_name: e.target.value }))
            }
            placeholder="Your display name"
          />
        </div>

        <div className="form-group">
          <label htmlFor="bio">Bio</label>
          <textarea
            id="bio"
            value={form.bio}
            onChange={(e) =>
              setForm((f) => ({ ...f, bio: e.target.value }))
            }
            placeholder="Tell us about yourself"
            rows={4}
          />
        </div>

        <div className="form-group">
          <label htmlFor="avatar_url">Avatar URL</label>
          <input
            id="avatar_url"
            type="text"
            value={form.avatar_url}
            onChange={(e) =>
              setForm((f) => ({ ...f, avatar_url: e.target.value }))
            }
            placeholder="https://example.com/avatar.png"
          />
        </div>

        <button
          type="submit"
          className="btn btn-primary"
          disabled={submitting}
        >
          {submitting ? "Saving..." : "Save profile"}
        </button>
      </form>
    </div>
  );
}

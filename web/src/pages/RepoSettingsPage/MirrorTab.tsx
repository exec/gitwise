import { useState, useEffect, type FormEvent } from "react";
import { useQuery, useMutation, type QueryClient } from "@tanstack/react-query";
import { get, put, post, del } from "../../lib/api";
import { MirrorStatusDot, type MirrorStatus, type MirrorDirection } from "../../components/MirrorStatusDot";
import { MirrorHistoryModal } from "../MirrorHistoryModal";

interface Mirror {
  repo_id: string;
  direction: MirrorDirection;
  github_owner: string;
  github_repo: string;
  has_pat: boolean;
  interval_seconds: number;
  auto_push: boolean;
  last_status: MirrorStatus;
  last_error?: string;
  last_synced_at?: string | null;
}

const INTERVALS = [
  { label: "Off", value: 0 },
  { label: "Every 5 minutes", value: 300 },
  { label: "Every 15 minutes", value: 900 },
  { label: "Every hour", value: 3600 },
  { label: "Every 6 hours", value: 21600 },
  { label: "Every 24 hours", value: 86400 },
];

interface Props {
  owner: string;
  repo: string;
  queryClient: QueryClient;
}

export function MirrorTab({ owner, repo, queryClient }: Props) {
  const key = ["mirror", owner, repo];
  const q = useQuery({
    queryKey: key,
    queryFn: () =>
      get<{ mirror: Mirror | null }>(`/repos/${owner}/${repo}/mirror`).then((r) => r.data),
  });

  const m = q.data?.mirror ?? null;

  const [direction, setDirection] = useState<MirrorDirection>("push");
  const [ghOwner, setGhOwner] = useState("");
  const [ghRepo, setGhRepo] = useState("");
  const [pat, setPat] = useState("");
  const [interval, setInterval] = useState(900);
  const [autoPush, setAutoPush] = useState(true);
  const [showHistory, setShowHistory] = useState(false);

  // Sync local state with server state
  useEffect(() => {
    if (m) {
      setDirection(m.direction);
      setGhOwner(m.github_owner);
      setGhRepo(m.github_repo);
      setInterval(m.interval_seconds);
      setAutoPush(m.auto_push);
    }
  }, [m]);

  const save = useMutation({
    mutationFn: (clearPat: boolean = false) =>
      put(`/repos/${owner}/${repo}/mirror`, {
        direction,
        github_owner: ghOwner,
        github_repo: ghRepo,
        pat: pat || undefined,
        clear_pat: clearPat,
        interval_seconds: interval,
        auto_push: autoPush,
      }),
    onSuccess: () => {
      setPat("");
      queryClient.invalidateQueries({ queryKey: key });
    },
  });

  const syncNow = useMutation({
    mutationFn: () => post(`/repos/${owner}/${repo}/mirror/sync`, {}),
    onSuccess: () => {
      // Poll for status update for ~60s
      const iv = window.setInterval(
        () => queryClient.invalidateQueries({ queryKey: key }),
        2000,
      );
      window.setTimeout(() => window.clearInterval(iv), 60000);
    },
  });

  const disable = useMutation({
    mutationFn: () => del(`/repos/${owner}/${repo}/mirror`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: key }),
  });

  const flipDirection = () => setDirection((d) => (d === "push" ? "pull" : "push"));

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate(false);
  };

  return (
    <div className="settings-section">
      <h2>Mirror</h2>
      <form onSubmit={handleSubmit}>
        <div className="mirror-direction">
          <span>Gitwise</span>
          <button
            type="button"
            onClick={flipDirection}
            className="direction-toggle"
            aria-label="Toggle sync direction"
          >
            {direction === "push" ? "→" : "←"}
          </button>
          <span>GitHub</span>
        </div>

        <label>
          GitHub repo
          <div style={{ display: "flex", gap: "0.25rem", alignItems: "center" }}>
            <input
              value={ghOwner}
              onChange={(e) => setGhOwner(e.target.value)}
              placeholder="owner"
              required
            />
            <span>/</span>
            <input
              value={ghRepo}
              onChange={(e) => setGhRepo(e.target.value)}
              placeholder="repo"
              required
            />
          </div>
        </label>

        <label>
          Access token
          <input
            type="password"
            value={pat}
            onChange={(e) => setPat(e.target.value)}
            placeholder={m?.has_pat ? "(unchanged)" : "ghp_…"}
          />
          <small>Optional for pulls from public repos.</small>
          {m?.has_pat && (
            <button type="button" onClick={() => save.mutate(true)}>
              Clear token
            </button>
          )}
        </label>

        <label>
          Sync interval
          <select value={interval} onChange={(e) => setInterval(Number(e.target.value))}>
            {INTERVALS.map((i) => (
              <option key={i.value} value={i.value}>
                {i.label}
              </option>
            ))}
          </select>
        </label>

        {direction === "push" && (
          <label>
            <input
              type="checkbox"
              checked={autoPush}
              onChange={(e) => setAutoPush(e.target.checked)}
            />
            Push immediately when refs change
          </label>
        )}

        <button type="submit" disabled={save.isPending}>
          {m ? "Update mirror" : "Enable mirror"}
        </button>
      </form>

      {m && (
        <div
          className="mirror-status"
          style={{ marginTop: "1rem", display: "flex", gap: "0.5rem", alignItems: "center" }}
        >
          <MirrorStatusDot
            status={m.last_status}
            direction={m.direction}
            lastSyncedAt={m.last_synced_at}
            lastError={m.last_error}
            size={14}
          />
          <span>
            {m.last_status === "failed" ? m.last_error || "Failed" : m.last_status}
          </span>
          <button
            type="button"
            onClick={() => syncNow.mutate()}
            disabled={m.last_status === "running"}
          >
            Sync now
          </button>
          <button type="button" onClick={() => setShowHistory(true)}>
            View history
          </button>
          <button type="button" onClick={() => disable.mutate()} className="danger">
            Disable mirror
          </button>
        </div>
      )}

      {showHistory && (
        <MirrorHistoryModal owner={owner} repo={repo} onClose={() => setShowHistory(false)} />
      )}
    </div>
  );
}

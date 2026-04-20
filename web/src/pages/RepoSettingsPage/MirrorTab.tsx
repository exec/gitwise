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
    refetchInterval: (query) =>
      query.state.data?.mirror?.last_status === "running" ? 2000 : false,
  });

  const m = q.data?.mirror ?? null;

  const [direction, setDirection] = useState<MirrorDirection>("push");
  const [ghOwner, setGhOwner] = useState("");
  const [ghRepo, setGhRepo] = useState("");
  const [pat, setPat] = useState("");
  const [interval, setInterval] = useState(900);
  const [autoPush, setAutoPush] = useState(true);
  const [showHistory, setShowHistory] = useState(false);
  const [showDisableConfirm, setShowDisableConfirm] = useState(false);

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
    onSuccess: () => queryClient.invalidateQueries({ queryKey: key }),
  });

  const disable = useMutation({
    mutationFn: () => del(`/repos/${owner}/${repo}/mirror`),
    onSuccess: () => {
      setShowDisableConfirm(false);
      queryClient.invalidateQueries({ queryKey: key });
    },
  });

  const flipDirection = () => setDirection((d) => (d === "push" ? "pull" : "push"));

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate(false);
  };

  const statusLabel =
    !m ? "" :
    m.last_status === "failed" ? (m.last_error || "Failed") :
    m.last_status === "success" ? "Up to date" :
    m.last_status === "running" ? "Syncing…" :
    "Not yet synced";

  if (q.isLoading) {
    return <p className="muted">Loading…</p>;
  }

  return (
    <div>
      <div className="settings-header">
        <h2>Mirror</h2>
        {m && (
          <div className="mirror-status-inline">
            <MirrorStatusDot
              status={m.last_status}
              direction={m.direction}
              lastSyncedAt={m.last_synced_at}
              lastError={m.last_error}
              size={14}
            />
            <span className="muted">{statusLabel}</span>
          </div>
        )}
      </div>

      <div className="settings-form-card">
        <h3>{m ? "Mirror configuration" : "Set up mirror"}</h3>
        <p className="muted" style={{ marginTop: 0 }}>
          Keep this repository in sync with a GitHub repo. Only git refs
          (commits, branches, tags) are mirrored — issues and PRs stay local.
        </p>

        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label>Direction</label>
            <div className="mirror-direction-row">
              <span className="mirror-direction-end">Gitwise</span>
              <button
                type="button"
                className="mirror-direction-toggle"
                onClick={flipDirection}
                aria-label={`Direction: ${direction === "push" ? "Gitwise to GitHub" : "GitHub to Gitwise"}`}
                title="Click to flip direction"
              >
                {direction === "push" ? "→" : "←"}
              </button>
              <span className="mirror-direction-end">GitHub</span>
            </div>
            <small className="muted">
              {direction === "push"
                ? "Gitwise pushes refs to GitHub on change."
                : "Gitwise pulls refs from GitHub on a schedule."}
            </small>
          </div>

          <div className="form-group">
            <label htmlFor="mirror-gh-owner">GitHub repo</label>
            <div className="mirror-slug-row">
              <input
                id="mirror-gh-owner"
                type="text"
                className="form-input"
                value={ghOwner}
                onChange={(e) => setGhOwner(e.target.value)}
                placeholder="owner"
                required
              />
              <span className="muted">/</span>
              <input
                id="mirror-gh-repo"
                type="text"
                className="form-input"
                value={ghRepo}
                onChange={(e) => setGhRepo(e.target.value)}
                placeholder="repo"
                required
              />
            </div>
          </div>

          <div className="form-group">
            <label htmlFor="mirror-pat">Access token</label>
            <div className="mirror-pat-row">
              <input
                id="mirror-pat"
                type="password"
                className="form-input"
                value={pat}
                onChange={(e) => setPat(e.target.value)}
                placeholder={m?.has_pat ? "•••••••• (unchanged)" : "ghp_…"}
                autoComplete="new-password"
              />
              {m?.has_pat && (
                <button
                  type="button"
                  className="btn btn-secondary btn-sm"
                  onClick={() => save.mutate(true)}
                  disabled={save.isPending}
                >
                  Clear token
                </button>
              )}
            </div>
            <small className="muted">
              Optional for pulls from public repos. Stored encrypted.
            </small>
          </div>

          <div className="form-group">
            <label htmlFor="mirror-interval">Sync interval</label>
            <select
              id="mirror-interval"
              className="form-input form-select"
              value={interval}
              onChange={(e) => setInterval(Number(e.target.value))}
            >
              {INTERVALS.map((i) => (
                <option key={i.value} value={i.value}>
                  {i.label}
                </option>
              ))}
            </select>
          </div>

          {direction === "push" && (
            <div className="form-group">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  checked={autoPush}
                  onChange={(e) => setAutoPush(e.target.checked)}
                />
                Push immediately when refs change
              </label>
            </div>
          )}

          <div className="form-actions">
            <button
              type="submit"
              className="btn btn-primary"
              disabled={save.isPending}
            >
              {save.isPending ? "Saving…" : m ? "Update mirror" : "Enable mirror"}
            </button>
            {m && (
              <>
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={() => syncNow.mutate()}
                  disabled={m.last_status === "running" || syncNow.isPending}
                >
                  Sync now
                </button>
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={() => setShowHistory(true)}
                >
                  View history
                </button>
              </>
            )}
          </div>
        </form>
      </div>

      {m && (
        <div className="danger-zone">
          <h3>Danger Zone</h3>
          <div className="danger-zone-item">
            <div>
              <strong>Disable mirror</strong>
              <p className="muted">
                Stops syncing. Does not delete the repo or its history.
              </p>
            </div>
            <button
              className="btn btn-danger"
              onClick={() => setShowDisableConfirm(true)}
            >
              Disable mirror
            </button>
          </div>
        </div>
      )}

      {showDisableConfirm && (
        <div className="confirm-overlay" onClick={() => setShowDisableConfirm(false)}>
          <div className="confirm-dialog" onClick={(e) => e.stopPropagation()}>
            <h3>Disable mirror?</h3>
            <p>
              This stops syncing to/from GitHub. Existing refs stay in place.
              You can re-enable later.
            </p>
            <div className="confirm-actions">
              <button
                className="btn btn-secondary"
                onClick={() => setShowDisableConfirm(false)}
              >
                Cancel
              </button>
              <button
                className="btn btn-danger"
                disabled={disable.isPending}
                onClick={() => disable.mutate()}
              >
                {disable.isPending ? "Disabling…" : "Disable mirror"}
              </button>
            </div>
          </div>
        </div>
      )}

      {showHistory && (
        <MirrorHistoryModal owner={owner} repo={repo} onClose={() => setShowHistory(false)} />
      )}
    </div>
  );
}

import { useQuery } from "@tanstack/react-query";
import { get } from "../lib/api";

interface Run {
  id: string;
  started_at: string;
  finished_at?: string;
  status: string;
  trigger: string;
  refs_changed?: number;
  error?: string;
  duration_ms?: number;
}

interface Props {
  owner: string;
  repo: string;
  onClose: () => void;
}

export function MirrorHistoryModal({ owner, repo, onClose }: Props) {
  const q = useQuery({
    queryKey: ["mirror-runs", owner, repo],
    queryFn: () =>
      get<{ runs: Run[] }>(`/repos/${owner}/${repo}/mirror/runs`).then((r) => r.data.runs),
  });

  return (
    <div
      className="modal-backdrop"
      onClick={onClose}
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,0.5)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        zIndex: 1000,
      }}
    >
      <div
        className="modal"
        onClick={(e) => e.stopPropagation()}
        style={{
          background: "white",
          padding: "1.5rem",
          borderRadius: "8px",
          maxWidth: "800px",
          maxHeight: "80vh",
          overflow: "auto",
        }}
      >
        <header
          style={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
            marginBottom: "1rem",
          }}
        >
          <h3 style={{ margin: 0 }}>Mirror history</h3>
          <button onClick={onClose} aria-label="Close">
            ×
          </button>
        </header>
        <table style={{ width: "100%", fontSize: "0.9rem" }}>
          <thead>
            <tr>
              <th align="left">When</th>
              <th align="left">Trigger</th>
              <th align="left">Status</th>
              <th align="left">Refs</th>
              <th align="left">Duration</th>
              <th align="left">Error</th>
            </tr>
          </thead>
          <tbody>
            {q.isLoading && (
              <tr>
                <td colSpan={6}>Loading…</td>
              </tr>
            )}
            {q.data?.length === 0 && (
              <tr>
                <td colSpan={6}>No runs yet.</td>
              </tr>
            )}
            {q.data?.map((r) => (
              <tr key={r.id}>
                <td>{new Date(r.started_at).toLocaleString()}</td>
                <td>{r.trigger}</td>
                <td>{r.status}</td>
                <td>{r.refs_changed ?? ""}</td>
                <td>{r.duration_ms ? `${r.duration_ms}ms` : ""}</td>
                <td
                  style={{
                    maxWidth: "300px",
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                  }}
                  title={r.error ?? ""}
                >
                  {r.error ?? ""}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

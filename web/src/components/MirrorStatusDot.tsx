import { useMemo } from "react";
import styles from "./MirrorStatusDot.module.css";

export type MirrorStatus = "pending" | "running" | "success" | "failed";
export type MirrorDirection = "push" | "pull";

interface Props {
  status: MirrorStatus;
  direction: MirrorDirection;
  lastSyncedAt?: string | null; // ISO string
  lastError?: string;
  size?: number; // px
  onRetry?: () => void;
}

export function MirrorStatusDot({
  status,
  direction,
  lastSyncedAt,
  lastError,
  size = 10,
  onRetry,
}: Props) {
  const tooltip = useMemo(() => {
    switch (status) {
      case "running":
        return direction === "pull" ? "Syncing from GitHub…" : "Syncing to GitHub…";
      case "success":
        return lastSyncedAt
          ? `Last synced ${relativeTime(lastSyncedAt)}`
          : "Last sync succeeded";
      case "failed":
        return `Sync failed${lastSyncedAt ? " " + relativeTime(lastSyncedAt) : ""}: ${lastError || "unknown error"}`;
      default:
        return "Mirror configured — not yet synced";
    }
  }, [status, direction, lastSyncedAt, lastError]);

  const clickable = status === "failed" && onRetry;

  return (
    <span
      className={`${styles.dot} ${styles[status]}`}
      style={{ width: size, height: size }}
      title={tooltip}
      onClick={clickable ? onRetry : undefined}
      role={clickable ? "button" : undefined}
    >
      <span className={styles.arrow}>{direction === "push" ? "↑" : "↓"}</span>
    </span>
  );
}

function relativeTime(iso: string): string {
  const now = Date.now();
  const t = new Date(iso).getTime();
  const diff = Math.max(0, Math.floor((now - t) / 1000));
  if (diff < 60) return `${diff}s ago`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

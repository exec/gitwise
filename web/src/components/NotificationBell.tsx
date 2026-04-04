import { useState, useRef, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { useNotifications } from "../hooks/useNotifications";

function timeAgo(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export default function NotificationBell() {
  const { notifications, unreadCount, markRead, markAllRead } = useNotifications();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  const handleNotificationClick = (id: string, link: string, read: boolean) => {
    if (!read) {
      markRead(id);
    }
    setOpen(false);
    if (link) {
      navigate(link);
    }
  };

  return (
    <div className="notification-bell" ref={ref}>
      <button
        className="notification-bell-btn"
        onClick={() => setOpen(!open)}
        aria-label="Notifications"
      >
        <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
          <path
            fillRule="evenodd"
            d="M8 1.5c-2.363 0-4 1.69-4 3.75 0 .984-.08 1.753-.371 2.477-.27.674-.72 1.28-1.396 2.013-.052.056-.096.107-.133.152a1.75 1.75 0 001.325 2.858h2.1a2.5 2.5 0 004.95 0h2.1a1.75 1.75 0 001.325-2.858 3.45 3.45 0 01-.133-.152c-.676-.733-1.126-1.34-1.396-2.013C12.08 6.003 12 5.234 12 4.25 12 3.19 10.363 1.5 8 1.5zM9.5 13h-3a1 1 0 001 1 1 1 0 001-1zM5.5 5.25c0-1.34 1.077-2.25 2.5-2.25s2.5.91 2.5 2.25c0 1.098.093 2.028.45 2.922.346.861.908 1.578 1.609 2.34.052.057.074.082.074.082l.003.005a.25.25 0 01-.19.401H3.554a.25.25 0 01-.19-.401l.003-.005s.022-.025.074-.082c.7-.762 1.263-1.479 1.609-2.34.357-.894.45-1.824.45-2.922z"
          />
        </svg>
        {unreadCount > 0 && (
          <span className="notification-badge">
            {unreadCount > 99 ? "99+" : unreadCount}
          </span>
        )}
      </button>

      {open && (
        <div className="notification-dropdown">
          <div className="notification-header">
            <span className="notification-header-title">Notifications</span>
            {unreadCount > 0 && (
              <button
                className="notification-mark-all"
                onClick={() => markAllRead()}
              >
                Mark all read
              </button>
            )}
          </div>
          <div className="notification-list">
            {notifications.length === 0 ? (
              <div className="notification-empty">No notifications</div>
            ) : (
              notifications.map((n) => (
                <div
                  key={n.id}
                  className={`notification-item ${n.read ? "" : "unread"}`}
                  onClick={() => handleNotificationClick(n.id, n.link, n.read)}
                >
                  <div className="notification-item-title">{n.title}</div>
                  {n.body && (
                    <div className="notification-item-body">
                      {n.body.length > 80 ? n.body.slice(0, 80) + "..." : n.body}
                    </div>
                  )}
                  <div className="notification-item-time">{timeAgo(n.created_at)}</div>
                </div>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}

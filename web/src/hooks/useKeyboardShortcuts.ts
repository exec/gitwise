import { useEffect, useRef, useCallback } from "react";
import { useNavigate, useLocation } from "react-router-dom";

/**
 * Returns true if the active element is a text input where keypresses
 * should not be intercepted as shortcuts.
 */
function isEditableTarget(): boolean {
  const el = document.activeElement;
  if (!el) return false;
  const tag = el.tagName;
  if (tag === "INPUT") {
    const type = (el as HTMLInputElement).type;
    // Allow shortcuts on checkboxes, radios, etc.
    const textTypes = ["text", "search", "url", "tel", "email", "password", "number"];
    return textTypes.includes(type);
  }
  if (tag === "TEXTAREA" || tag === "SELECT") return true;
  if ((el as HTMLElement).isContentEditable) return true;
  return false;
}

/**
 * Extracts the /:owner/:repo prefix from the current path, or null.
 */
function getRepoPrefix(pathname: string): string | null {
  // Match /:owner/:repo, but not system routes
  const systemRoutes = [
    "/login", "/register", "/new", "/search", "/settings", "/orgs",
  ];
  for (const prefix of systemRoutes) {
    if (pathname === prefix || pathname.startsWith(prefix + "/")) return null;
  }
  const match = pathname.match(/^\/([^/]+)\/([^/]+)/);
  if (match) {
    // Exclude the special case where the second segment is a system route part
    return `/${match[1]}/${match[2]}`;
  }
  return null;
}

export interface ShortcutsCallbacks {
  onToggleHelp: () => void;
  onFocusSearch: () => void;
}

export function useKeyboardShortcuts(callbacks: ShortcutsCallbacks) {
  const navigate = useNavigate();
  const location = useLocation();
  const pendingPrefix = useRef<string | null>(null);
  const prefixTimeout = useRef<ReturnType<typeof setTimeout> | null>(null);

  const clearPrefix = useCallback(() => {
    pendingPrefix.current = null;
    if (prefixTimeout.current) {
      clearTimeout(prefixTimeout.current);
      prefixTimeout.current = null;
    }
  }, []);

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      // Ignore if user is typing in an editable field
      if (isEditableTarget()) {
        clearPrefix();
        return;
      }

      // Ignore if any modifier is held (except for Cmd/Ctrl+K)
      const hasModifier = e.ctrlKey || e.metaKey || e.altKey;
      const key = e.key;

      // Cmd/Ctrl+K: focus search (command palette style)
      if ((e.metaKey || e.ctrlKey) && key === "k") {
        e.preventDefault();
        clearPrefix();
        callbacks.onFocusSearch();
        return;
      }

      // All other shortcuts require no modifiers
      if (hasModifier) {
        clearPrefix();
        return;
      }

      // Check if we are in a two-key sequence
      if (pendingPrefix.current === "g") {
        clearPrefix();
        const repoPrefix = getRepoPrefix(location.pathname);

        switch (key) {
          case "i":
            if (repoPrefix) {
              e.preventDefault();
              navigate(`${repoPrefix}/issues`);
            }
            return;
          case "p":
            if (repoPrefix) {
              e.preventDefault();
              navigate(`${repoPrefix}/pulls`);
            }
            return;
          case "c":
            if (repoPrefix) {
              e.preventDefault();
              navigate(repoPrefix);
            }
            return;
          case "n":
            e.preventDefault();
            navigate("/");
            return;
          default:
            // Unknown second key, ignore
            return;
        }
      }

      // Start a two-key sequence
      if (key === "g") {
        e.preventDefault();
        pendingPrefix.current = "g";
        prefixTimeout.current = setTimeout(() => {
          pendingPrefix.current = null;
        }, 1000);
        return;
      }

      // Single key shortcuts
      switch (key) {
        case "/":
        case "s":
          e.preventDefault();
          clearPrefix();
          callbacks.onFocusSearch();
          return;
        case "?":
          e.preventDefault();
          clearPrefix();
          callbacks.onToggleHelp();
          return;
        case "Escape":
          clearPrefix();
          return;
      }
    }

    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      if (prefixTimeout.current) {
        clearTimeout(prefixTimeout.current);
      }
    };
  }, [navigate, location.pathname, callbacks, clearPrefix]);
}

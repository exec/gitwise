import { useEffect, useRef } from "react";

interface KeyboardShortcutsHelpProps {
  isOpen: boolean;
  onClose: () => void;
}

interface ShortcutEntry {
  keys: string[];
  description: string;
}

interface ShortcutGroup {
  title: string;
  shortcuts: ShortcutEntry[];
}

const shortcutGroups: ShortcutGroup[] = [
  {
    title: "Site-wide shortcuts",
    shortcuts: [
      { keys: ["/", "s"], description: "Focus search bar" },
      { keys: ["?"], description: "Show keyboard shortcuts" },
      { keys: ["Esc"], description: "Close dialog / cancel" },
    ],
  },
  {
    title: "Repository navigation",
    shortcuts: [
      { keys: ["g", "c"], description: "Go to Code" },
      { keys: ["g", "i"], description: "Go to Issues" },
      { keys: ["g", "p"], description: "Go to Pull Requests" },
      { keys: ["g", "n"], description: "Go to Dashboard" },
    ],
  },
];

function Key({ children }: { children: string }) {
  return <kbd className="shortcut-key">{children}</kbd>;
}

export default function KeyboardShortcutsHelp({ isOpen, onClose }: KeyboardShortcutsHelpProps) {
  const dialogRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!isOpen) return;

    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape" || e.key === "?") {
        e.preventDefault();
        e.stopPropagation();
        onClose();
      }
    }

    // Use capture phase so we intercept before the main shortcut handler
    document.addEventListener("keydown", handleKeyDown, true);
    return () => document.removeEventListener("keydown", handleKeyDown, true);
  }, [isOpen, onClose]);

  useEffect(() => {
    if (!isOpen) return;

    function handleClick(e: MouseEvent) {
      if (dialogRef.current && !dialogRef.current.contains(e.target as Node)) {
        onClose();
      }
    }

    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  return (
    <div className="shortcuts-overlay">
      <div className="shortcuts-dialog" ref={dialogRef}>
        <div className="shortcuts-header">
          <h2>Keyboard shortcuts</h2>
          <button className="shortcuts-close" onClick={onClose} aria-label="Close">
            <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
              <path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z" />
            </svg>
          </button>
        </div>
        <div className="shortcuts-body">
          {shortcutGroups.map((group) => (
            <div className="shortcuts-group" key={group.title}>
              <h3 className="shortcuts-group-title">{group.title}</h3>
              <ul className="shortcuts-list">
                {group.shortcuts.map((shortcut) => (
                  <li className="shortcuts-row" key={shortcut.description}>
                    <span className="shortcuts-keys">
                      {shortcut.keys.map((k, i) => (
                        <span key={i}>
                          {i > 0 && <span className="shortcuts-then">then</span>}
                          <Key>{k}</Key>
                        </span>
                      ))}
                    </span>
                    <span className="shortcuts-desc">{shortcut.description}</span>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

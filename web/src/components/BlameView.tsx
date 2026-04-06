import { useMemo } from "react";
import { highlightCode } from "../lib/syntax";

function formatRelativeDate(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

  if (diffDays === 0) return "today";
  if (diffDays === 1) return "yesterday";
  if (diffDays < 30) return `${diffDays} days ago`;
  if (diffDays < 365) {
    const months = Math.floor(diffDays / 30);
    return months === 1 ? "1 month ago" : `${months} months ago`;
  }
  const years = Math.floor(diffDays / 365);
  return years === 1 ? "1 year ago" : `${years} years ago`;
}

export interface BlameLineData {
  commit_sha: string;
  author_name: string;
  author_email: string;
  date: string;
  line_number: number;
  line_content: string;
}

interface BlameViewProps {
  lines: BlameLineData[];
  filename: string;
}

/**
 * Renders blame data with syntax highlighting and commit info gutter.
 *
 * Safety note on dangerouslySetInnerHTML usage: Prism.highlight() receives
 * raw source code text and escapes all HTML entities (&, <, >) internally
 * before wrapping recognized syntax tokens in <span> elements. The
 * plain-text fallback also applies explicit escapeHtml(). No user-supplied
 * HTML reaches the DOM unescaped through either path. This is the same
 * pattern used by the existing CodeView component.
 */
export default function BlameView({ lines, filename }: BlameViewProps) {
  const highlightedLines = useMemo(() => {
    const code = lines.map((l) => l.line_content).join("\n");
    return highlightCode(code, filename).split("\n");
  }, [lines, filename]);

  // Group consecutive lines by commit SHA to show commit info only on first line of group
  const groups = useMemo(() => {
    const result: { startLine: number; endLine: number; sha: string; author: string; date: string }[] = [];
    let current: typeof result[0] | null = null;

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      if (!current || current.sha !== line.commit_sha) {
        if (current) result.push(current);
        current = {
          startLine: i,
          endLine: i,
          sha: line.commit_sha,
          author: line.author_name,
          date: line.date,
        };
      } else {
        current.endLine = i;
      }
    }
    if (current) result.push(current);
    return result;
  }, [lines]);

  // Build a lookup: line index -> group index
  const lineToGroup = useMemo(() => {
    const map = new Map<number, number>();
    groups.forEach((g, gi) => {
      for (let i = g.startLine; i <= g.endLine; i++) {
        map.set(i, gi);
      }
    });
    return map;
  }, [groups]);

  return (
    <div className="blame-view-container">
      <table className="blame-view-table">
        <tbody>
          {lines.map((line, i) => {
            const groupIdx = lineToGroup.get(i) ?? 0;
            const group = groups[groupIdx];
            const isFirstInGroup = i === group.startLine;
            const isLastInGroup = i === group.endLine;

            return (
              <tr
                key={i}
                className={`blame-view-line${isLastInGroup ? " blame-group-end" : ""}`}
              >
                <td className="blame-commit-cell">
                  {isFirstInGroup ? (
                    <div className="blame-commit-info">
                      <span className="blame-sha">{line.commit_sha.slice(0, 7)}</span>
                      <span className="blame-author">{line.author_name}</span>
                      <span className="blame-date">{formatRelativeDate(line.date)}</span>
                    </div>
                  ) : null}
                </td>
                <td className="blame-ln">{line.line_number}</td>
                {/* eslint-disable-next-line react/no-danger -- Prism escapes all HTML entities before wrapping tokens in spans; see component docstring for safety analysis */}
                <td
                  className="blame-code"
                  dangerouslySetInnerHTML={{
                    __html: highlightedLines[i] || "&nbsp;",
                  }}
                />
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

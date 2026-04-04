interface DiffFile {
  path: string;
  old_path?: string;
  status: string;
  insertions: number;
  deletions: number;
  patch?: string;
}

interface DiffViewerProps {
  files: DiffFile[];
}

export default function DiffViewer({ files }: DiffViewerProps) {
  return (
    <div className="diff-viewer">
      {files.map((file, idx) => (
        <div key={idx} className="diff-file">
          <div className="diff-file-header">
            <span className={`diff-status diff-status-${file.status}`}>
              {file.status === "added"
                ? "A"
                : file.status === "deleted"
                  ? "D"
                  : file.status === "renamed"
                    ? "R"
                    : "M"}
            </span>
            <span className="diff-file-path">
              {file.old_path && file.old_path !== file.path
                ? `${file.old_path} \u2192 ${file.path}`
                : file.path}
            </span>
            <span className="diff-file-stats">
              {file.insertions > 0 && (
                <span className="additions">+{file.insertions}</span>
              )}
              {file.deletions > 0 && (
                <span className="deletions">-{file.deletions}</span>
              )}
            </span>
          </div>
          {file.patch && (
            <div className="diff-content">
              <table className="diff-table">
                <tbody>
                  {parsePatch(file.patch).map((line, lineIdx) => (
                    <tr key={lineIdx} className={`diff-line diff-${line.type}`}>
                      <td className="diff-line-num diff-line-num-old">
                        {line.oldNum ?? ""}
                      </td>
                      <td className="diff-line-num diff-line-num-new">
                        {line.newNum ?? ""}
                      </td>
                      <td className="diff-line-content">
                        <span className="diff-line-prefix">
                          {line.type === "add"
                            ? "+"
                            : line.type === "del"
                              ? "-"
                              : line.type === "hunk"
                                ? ""
                                : " "}
                        </span>
                        {line.content}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      ))}
    </div>
  );
}

interface PatchLine {
  type: "context" | "add" | "del" | "hunk";
  content: string;
  oldNum?: number;
  newNum?: number;
}

function parsePatch(patch: string): PatchLine[] {
  const lines: PatchLine[] = [];
  let oldLine = 0;
  let newLine = 0;

  for (const raw of patch.split("\n")) {
    if (raw.startsWith("@@")) {
      const match = raw.match(/@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
      if (match) {
        oldLine = parseInt(match[1], 10);
        newLine = parseInt(match[2], 10);
      }
      lines.push({ type: "hunk", content: raw });
    } else if (raw.startsWith("+")) {
      lines.push({
        type: "add",
        content: raw.slice(1),
        newNum: newLine++,
      });
    } else if (raw.startsWith("-")) {
      lines.push({
        type: "del",
        content: raw.slice(1),
        oldNum: oldLine++,
      });
    } else if (raw.startsWith(" ")) {
      lines.push({
        type: "context",
        content: raw.slice(1),
        oldNum: oldLine++,
        newNum: newLine++,
      });
    } else if (raw.startsWith("diff --git") || raw.startsWith("index ") || raw.startsWith("---") || raw.startsWith("+++")) {
      // Skip diff headers
    } else if (raw.trim()) {
      lines.push({
        type: "context",
        content: raw,
        oldNum: oldLine++,
        newNum: newLine++,
      });
    }
  }
  return lines;
}

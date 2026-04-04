import { useState } from 'react';
import React from 'react';

type ViewMode = 'unified' | 'split';

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

const CODE_EXTENSIONS = new Set([
  '.js', '.ts', '.tsx', '.jsx', '.go', '.rs', '.py', '.java', '.c', '.h',
  '.cpp', '.hpp', '.rb', '.swift', '.kt', '.scala', '.sh', '.bash', '.zsh',
  '.css', '.scss', '.less', '.json', '.yaml', '.yml', '.toml', '.xml',
]);

const KEYWORDS = new Set([
  'function', 'const', 'let', 'var', 'if', 'else', 'for', 'while', 'return',
  'import', 'export', 'class', 'struct', 'type', 'interface', 'func', 'def',
  'pub', 'fn', 'async', 'await', 'switch', 'case', 'break', 'continue',
  'new', 'this', 'self', 'true', 'false', 'null', 'nil', 'undefined',
  'package', 'from', 'try', 'catch', 'throw', 'finally', 'default',
  'extends', 'implements', 'static', 'void', 'int', 'string', 'bool',
]);

function getExtension(filePath: string): string {
  const dot = filePath.lastIndexOf('.');
  if (dot === -1) return '';
  return filePath.slice(dot).toLowerCase();
}

function highlightLine(content: string, filePath: string): React.ReactNode {
  const ext = getExtension(filePath);
  if (!CODE_EXTENSIONS.has(ext)) {
    return content;
  }

  const tokens: React.ReactNode[] = [];
  let i = 0;
  let key = 0;

  while (i < content.length) {
    // Comments: // to end of line
    if (content[i] === '/' && content[i + 1] === '/') {
      tokens.push(
        <span key={key++} className="hl-comment">{content.slice(i)}</span>
      );
      i = content.length;
      continue;
    }

    // Comments: # to end of line (Python, shell, YAML)
    if (content[i] === '#' && (ext === '.py' || ext === '.sh' || ext === '.bash' || ext === '.zsh' || ext === '.yaml' || ext === '.yml' || ext === '.toml')) {
      tokens.push(
        <span key={key++} className="hl-comment">{content.slice(i)}</span>
      );
      i = content.length;
      continue;
    }

    // Strings: double-quoted
    if (content[i] === '"') {
      let j = i + 1;
      while (j < content.length && content[j] !== '"') {
        if (content[j] === '\\') j++; // skip escaped char
        j++;
      }
      j = Math.min(j + 1, content.length);
      tokens.push(
        <span key={key++} className="hl-string">{content.slice(i, j)}</span>
      );
      i = j;
      continue;
    }

    // Strings: single-quoted
    if (content[i] === "'") {
      let j = i + 1;
      while (j < content.length && content[j] !== "'") {
        if (content[j] === '\\') j++;
        j++;
      }
      j = Math.min(j + 1, content.length);
      tokens.push(
        <span key={key++} className="hl-string">{content.slice(i, j)}</span>
      );
      i = j;
      continue;
    }

    // Backtick strings (template literals)
    if (content[i] === '`') {
      let j = i + 1;
      while (j < content.length && content[j] !== '`') {
        if (content[j] === '\\') j++;
        j++;
      }
      j = Math.min(j + 1, content.length);
      tokens.push(
        <span key={key++} className="hl-string">{content.slice(i, j)}</span>
      );
      i = j;
      continue;
    }

    // Numbers
    if (/[0-9]/.test(content[i]) && (i === 0 || /[\s(,=+\-*/<>[\]{}:;!&|^~%]/.test(content[i - 1]))) {
      let j = i;
      while (j < content.length && /[0-9a-fA-Fx._]/.test(content[j])) j++;
      tokens.push(
        <span key={key++} className="hl-number">{content.slice(i, j)}</span>
      );
      i = j;
      continue;
    }

    // Keywords (word boundary match)
    if (/[a-zA-Z_]/.test(content[i])) {
      let j = i;
      while (j < content.length && /[a-zA-Z0-9_]/.test(content[j])) j++;
      const word = content.slice(i, j);
      if (KEYWORDS.has(word)) {
        tokens.push(
          <span key={key++} className="hl-keyword">{word}</span>
        );
      } else {
        tokens.push(<React.Fragment key={key++}>{word}</React.Fragment>);
      }
      i = j;
      continue;
    }

    // Plain character — accumulate consecutive plain chars
    let j = i;
    while (j < content.length &&
      !/[a-zA-Z_0-9"'`#/]/.test(content[j])) {
      j++;
    }
    if (j === i) j = i + 1; // advance at least one char
    tokens.push(<React.Fragment key={key++}>{content.slice(i, j)}</React.Fragment>);
    i = j;
  }

  return tokens.length > 0 ? tokens : content;
}

interface SideBySidePair {
  left: PatchLine | null;
  right: PatchLine | null;
  isHunk: boolean;
}

function parsePatchSideBySide(lines: PatchLine[]): SideBySidePair[] {
  const pairs: SideBySidePair[] = [];
  let i = 0;

  while (i < lines.length) {
    const line = lines[i];

    if (line.type === 'hunk') {
      pairs.push({ left: line, right: line, isHunk: true });
      i++;
      continue;
    }

    if (line.type === 'context') {
      pairs.push({ left: line, right: line, isHunk: false });
      i++;
      continue;
    }

    // Collect consecutive del lines
    const dels: PatchLine[] = [];
    while (i < lines.length && lines[i].type === 'del') {
      dels.push(lines[i]);
      i++;
    }

    // Collect consecutive add lines
    const adds: PatchLine[] = [];
    while (i < lines.length && lines[i].type === 'add') {
      adds.push(lines[i]);
      i++;
    }

    // Pair them up
    const maxLen = Math.max(dels.length, adds.length);
    for (let j = 0; j < maxLen; j++) {
      pairs.push({
        left: j < dels.length ? dels[j] : null,
        right: j < adds.length ? adds[j] : null,
        isHunk: false,
      });
    }
  }

  return pairs;
}

export default function DiffViewer({ files }: DiffViewerProps) {
  const [viewMode, setViewMode] = useState<ViewMode>('unified');

  return (
    <div className="diff-viewer">
      <div className="diff-view-toggle">
        <button
          className={`btn btn-sm ${viewMode === 'unified' ? 'btn-primary' : 'btn-secondary'}`}
          onClick={() => setViewMode('unified')}
        >
          Unified
        </button>
        <button
          className={`btn btn-sm ${viewMode === 'split' ? 'btn-primary' : 'btn-secondary'}`}
          onClick={() => setViewMode('split')}
        >
          Split
        </button>
      </div>
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
          {file.patch && viewMode === 'unified' && (
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
                        {highlightLine(line.content, file.path)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {file.patch && viewMode === 'split' && (
            <div className="diff-content">
              <table className="diff-split-table">
                <tbody>
                  {parsePatchSideBySide(parsePatch(file.patch)).map((pair, pairIdx) => {
                    if (pair.isHunk) {
                      return (
                        <tr key={pairIdx} className="diff-line diff-hunk">
                          <td className="diff-split-num diff-line-num">{""}</td>
                          <td className="diff-split-content diff-line-content" colSpan={3}>
                            {pair.left?.content}
                          </td>
                        </tr>
                      );
                    }
                    const leftClass = pair.left?.type === 'del' ? 'diff-del' : '';
                    const rightClass = pair.right?.type === 'add' ? 'diff-add' : '';
                    return (
                      <tr key={pairIdx} className="diff-line">
                        <td className={`diff-split-num diff-line-num ${leftClass}`}>
                          {pair.left?.oldNum ?? pair.left?.newNum ?? ""}
                        </td>
                        <td className={`diff-split-content ${leftClass}`}>
                          {pair.left ? highlightLine(pair.left.content, file.path) : ""}
                        </td>
                        <td className="diff-split-divider"></td>
                        <td className={`diff-split-num diff-line-num ${rightClass}`}>
                          {pair.right?.newNum ?? pair.right?.oldNum ?? ""}
                        </td>
                        <td className={`diff-split-content ${rightClass}`}>
                          {pair.right ? highlightLine(pair.right.content, file.path) : ""}
                        </td>
                      </tr>
                    );
                  })}
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

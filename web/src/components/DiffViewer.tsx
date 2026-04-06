import { Fragment, useState } from "react";
import React from "react";

type ViewMode = "unified" | "split";

interface DiffFile {
  path: string;
  old_path?: string;
  status: string;
  insertions: number;
  deletions: number;
  patch?: string;
}

interface InlineComment {
  path: string;
  line: number;
  side: string;
  body: string;
  author_name?: string;
  pendingIndex?: number;
}

interface DiffViewerProps {
  files: DiffFile[];
  onAddInlineComment?: (path: string, line: number, side: string, body: string) => void;
  onRemoveInlineComment?: (pendingIndex: number) => void;
  inlineComments?: InlineComment[];
}

const CODE_EXTENSIONS = new Set([
  ".js", ".ts", ".tsx", ".jsx", ".go", ".rs", ".py", ".java", ".c", ".h",
  ".cpp", ".hpp", ".rb", ".swift", ".kt", ".scala", ".sh", ".bash", ".zsh",
  ".css", ".scss", ".less", ".json", ".yaml", ".yml", ".toml", ".xml",
]);

const KEYWORDS = new Set([
  "function", "const", "let", "var", "if", "else", "for", "while", "return",
  "import", "export", "class", "struct", "type", "interface", "func", "def",
  "pub", "fn", "async", "await", "switch", "case", "break", "continue",
  "new", "this", "self", "true", "false", "null", "nil", "undefined",
  "package", "from", "try", "catch", "throw", "finally", "default",
  "extends", "implements", "static", "void", "int", "string", "bool",
]);

function getExtension(filePath: string): string {
  const dot = filePath.lastIndexOf(".");
  if (dot === -1) return "";
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
    if (content[i] === "/" && content[i + 1] === "/") {
      tokens.push(
        <span key={key++} className="hl-comment">{content.slice(i)}</span>
      );
      i = content.length;
      continue;
    }

    // Comments: # to end of line (Python, shell, YAML)
    if (content[i] === "#" && (ext === ".py" || ext === ".sh" || ext === ".bash" || ext === ".zsh" || ext === ".yaml" || ext === ".yml" || ext === ".toml")) {
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
        if (content[j] === "\\") j++; // skip escaped char
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
        if (content[j] === "\\") j++;
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
    if (content[i] === "`") {
      let j = i + 1;
      while (j < content.length && content[j] !== "`") {
        if (content[j] === "\\") j++;
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

    if (line.type === "hunk") {
      pairs.push({ left: line, right: line, isHunk: true });
      i++;
      continue;
    }

    if (line.type === "context") {
      pairs.push({ left: line, right: line, isHunk: false });
      i++;
      continue;
    }

    // Collect consecutive del lines
    const dels: PatchLine[] = [];
    while (i < lines.length && lines[i].type === "del") {
      dels.push(lines[i]);
      i++;
    }

    // Collect consecutive add lines
    const adds: PatchLine[] = [];
    while (i < lines.length && lines[i].type === "add") {
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

export default function DiffViewer({ files, onAddInlineComment, onRemoveInlineComment, inlineComments }: DiffViewerProps) {
  const [viewMode, setViewMode] = useState<ViewMode>("unified");
  const [commentForm, setCommentForm] = useState<{path: string, line: number, side: string} | null>(null);
  const [commentText, setCommentText] = useState("");

  function handleSubmitComment() {
    if (commentForm && onAddInlineComment && commentText.trim()) {
      onAddInlineComment(commentForm.path, commentForm.line, commentForm.side, commentText);
      setCommentForm(null);
      setCommentText("");
    }
  }

  function handleCancelComment() {
    setCommentForm(null);
    setCommentText("");
  }

  function getCommentsForLine(path: string, line: number, side: string): InlineComment[] {
    if (!inlineComments) return [];
    return inlineComments.filter(c => c.path === path && c.line === line && c.side === side);
  }

  return (
    <div className="diff-viewer">
      <div className="diff-view-toggle">
        <button
          className={`btn btn-sm ${viewMode === "unified" ? "btn-primary" : "btn-secondary"}`}
          onClick={() => setViewMode("unified")}
        >
          Unified
        </button>
        <button
          className={`btn btn-sm ${viewMode === "split" ? "btn-primary" : "btn-secondary"}`}
          onClick={() => setViewMode("split")}
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
          {file.patch && viewMode === "unified" && (
            <div className="diff-content">
              <table className="diff-table">
                <tbody>
                  {parsePatch(file.patch).map((line, lineIdx) => {
                    const lineNum = line.type === "del" ? line.oldNum : line.newNum;
                    const side = line.type === "del" ? "left" : "right";
                    const lineComments = lineNum != null
                      ? getCommentsForLine(file.path, lineNum, side)
                      : [];
                    const isFormOpen = commentForm != null
                      && commentForm.path === file.path
                      && commentForm.line === lineNum
                      && commentForm.side === side;

                    return (
                      <Fragment key={lineIdx}>
                        <tr className={`diff-line diff-${line.type}`}>
                          <td className="diff-line-num diff-line-num-old">
                            {onAddInlineComment && line.type !== "hunk" && lineNum != null && (
                              <button
                                className="inline-comment-btn"
                                onClick={() => setCommentForm({ path: file.path, line: lineNum, side })}
                                title="Add comment"
                              >
                                +
                              </button>
                            )}
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
                        {lineComments.length > 0 && (
                          <tr className="inline-comment-row">
                            <td colSpan={3}>
                              {lineComments.map((c, ci) => (
                                <div key={ci} className="inline-comment-display">
                                  {c.author_name && <strong>{c.author_name}</strong>}
                                  {c.body}
                                  {c.pendingIndex != null && onRemoveInlineComment && (
                                    <button
                                      className="inline-comment-remove-btn"
                                      onClick={() => onRemoveInlineComment(c.pendingIndex!)}
                                      title="Remove pending comment"
                                    >
                                      &times;
                                    </button>
                                  )}
                                </div>
                              ))}
                            </td>
                          </tr>
                        )}
                        {isFormOpen && (
                          <tr className="inline-comment-row">
                            <td colSpan={3}>
                              <div className="inline-comment-form">
                                <textarea
                                  autoFocus
                                  rows={3}
                                  placeholder="Write a comment..."
                                  value={commentText}
                                  onChange={(e) => setCommentText(e.target.value)}
                                />
                                <div className="inline-comment-actions">
                                  <button
                                    className="btn btn-primary btn-sm"
                                    disabled={!commentText.trim()}
                                    onClick={handleSubmitComment}
                                  >
                                    Add Comment
                                  </button>
                                  <button
                                    className="btn btn-secondary btn-sm"
                                    onClick={handleCancelComment}
                                  >
                                    Cancel
                                  </button>
                                </div>
                              </div>
                            </td>
                          </tr>
                        )}
                      </Fragment>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
          {file.patch && viewMode === "split" && (
            <div className="diff-content">
              <table className="diff-split-table">
                <tbody>
                  {parsePatchSideBySide(parsePatch(file.patch)).map((pair, pairIdx) => {
                    if (pair.isHunk) {
                      return (
                        <tr key={pairIdx} className="diff-line diff-hunk">
                          <td className="diff-split-num diff-line-num">{""}</td>
                          <td className="diff-split-content diff-line-content" colSpan={4}>
                            {pair.left?.content}
                          </td>
                        </tr>
                      );
                    }
                    const leftClass = pair.left?.type === "del" ? "diff-del" : "";
                    const rightClass = pair.right?.type === "add" ? "diff-add" : "";

                    // Determine line number and side for inline comments
                    const splitLineNum = pair.left?.type === "del"
                      ? pair.left.oldNum
                      : pair.right?.type === "add"
                        ? pair.right.newNum
                        : (pair.right?.newNum ?? pair.left?.newNum);
                    const splitSide = pair.left?.type === "del"
                      ? "left"
                      : "right";
                    const splitComments = splitLineNum != null
                      ? getCommentsForLine(file.path, splitLineNum, splitSide)
                      : [];
                    const splitFormOpen = commentForm != null
                      && commentForm.path === file.path
                      && commentForm.line === splitLineNum
                      && commentForm.side === splitSide;

                    return (
                      <Fragment key={pairIdx}>
                        <tr className="diff-line">
                          <td className={`diff-split-num diff-line-num ${leftClass}`}>
                            {onAddInlineComment && splitLineNum != null && (
                              <button
                                className="inline-comment-btn"
                                onClick={() => setCommentForm({ path: file.path, line: splitLineNum, side: splitSide })}
                                title="Add comment"
                              >
                                +
                              </button>
                            )}
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
                        {splitComments.length > 0 && (
                          <tr className="inline-comment-row">
                            <td colSpan={5}>
                              {splitComments.map((c, ci) => (
                                <div key={ci} className="inline-comment-display">
                                  {c.author_name && <strong>{c.author_name}</strong>}
                                  {c.body}
                                  {c.pendingIndex != null && onRemoveInlineComment && (
                                    <button
                                      className="inline-comment-remove-btn"
                                      onClick={() => onRemoveInlineComment(c.pendingIndex!)}
                                      title="Remove pending comment"
                                    >
                                      &times;
                                    </button>
                                  )}
                                </div>
                              ))}
                            </td>
                          </tr>
                        )}
                        {splitFormOpen && (
                          <tr className="inline-comment-row">
                            <td colSpan={5}>
                              <div className="inline-comment-form">
                                <textarea
                                  autoFocus
                                  rows={3}
                                  placeholder="Write a comment..."
                                  value={commentText}
                                  onChange={(e) => setCommentText(e.target.value)}
                                />
                                <div className="inline-comment-actions">
                                  <button
                                    className="btn btn-primary btn-sm"
                                    disabled={!commentText.trim()}
                                    onClick={handleSubmitComment}
                                  >
                                    Add Comment
                                  </button>
                                  <button
                                    className="btn btn-secondary btn-sm"
                                    onClick={handleCancelComment}
                                  >
                                    Cancel
                                  </button>
                                </div>
                              </div>
                            </td>
                          </tr>
                        )}
                      </Fragment>
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

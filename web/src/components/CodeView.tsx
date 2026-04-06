import { useMemo } from "react";
import { highlightCode } from "../lib/syntax";

interface CodeViewProps {
  code: string;
  filename: string;
}

/**
 * Renders source code with Prism.js syntax highlighting and line numbers.
 *
 * Safety note on dangerouslySetInnerHTML: Prism.highlight() escapes all HTML
 * entities in the source code before wrapping syntax tokens in <span> elements.
 * The plain-text fallback path also explicitly escapes &, <, and >. User input
 * cannot inject raw HTML through either path.
 */
export default function CodeView({ code, filename }: CodeViewProps) {
  const highlighted = useMemo(() => highlightCode(code, filename), [code, filename]);

  const lines = highlighted.split("\n");
  // Remove trailing empty line that results from a final newline
  if (lines.length > 1 && lines[lines.length - 1] === "") {
    lines.pop();
  }

  return (
    <div className="code-view-container">
      <table className="code-view-table">
        <tbody>
          {lines.map((line, i) => (
            <tr key={i} className="code-view-line">
              <td className="code-view-ln">{i + 1}</td>
              {/* eslint-disable-next-line react/no-danger -- Prism escapes HTML entities; see component docstring */}
              <td
                className="code-view-code"
                dangerouslySetInnerHTML={{ __html: line || "&nbsp;" }}
              />
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

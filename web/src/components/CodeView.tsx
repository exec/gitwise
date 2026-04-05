import { useMemo } from "react";
import Prism from "prismjs";
import "prismjs/components/prism-typescript";
import "prismjs/components/prism-javascript";
import "prismjs/components/prism-jsx";
import "prismjs/components/prism-tsx";
import "prismjs/components/prism-go";
import "prismjs/components/prism-python";
import "prismjs/components/prism-rust";
import "prismjs/components/prism-bash";
import "prismjs/components/prism-json";
import "prismjs/components/prism-yaml";
import "prismjs/components/prism-css";
import "prismjs/components/prism-sql";
import "prismjs/components/prism-markdown";
import "prismjs/components/prism-toml";
import "prismjs/components/prism-docker";
import "prismjs/components/prism-ruby";
import "prismjs/components/prism-java";
import "prismjs/components/prism-c";
import "prismjs/components/prism-cpp";
import "prismjs/components/prism-markup";
import "prismjs/components/prism-xml-doc";

const EXT_TO_LANG: Record<string, string> = {
  ts: "typescript",
  tsx: "tsx",
  js: "javascript",
  jsx: "jsx",
  mjs: "javascript",
  cjs: "javascript",
  go: "go",
  py: "python",
  rs: "rust",
  sh: "bash",
  bash: "bash",
  zsh: "bash",
  json: "json",
  yaml: "yaml",
  yml: "yaml",
  css: "css",
  scss: "css",
  sql: "sql",
  md: "markdown",
  toml: "toml",
  rb: "ruby",
  java: "java",
  c: "c",
  h: "c",
  cpp: "cpp",
  cxx: "cpp",
  cc: "cpp",
  hpp: "cpp",
  hxx: "cpp",
  html: "markup",
  htm: "markup",
  xml: "markup",
  svg: "markup",
  Makefile: "makefile",
  Dockerfile: "docker",
};

const FILENAME_TO_LANG: Record<string, string> = {
  Makefile: "bash",
  Dockerfile: "docker",
  Vagrantfile: "ruby",
  Gemfile: "ruby",
  Rakefile: "ruby",
};

function getLanguage(filename: string): string | null {
  const basename = filename.split("/").pop() ?? filename;

  if (FILENAME_TO_LANG[basename]) {
    return FILENAME_TO_LANG[basename];
  }

  const dotIndex = basename.lastIndexOf(".");
  if (dotIndex === -1) return null;

  const ext = basename.slice(dotIndex + 1);
  return EXT_TO_LANG[ext] ?? null;
}

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

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
  const highlighted = useMemo(() => {
    const lang = getLanguage(filename);

    if (!lang || !Prism.languages[lang]) {
      return escapeHtml(code);
    }

    return Prism.highlight(code, Prism.languages[lang], lang);
  }, [code, filename]);

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
                dangerouslySetInnerHTML={{ __html: line || "\n" }}
              />
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

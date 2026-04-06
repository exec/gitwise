import Prism from "prismjs";
import "prismjs/components/prism-javascript";
import "prismjs/components/prism-typescript";
import "prismjs/components/prism-jsx";
import "prismjs/components/prism-tsx";
import "prismjs/components/prism-go";
import "prismjs/components/prism-python";
import "prismjs/components/prism-rust";
import "prismjs/components/prism-bash";
import "prismjs/components/prism-json";
import "prismjs/components/prism-yaml";
import "prismjs/components/prism-css";
import "prismjs/components/prism-scss";
import "prismjs/components/prism-sql";
import "prismjs/components/prism-markdown";
import "prismjs/components/prism-toml";
import "prismjs/components/prism-docker";
import "prismjs/components/prism-makefile";
import "prismjs/components/prism-ruby";
import "prismjs/components/prism-java";
import "prismjs/components/prism-c";
import "prismjs/components/prism-cpp";
import "prismjs/components/prism-markup";

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
  scss: "scss",
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
  Dockerfile: "docker",
};

const FILENAME_TO_LANG: Record<string, string> = {
  Makefile: "makefile",
  Dockerfile: "docker",
  Vagrantfile: "ruby",
  Gemfile: "ruby",
  Rakefile: "ruby",
};

export function getLanguage(filename: string): string | null {
  const basename = filename.split("/").pop() ?? filename;
  if (FILENAME_TO_LANG[basename]) return FILENAME_TO_LANG[basename];
  const dotIndex = basename.lastIndexOf(".");
  if (dotIndex === -1) return null;
  const ext = basename.slice(dotIndex + 1);
  return EXT_TO_LANG[ext] ?? null;
}

export function escapeHtml(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

export function highlightCode(code: string, filename: string): string {
  const lang = getLanguage(filename);
  if (!lang || !Prism.languages[lang]) {
    return escapeHtml(code);
  }
  return Prism.highlight(code, Prism.languages[lang], lang);
}

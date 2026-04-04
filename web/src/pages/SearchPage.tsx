import { useState, useEffect } from "react";
import { useSearchParams, Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { post } from "../lib/api";

interface SearchResultMeta {
  language?: string;
  stars?: number;
  status?: string;
  number?: number;
  repo?: string;
  owner?: string;
  file_path?: string;
  line_start?: number;
  sha?: string;
  date?: string;
}

interface SearchResult {
  type: string;
  id: string;
  title: string;
  snippet: string;
  url: string;
  score: number;
  meta: SearchResultMeta;
}

interface FacetEntry {
  value: string;
  count: number;
}

interface SearchResponse {
  results: SearchResult[];
  facets: {
    types: FacetEntry[];
    languages: FacetEntry[];
  };
  total: number;
}

const SCOPES = [
  { key: "", label: "All" },
  { key: "repo", label: "Repos" },
  { key: "issue", label: "Issues" },
  { key: "pull", label: "PRs" },
  { key: "code", label: "Code" },
  { key: "commit", label: "Commits" },
];

const LIMIT = 20;

export default function SearchPage() {
  const [searchParams] = useSearchParams();
  const query = searchParams.get("q") ?? "";
  const [scope, setScope] = useState("");
  const [offset, setOffset] = useState(0);
  const [accumulated, setAccumulated] = useState<SearchResult[]>([]);

  useEffect(() => {
    setOffset(0);
    setAccumulated([]);
  }, [query, scope]);

  const searchQuery = useQuery({
    queryKey: ["search", query, scope, offset],
    queryFn: async () => {
      const body: Record<string, unknown> = {
        query,
        limit: LIMIT,
        offset,
      };
      if (scope) {
        body.scope = scope;
      }
      const r = await post<SearchResponse>("/search", body);
      return r.data;
    },
    enabled: query.length > 0,
  });

  useEffect(() => {
    if (searchQuery.data) {
      if (offset === 0) {
        setAccumulated(searchQuery.data.results);
      } else {
        setAccumulated((prev) => [...prev, ...searchQuery.data!.results]);
      }
    }
  }, [searchQuery.data]); // eslint-disable-line react-hooks/exhaustive-deps

  const facets = searchQuery.data?.facets;
  const total = searchQuery.data?.total ?? 0;

  function getFacetCount(key: string): number | null {
    if (!facets) return null;
    if (key === "") return total;
    const entry = facets.types.find((f) => f.value === key);
    return entry?.count ?? 0;
  }

  if (!query) {
    return (
      <div className="search-page">
        <p className="muted">Enter a search query to get started.</p>
      </div>
    );
  }

  return (
    <div className="search-page">
      <div className="search-sidebar">
        <h3 className="search-sidebar-title">Scope</h3>
        <ul className="search-scope-list">
          {SCOPES.map((s) => {
            const count = getFacetCount(s.key);
            return (
              <li key={s.key}>
                <button
                  className={`search-scope-btn ${scope === s.key ? "active" : ""}`}
                  onClick={() => setScope(s.key)}
                >
                  <span>{s.label}</span>
                  {count !== null && (
                    <span className="search-scope-count">{count}</span>
                  )}
                </button>
              </li>
            );
          })}
        </ul>
        {facets && facets.languages.length > 0 && (
          <>
            <h3 className="search-sidebar-title">Languages</h3>
            <ul className="search-language-list">
              {facets.languages.map((lang) => (
                <li key={lang.value} className="search-language-item">
                  <span>{lang.value}</span>
                  <span className="search-scope-count">{lang.count}</span>
                </li>
              ))}
            </ul>
          </>
        )}
      </div>

      <div className="search-results">
        <p className="search-results-count muted">
          {searchQuery.isLoading
            ? "Searching..."
            : `${total} result${total !== 1 ? "s" : ""} for "${query}"`}
        </p>

        {searchQuery.error && (
          <div className="error-banner">
            {searchQuery.error instanceof Error
              ? searchQuery.error.message
              : "Search failed"}
          </div>
        )}

        {accumulated.map((result) => (
          <div key={`${result.type}-${result.id}`} className={`search-result search-result-${result.type}`}>
            {result.type === "repo" && <RepoResult result={result} />}
            {result.type === "issue" && <IssueResult result={result} />}
            {result.type === "pull" && <PullResult result={result} />}
            {result.type === "code" && <CodeResult result={result} />}
            {result.type === "commit" && <CommitResult result={result} />}
          </div>
        ))}

        {!searchQuery.isLoading &&
          accumulated.length > 0 &&
          accumulated.length < total && (
            <div style={{ textAlign: "center", padding: "16px 0" }}>
              <button
                className="btn btn-secondary"
                onClick={() => setOffset(accumulated.length)}
              >
                Load more
              </button>
            </div>
          )}

        {!searchQuery.isLoading && accumulated.length === 0 && query && (
          <p className="muted">No results found.</p>
        )}
      </div>
    </div>
  );
}

function RepoResult({ result }: { result: SearchResult }) {
  return (
    <>
      <div className="search-result-header">
        <Link to={result.url} className="search-result-title">
          {result.title}
        </Link>
      </div>
      {result.snippet && (
        <p className="search-result-desc">{result.snippet}</p>
      )}
      <div className="search-result-meta">
        {result.meta.language && <span>{result.meta.language}</span>}
        {result.meta.stars !== undefined && <span>{result.meta.stars} stars</span>}
      </div>
    </>
  );
}

function IssueResult({ result }: { result: SearchResult }) {
  return (
    <>
      <div className="search-result-header">
        <span
          className={`issue-status-dot ${result.meta.status === "open" ? "open" : "closed"}`}
        />
        <Link to={result.url} className="search-result-title">
          {result.title}
        </Link>
        {result.meta.number !== undefined && (
          <span className="search-result-number">#{result.meta.number}</span>
        )}
      </div>
      {result.meta.repo && (
        <div className="search-result-meta">
          <span>{result.meta.owner}/{result.meta.repo}</span>
        </div>
      )}
    </>
  );
}

function PullResult({ result }: { result: SearchResult }) {
  return (
    <>
      <div className="search-result-header">
        <span
          className={`pr-status-dot ${result.meta.status === "merged" ? "merged" : result.meta.status === "open" ? "open" : "closed"}`}
        />
        <Link to={result.url} className="search-result-title">
          {result.title}
        </Link>
        {result.meta.number !== undefined && (
          <span className="search-result-number">#{result.meta.number}</span>
        )}
      </div>
      {result.meta.repo && (
        <div className="search-result-meta">
          <span>{result.meta.owner}/{result.meta.repo}</span>
        </div>
      )}
    </>
  );
}

function CodeResult({ result }: { result: SearchResult }) {
  const lineStart = result.meta.line_start ?? 1;
  const lines = result.snippet.split("\n");

  return (
    <>
      <div className="search-result-header">
        <Link to={result.url} className="search-result-title search-result-filepath">
          {result.meta.file_path ?? result.title}
        </Link>
        {result.meta.language && (
          <span className="search-result-lang">{result.meta.language}</span>
        )}
      </div>
      {result.meta.repo && (
        <div className="search-result-meta">
          <span>{result.meta.owner}/{result.meta.repo}</span>
        </div>
      )}
      <div className="code-snippet">
        <table className="code-snippet-table">
          <tbody>
            {lines.map((line, i) => (
              <tr key={i} className="code-snippet-line">
                <td className="code-snippet-num">{lineStart + i}</td>
                <td className="code-snippet-content">{line}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}

function CommitResult({ result }: { result: SearchResult }) {
  const shortSha = result.meta.sha?.slice(0, 7) ?? "";

  return (
    <>
      <div className="search-result-header">
        {shortSha && <span className="commit-sha">{shortSha}</span>}
        <Link to={result.url} className="search-result-title">
          {result.title}
        </Link>
      </div>
      <div className="search-result-meta">
        {result.meta.repo && (
          <span>{result.meta.owner}/{result.meta.repo}</span>
        )}
        {result.meta.date && (
          <span>{new Date(result.meta.date).toLocaleDateString()}</span>
        )}
      </div>
    </>
  );
}

import { useState } from "react";
import { useParams, Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { get } from "../lib/api";
import RepoHeader from "../components/RepoHeader";
import Markdown from "../components/Markdown";

interface AgentTask {
  id: string;
  trigger_event: string;
  status: "queued" | "running" | "completed" | "failed";
  duration_ms: number;
  created_at: string;
  completed_at: string | null;
  result: Record<string, unknown>;
  error: string | null;
}

interface AgentDoc {
  id: string;
  title: string;
  content: string;
  doc_type: string;
  updated_at: string;
}

interface ChatConversation {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
  message_count?: number;
}

interface RepoAgent {
  id: string;
  agent_id: string;
  agent_name: string;
  agent_slug: string;
  enabled: boolean;
}

type AgentsSection = "activity" | "documents" | "conversations";

function statusColor(status: string): string {
  switch (status) {
    case "completed":
      return "status-completed";
    case "running":
      return "status-running";
    case "failed":
      return "status-failed";
    case "queued":
    default:
      return "status-queued";
  }
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.floor(ms / 60000)}m ${Math.round((ms % 60000) / 1000)}s`;
}

function docTypeLabel(docType: string): string {
  const labels: Record<string, string> = {
    architecture: "Architecture",
    components: "Components",
    api: "API",
    dependencies: "Dependencies",
    conventions: "Conventions",
    onboarding: "Onboarding",
  };
  return labels[docType] || docType;
}

export default function AgentsTab() {
  const { owner, repo } = useParams();
  const [section, setSection] = useState<AgentsSection>("activity");
  const [expandedDoc, setExpandedDoc] = useState<string | null>(null);

  const agentsQuery = useQuery({
    queryKey: ["repo-agents", owner, repo],
    queryFn: () =>
      get<RepoAgent[]>(`/repos/${owner}/${repo}/agents`).then((r) => r.data),
    enabled: !!owner && !!repo,
  });

  const tasksQuery = useQuery({
    queryKey: ["repo-tasks", owner, repo],
    queryFn: () =>
      get<AgentTask[]>(`/repos/${owner}/${repo}/tasks`).then((r) => r.data),
    enabled: !!owner && !!repo && section === "activity",
  });

  const docsQuery = useQuery({
    queryKey: ["repo-docs", owner, repo],
    queryFn: () =>
      get<AgentDoc[]>(`/repos/${owner}/${repo}/docs`).then((r) => r.data),
    enabled: !!owner && !!repo && section === "documents",
  });

  const chatsQuery = useQuery({
    queryKey: ["repo-chats", owner, repo],
    queryFn: () =>
      get<ChatConversation[]>(`/repos/${owner}/${repo}/chat`).then(
        (r) => r.data,
      ),
    enabled: !!owner && !!repo && section === "conversations",
  });

  const hasAgents =
    agentsQuery.data !== undefined && agentsQuery.data.length > 0;
  const isLoading = agentsQuery.isLoading;

  return (
    <div className="repo-page">
      <RepoHeader owner={owner!} repo={repo!} activeTab="agents" />

      {isLoading && <p className="muted">Loading agents...</p>}

      {agentsQuery.error && (
        <div className="error-banner">
          {agentsQuery.error instanceof Error
            ? agentsQuery.error.message
            : "Failed to load agents"}
        </div>
      )}

      {!isLoading && !hasAgents && (
        <div className="agent-empty-state">
          <svg
            width="48"
            height="48"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d="M12 8V4H8" />
            <rect x="2" y="2" width="20" height="20" rx="5" />
            <path d="M2 12h4" />
            <path d="M18 12h4" />
            <circle cx="12" cy="12" r="2" />
          </svg>
          <h3>No agents configured</h3>
          <p className="muted">
            Enable agents in Settings to get AI-powered code review,
            documentation, and more.
          </p>
          <Link
            to={`/${owner}/${repo}/settings`}
            className="btn btn-primary"
          >
            Go to Settings
          </Link>
        </div>
      )}

      {!isLoading && hasAgents && (
        <>
          <div className="agents-section-nav">
            <button
              className={`agents-section-btn ${section === "activity" ? "active" : ""}`}
              onClick={() => setSection("activity")}
            >
              Activity
            </button>
            <button
              className={`agents-section-btn ${section === "documents" ? "active" : ""}`}
              onClick={() => setSection("documents")}
            >
              Documents
            </button>
            <button
              className={`agents-section-btn ${section === "conversations" ? "active" : ""}`}
              onClick={() => setSection("conversations")}
            >
              Conversations
            </button>
          </div>

          {section === "activity" && (
            <div className="agents-activity">
              {tasksQuery.isLoading && (
                <p className="muted">Loading activity...</p>
              )}
              {tasksQuery.error && (
                <div className="error-banner">
                  {tasksQuery.error instanceof Error
                    ? tasksQuery.error.message
                    : "Failed to load tasks"}
                </div>
              )}
              {tasksQuery.data && tasksQuery.data.length === 0 && (
                <p className="muted">No agent activity yet.</p>
              )}
              {tasksQuery.data && tasksQuery.data.length > 0 && (
                <ul className="agent-activity-list">
                  {tasksQuery.data.map((task) => (
                    <li key={task.id} className="agent-activity-item">
                      <div className="agent-activity-row">
                        <span
                          className={`agent-status-badge ${statusColor(task.status)}`}
                        >
                          {task.status}
                        </span>
                        <span className="agent-activity-trigger">
                          {task.trigger_event}
                        </span>
                        {task.duration_ms > 0 && (
                          <span className="agent-activity-duration">
                            {formatDuration(task.duration_ms)}
                          </span>
                        )}
                        <span className="agent-activity-time">
                          {new Date(task.created_at).toLocaleDateString()}{" "}
                          {new Date(task.created_at).toLocaleTimeString()}
                        </span>
                      </div>
                      {task.error && (
                        <div className="agent-activity-error">
                          {task.error}
                        </div>
                      )}
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}

          {section === "documents" && (
            <div className="agents-documents">
              {docsQuery.isLoading && (
                <p className="muted">Loading documents...</p>
              )}
              {docsQuery.error && (
                <div className="error-banner">
                  {docsQuery.error instanceof Error
                    ? docsQuery.error.message
                    : "Failed to load documents"}
                </div>
              )}
              {docsQuery.data && docsQuery.data.length === 0 && (
                <p className="muted">No documents generated yet.</p>
              )}
              {docsQuery.data && docsQuery.data.length > 0 && (
                <ul className="agent-doc-list">
                  {docsQuery.data.map((doc) => (
                    <li key={doc.id} className="agent-doc-card">
                      <button
                        className="agent-doc-header"
                        onClick={() =>
                          setExpandedDoc(
                            expandedDoc === doc.id ? null : doc.id,
                          )
                        }
                      >
                        <div className="agent-doc-title-row">
                          <span className="agent-doc-title">{doc.title}</span>
                          <span
                            className={`agent-doc-type doc-type-${doc.doc_type}`}
                          >
                            {docTypeLabel(doc.doc_type)}
                          </span>
                        </div>
                        <span className="agent-doc-updated">
                          Updated{" "}
                          {new Date(doc.updated_at).toLocaleDateString()}
                        </span>
                      </button>
                      {expandedDoc === doc.id && (
                        <div className="agent-doc-content">
                          <Markdown content={doc.content} />
                        </div>
                      )}
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}

          {section === "conversations" && (
            <div className="agents-conversations">
              {chatsQuery.isLoading && (
                <p className="muted">Loading conversations...</p>
              )}
              {chatsQuery.error && (
                <div className="error-banner">
                  {chatsQuery.error instanceof Error
                    ? chatsQuery.error.message
                    : "Failed to load conversations"}
                </div>
              )}
              {chatsQuery.data && chatsQuery.data.length === 0 && (
                <p className="muted">No conversations yet.</p>
              )}
              {chatsQuery.data && chatsQuery.data.length > 0 && (
                <ul className="agent-conversation-list">
                  {chatsQuery.data.map((conv) => (
                    <li key={conv.id} className="agent-conversation-item">
                      <div className="agent-conversation-title">
                        {conv.title || "Untitled conversation"}
                      </div>
                      <div className="agent-conversation-meta">
                        <span>
                          {new Date(conv.updated_at).toLocaleDateString()}
                        </span>
                        {conv.message_count !== undefined && (
                          <span>
                            {conv.message_count} message
                            {conv.message_count !== 1 ? "s" : ""}
                          </span>
                        )}
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </>
      )}
    </div>
  );
}

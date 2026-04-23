import { useState, useRef, useEffect, useCallback } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { get, post, del } from "../lib/api";
import { useAuthStore } from "../stores/auth";
import Markdown from "./Markdown";

const API_BASE = "/api/v1";

interface ChatConversation {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
}

interface ChatMessage {
  id: string;
  role: "user" | "assistant";
  content: string;
  created_at: string;
}

interface ChatConversationDetail {
  id: string;
  title: string;
  messages: ChatMessage[];
}

interface ChatPanelProps {
  owner: string;
  repo: string;
}

export default function ChatPanel({ owner, repo }: ChatPanelProps) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const queryClient = useQueryClient();
  const [isOpen, setIsOpen] = useState(false);
  const [activeConversation, setActiveConversation] = useState<string | null>(
    null,
  );
  const [messageInput, setMessageInput] = useState("");
  const [showConversationList, setShowConversationList] = useState(false);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const agentsQuery = useQuery({
    queryKey: ["repo-agents", owner, repo],
    queryFn: () =>
      get<{ id: string }[]>(`/repos/${owner}/${repo}/agents`).then(
        (r) => r.data,
      ),
    enabled: !!owner && !!repo && isAuthenticated,
  });

  const hasAgents =
    agentsQuery.data !== undefined && agentsQuery.data.length > 0;

  const conversationsQuery = useQuery({
    queryKey: ["chat-conversations", owner, repo],
    queryFn: () =>
      get<ChatConversation[]>(`/repos/${owner}/${repo}/chat`).then(
        (r) => r.data,
      ),
    enabled: !!owner && !!repo && isOpen,
  });

  const conversationQuery = useQuery({
    queryKey: ["chat-conversation", owner, repo, activeConversation],
    queryFn: () =>
      get<ChatConversationDetail>(
        `/repos/${owner}/${repo}/chat/${activeConversation}`,
      ).then((r) => r.data),
    enabled: !!owner && !!repo && !!activeConversation,
  });

  const createConversation = useMutation({
    mutationFn: () =>
      post<ChatConversation>(`/repos/${owner}/${repo}/chat`, {}).then(
        (r) => r.data,
      ),
    onSuccess: (data) => {
      setActiveConversation(data.id);
      setShowConversationList(false);
      queryClient.invalidateQueries({
        queryKey: ["chat-conversations", owner, repo],
      });
    },
  });

  const [isSending, setIsSending] = useState(false);
  const [streamingContent, setStreamingContent] = useState("");
  const [sendError, setSendError] = useState<string | null>(null);
  // Hold the optimistic user message to display immediately
  const [optimisticUserMsg, setOptimisticUserMsg] = useState<ChatMessage | null>(null);
  // AbortController for the in-flight streaming fetch
  const abortControllerRef = useRef<AbortController | null>(null);

  const deleteConversation = useMutation({
    mutationFn: (convId: string) =>
      del(`/repos/${owner}/${repo}/chat/${convId}`),
    onSuccess: (_data, convId) => {
      if (activeConversation === convId) {
        setActiveConversation(null);
      }
      setConfirmDeleteId(null);
      queryClient.invalidateQueries({
        queryKey: ["chat-conversations", owner, repo],
      });
    },
  });

  const handleDeleteClick = (convId: string) => {
    if (confirmDeleteId === convId) {
      deleteConversation.mutate(convId);
    } else {
      setConfirmDeleteId(convId);
    }
  };

  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, []);

  useEffect(() => {
    scrollToBottom();
  }, [conversationQuery.data, streamingContent, scrollToBottom]);

  // Abort any in-flight stream on unmount
  useEffect(() => {
    return () => {
      abortControllerRef.current?.abort();
    };
  }, []);

  const handleSend = async () => {
    const content = messageInput.trim();
    if (!content || !activeConversation || isSending) return;

    // Abort any previous in-flight request and create a fresh controller
    abortControllerRef.current?.abort();
    const controller = new AbortController();
    abortControllerRef.current = controller;
    const { signal } = controller;

    setMessageInput("");
    setIsSending(true);
    setStreamingContent("");
    setSendError(null);
    setOptimisticUserMsg(null);

    try {
      const resp = await fetch(
        `${API_BASE}/repos/${owner}/${repo}/chat/${activeConversation}/messages`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ content }),
          credentials: "include",
          signal,
        },
      );

      if (!resp.ok || !resp.body) {
        if (!signal.aborted) setSendError(`Request failed with status ${resp.status}`);
        return;
      }

      const reader = resp.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        if (signal.aborted) return;

        buffer += decoder.decode(value, { stream: true });

        // Parse SSE events from buffer
        const events = buffer.split("\n\n");
        buffer = events.pop()!; // Keep incomplete event in buffer

        for (const event of events) {
          if (signal.aborted) return;
          if (!event.trim()) continue;
          const lines = event.split("\n");
          let eventType = "";
          let data = "";

          for (const line of lines) {
            if (line.startsWith("event: ")) eventType = line.slice(7);
            if (line.startsWith("data: ")) data = line.slice(6);
          }

          if (eventType === "user_message" && data) {
            try {
              const msg = JSON.parse(data) as ChatMessage;
              setOptimisticUserMsg(msg);
            } catch (err) {
              console.warn("[ChatPanel] Failed to parse user_message chunk:", err);
            }
          } else if (eventType === "chunk" && data) {
            try {
              const chunk = JSON.parse(data) as { content: string };
              setStreamingContent((prev) => prev + chunk.content);
            } catch (err) {
              console.warn("[ChatPanel] Failed to parse chunk:", err);
            }
          } else if (eventType === "tool_call" && data) {
            try {
              const tc = JSON.parse(data) as { files?: string[] };
              const names = tc.files?.join(", ") || "files";
              setStreamingContent((prev) => prev + `\n\n*Reading ${names}...*\n\n`);
            } catch (err) {
              console.warn("[ChatPanel] Failed to parse tool_call chunk:", err);
            }
          } else if (eventType === "clear_stream") {
            setStreamingContent("");
          } else if (eventType === "assistant_message") {
            // Final saved message — we clear streaming content; the
            // invalidation below will fetch it from the DB.
            setStreamingContent("");
            setOptimisticUserMsg(null);
          } else if (eventType === "error" && data) {
            try {
              const errData = JSON.parse(data) as { message: string };
              setSendError(errData.message);
            } catch (err) {
              console.warn("[ChatPanel] Failed to parse error chunk:", err);
            }
          } else if (eventType === "done") {
            break;
          }
        }
      }
    } catch (e) {
      if (!signal.aborted) {
        setSendError(e instanceof Error ? e.message : "Failed to send message");
      }
    } finally {
      if (!signal.aborted) {
        setIsSending(false);
        setStreamingContent("");
        setOptimisticUserMsg(null);
        // Refresh conversation to get the saved messages from DB
        queryClient.invalidateQueries({
          queryKey: ["chat-conversation", owner, repo, activeConversation],
        });
        queryClient.invalidateQueries({
          queryKey: ["chat-conversations", owner, repo],
        });
      }
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  const handleNewConversation = () => {
    createConversation.mutate();
  };

  if (!isAuthenticated || !hasAgents) return null;

  const messages = conversationQuery.data?.messages ?? [];

  return (
    <>
      <button
        className="chat-button"
        onClick={() => setIsOpen(true)}
        aria-label="Open chat"
      >
        <svg
          width="24"
          height="24"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
        </svg>
      </button>

      {isOpen && (
        <>
          <div className="chat-panel-overlay" onClick={() => setIsOpen(false)} />
          <div className="chat-panel">
            <div className="chat-panel-header">
              <h3>Chat with Gitwise</h3>
              <div className="chat-panel-header-actions">
                <button
                  className="chat-panel-btn"
                  onClick={() =>
                    setShowConversationList(!showConversationList)
                  }
                  title="Conversation list"
                >
                  <svg
                    width="16"
                    height="16"
                    viewBox="0 0 16 16"
                    fill="currentColor"
                  >
                    <path d="M1 2.75A.75.75 0 0 1 1.75 2h12.5a.75.75 0 0 1 0 1.5H1.75A.75.75 0 0 1 1 2.75Zm0 5A.75.75 0 0 1 1.75 7h12.5a.75.75 0 0 1 0 1.5H1.75A.75.75 0 0 1 1 7.75ZM1.75 12h12.5a.75.75 0 0 1 0 1.5H1.75a.75.75 0 0 1 0-1.5Z" />
                  </svg>
                </button>
                <button
                  className="chat-panel-btn"
                  onClick={handleNewConversation}
                  disabled={createConversation.isPending}
                  title="New conversation"
                >
                  <svg
                    width="16"
                    height="16"
                    viewBox="0 0 16 16"
                    fill="currentColor"
                  >
                    <path d="M7.75 2a.75.75 0 0 1 .75.75V7h4.25a.75.75 0 0 1 0 1.5H8.5v4.25a.75.75 0 0 1-1.5 0V8.5H2.75a.75.75 0 0 1 0-1.5H7V2.75A.75.75 0 0 1 7.75 2Z" />
                  </svg>
                </button>
                <button
                  className="chat-panel-btn"
                  onClick={() => setIsOpen(false)}
                  title="Close"
                >
                  <svg
                    width="16"
                    height="16"
                    viewBox="0 0 16 16"
                    fill="currentColor"
                  >
                    <path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.75.75 0 1 1 1.06 1.06L9.06 8l3.22 3.22a.75.75 0 1 1-1.06 1.06L8 9.06l-3.22 3.22a.75.75 0 0 1-1.06-1.06L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z" />
                  </svg>
                </button>
              </div>
            </div>

            {showConversationList && (
              <div className="chat-conversation-list">
                {conversationsQuery.isLoading && (
                  <p className="muted" style={{ padding: "8px 12px" }}>
                    Loading...
                  </p>
                )}
                {conversationsQuery.data &&
                  conversationsQuery.data.length === 0 && (
                    <p className="muted" style={{ padding: "8px 12px" }}>
                      No conversations yet.
                    </p>
                  )}
                {conversationsQuery.data?.map((conv) => (
                  <div
                    key={conv.id}
                    className={`chat-conversation-entry ${activeConversation === conv.id ? "active" : ""}`}
                  >
                    <button
                      className="chat-conversation-entry-main"
                      onClick={() => setActiveConversation(conv.id)}
                    >
                      <span className="chat-conversation-entry-title">
                        {conv.title || "Untitled"}
                      </span>
                      {activeConversation === conv.id && (
                        <span className="chat-conversation-active-badge">Active</span>
                      )}
                      <span className="chat-conversation-entry-date">
                        {new Date(conv.updated_at).toLocaleDateString()}
                      </span>
                    </button>
                    <button
                      className={`chat-conversation-delete-btn ${confirmDeleteId === conv.id ? "confirm" : ""}`}
                      onClick={(e) => {
                        e.stopPropagation();
                        handleDeleteClick(conv.id);
                      }}
                      onBlur={() => setConfirmDeleteId(null)}
                      disabled={deleteConversation.isPending}
                    >
                      {confirmDeleteId === conv.id ? "Sure?" : "\u00d7"}
                    </button>
                  </div>
                ))}
              </div>
            )}

            <div className="chat-messages">
              {!activeConversation && (
                <div className="chat-empty-state">
                  <p className="muted">
                    Start a new conversation to chat with the Gitwise agent
                    about this repository.
                  </p>
                  <button
                    className="btn btn-primary btn-sm"
                    onClick={handleNewConversation}
                    disabled={createConversation.isPending}
                  >
                    New Conversation
                  </button>
                </div>
              )}

              {activeConversation && conversationQuery.isLoading && (
                <p className="muted" style={{ padding: 16 }}>
                  Loading messages...
                </p>
              )}

              {activeConversation && messages.length === 0 && !conversationQuery.isLoading && (
                <div className="chat-empty-state">
                  <p className="muted">
                    Ask anything about this repository.
                  </p>
                </div>
              )}

              {messages.map((msg) => (
                <div
                  key={msg.id}
                  className={`chat-message ${msg.role === "user" ? "chat-message-user" : "chat-message-assistant"}`}
                >
                  <div className="chat-message-content">
                    <Markdown content={msg.content} />
                  </div>
                </div>
              ))}

              {optimisticUserMsg && (
                <div className="chat-message chat-message-user">
                  <div className="chat-message-content">
                    <Markdown content={optimisticUserMsg.content} />
                  </div>
                </div>
              )}

              {streamingContent && (
                <div className="chat-message chat-message-assistant">
                  <div className="chat-message-content">
                    <Markdown content={streamingContent} />
                  </div>
                </div>
              )}

              {isSending && !streamingContent && (
                <div className="chat-message chat-message-assistant">
                  <div className="chat-typing">
                    <span></span>
                    <span></span>
                    <span></span>
                  </div>
                </div>
              )}

              <div ref={messagesEndRef} />
            </div>

            {activeConversation && (
              <div className="chat-input-area">
                {sendError && (
                  <div className="chat-error">
                    {sendError}
                  </div>
                )}
                <div className="chat-input">
                  <textarea
                    ref={textareaRef}
                    value={messageInput}
                    onChange={(e) => setMessageInput(e.target.value)}
                    onKeyDown={handleKeyDown}
                    placeholder="Ask about this repository..."
                    rows={2}
                    disabled={isSending}
                  />
                  <button
                    className="chat-send-btn"
                    onClick={handleSend}
                    disabled={
                      !messageInput.trim() || isSending
                    }
                    title="Send message"
                  >
                    <svg
                      width="16"
                      height="16"
                      viewBox="0 0 16 16"
                      fill="currentColor"
                    >
                      <path d="M.989 8 .064 2.68a1.342 1.342 0 0 1 1.85-1.462l13.402 5.744a1.13 1.13 0 0 1 0 2.076L1.913 14.782a1.343 1.343 0 0 1-1.85-1.463L.99 8Zm.603-5.074L2.2 7.25h4.55a.75.75 0 0 1 0 1.5H2.2l-.608 4.324L13.254 8 1.592 2.926Z" />
                    </svg>
                  </button>
                </div>
              </div>
            )}
          </div>
        </>
      )}
    </>
  );
}

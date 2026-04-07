# Gitwise V2 — AI Layer Specification

**An AI-Native Code Collaboration Platform — Intelligence Release**

Version 2.0 DRAFT — April 2026 | CONFIDENTIAL

---

## 1. Executive Summary

V1 shipped a fully functional git hosting platform with an AI-native data model — embeddings on every text field, structured metadata, full edit history, typed entity graphs, and a provider-agnostic embedding pipeline. V2 activates that infrastructure.

The headline feature is the **Gitwise Agent** — an autonomous AI team member that lives in your repository. It reads your code, maintains living documentation, reviews every push, and opens issues and PRs when it finds problems or opportunities. Users interact with it through a chat interface and see its work throughout the repo alongside human contributions.

V2 also introduces a **chat assistant** for every repo, **semantic search powered by real understanding** (not just keyword matching), and an **agent framework** that supports both the official Gitwise Agent and user-defined custom agents.

---

## 2. Design Principles

### 2.1 Anthropic-First, Ollama-Supported

Claude is the first-class AI provider. The Anthropic API delivers the quality needed for code review, documentation generation, and agentic reasoning. Ollama is supported for users who need local-only operation or want to experiment with open models.

**Provider tiers:**

| Provider | Parallel Agents | Quality | Privacy | Use Case |
|----------|----------------|---------|---------|----------|
| Anthropic API | Yes (concurrent) | Highest | Code sent to Anthropic | Teams that want the best results |
| Ollama Cloud | Yes (concurrent) | Varies by model | Code sent to Ollama Cloud | Teams wanting cloud speed with open models |
| Ollama Local | No (sequential queue) | Varies by model | Code never leaves server | Maximum privacy, smaller projects |

**Critical constraint:** Local Ollama cannot run parallel requests — it kills concurrent jobs. The agent queue must enforce sequential execution for local Ollama while allowing parallelism for Anthropic and Ollama Cloud.

### 2.2 Privacy Model

- **Default:** Your code never leaves your server. Embeddings and local Ollama keep everything on-premises.
- **Opt-in:** When a user configures an Anthropic API key, code is sent to Anthropic for processing. This is explicit and per-instance — Gitwise never phones home.
- **No OpenAI.** Anthropic is the only commercial provider. This is a deliberate positioning choice.

### 2.3 Agents as Team Members

Agents are not tools you invoke — they are team members that participate. An agent's review appears in the PR review thread alongside human reviews. An agent's issues appear in the issue list. An agent's PRs appear in the PR list. The only difference is a `[bot]` tag.

---

## 3. Architecture Overview

### 3.1 New Components

| Component | Technology | Purpose |
|-----------|-----------|---------|
| Agent Runtime | Go + goroutine pool | Executes agent tasks (review, doc gen, analysis) |
| Agent Queue | Redis streams | Ordered task queue with provider-aware concurrency |
| LLM Gateway | Go service | Provider abstraction for Anthropic/Ollama, handles routing, retries, rate limits |
| Document Store | PostgreSQL + git | Agent-generated docs stored as markdown in a special branch or DB |
| Chat Backend | WebSocket + LLM Gateway | Streaming chat responses with repo context |
| Context Builder | Go | Assembles relevant context (code, docs, issues, PRs) for LLM prompts |

### 3.2 Provider Abstraction — LLM Gateway

```
internal/services/llm/
  gateway.go        — Provider router, concurrency control, retry logic
  anthropic.go      — Claude API client (Messages API, streaming)
  ollama_gen.go     — Ollama generate/chat endpoint client
  queue.go          — Redis-backed task queue with provider-aware parallelism
  context.go        — Context window management, truncation, prioritization
```

The gateway handles:
- **Routing:** Directs requests to the configured provider
- **Concurrency:** Unlimited for Anthropic, unlimited for Ollama Cloud, sequential for Ollama Local
- **Queuing:** Redis streams with priority levels (user-initiated chat > push-triggered review > background doc gen)
- **Streaming:** SSE/WebSocket streaming for chat responses
- **Cost tracking:** Token usage logged per repo, per agent, per user

### 3.3 Data Model Extensions

New tables:

```sql
-- Agent definitions (built-in + custom)
CREATE TABLE agents (
    id UUID PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    slug VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    is_official BOOLEAN NOT NULL DEFAULT false,
    author_id UUID REFERENCES users(id),
    config JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Agent installations on repos
CREATE TABLE repo_agents (
    id UUID PRIMARY KEY,
    repo_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT true,
    config JSONB NOT NULL DEFAULT '{}',     -- per-repo overrides
    instructions TEXT DEFAULT '',            -- custom instructions (like CLAUDE.md)
    trigger_events TEXT[] DEFAULT '{push}',  -- which events trigger the agent
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (repo_id, agent_id)
);

-- Agent-generated documents (living knowledge base)
CREATE TABLE agent_documents (
    id UUID PRIMARY KEY,
    repo_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    title VARCHAR(500) NOT NULL,
    content TEXT NOT NULL,
    content_embedding VECTOR(1536),
    doc_type VARCHAR(50) NOT NULL,          -- architecture, component, api, dependency, etc.
    metadata JSONB DEFAULT '{}',
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Agent task log
CREATE TABLE agent_tasks (
    id UUID PRIMARY KEY,
    repo_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id),
    trigger_event VARCHAR(50) NOT NULL,     -- push, schedule, manual, chat
    trigger_ref TEXT,                        -- branch/commit that triggered it
    status VARCHAR(20) NOT NULL DEFAULT 'queued', -- queued, running, completed, failed
    provider VARCHAR(20) NOT NULL,          -- anthropic, ollama_cloud, ollama_local
    input_tokens INT DEFAULT 0,
    output_tokens INT DEFAULT 0,
    duration_ms INT DEFAULT 0,
    result JSONB DEFAULT '{}',              -- summary of actions taken
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

-- Bot user account for agent actions
-- Uses existing users table with is_bot = true flag
ALTER TABLE users ADD COLUMN is_bot BOOLEAN NOT NULL DEFAULT false;

-- Chat conversations
CREATE TABLE chat_conversations (
    id UUID PRIMARY KEY,
    repo_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE chat_messages (
    id UUID PRIMARY KEY,
    conversation_id UUID NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL,              -- user, assistant
    content TEXT NOT NULL,
    content_embedding VECTOR(1536),
    metadata JSONB DEFAULT '{}',            -- token counts, model used, context refs
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 3.4 Configuration

New environment variables:

```
# LLM Provider
GITWISE_LLM_PROVIDER=anthropic           # anthropic, ollama_cloud, ollama_local, none
GITWISE_ANTHROPIC_API_KEY=sk-ant-...     # Anthropic API key
GITWISE_ANTHROPIC_MODEL=claude-sonnet-4-6 # Default model for agent tasks
GITWISE_ANTHROPIC_CHAT_MODEL=claude-sonnet-4-6  # Model for chat (can differ)

# Ollama
GITWISE_OLLAMA_CLOUD_URL=                # Ollama Cloud endpoint (if using cloud)
GITWISE_OLLAMA_LOCAL_URL=http://localhost:11434
GITWISE_OLLAMA_GEN_MODEL=llama3          # Model for generative tasks

# Agent Runtime
GITWISE_AGENT_QUEUE_WORKERS=4            # Max concurrent agent tasks (Anthropic/Cloud)
GITWISE_AGENT_MAX_CONTEXT=100000         # Max tokens for context assembly
GITWISE_AGENT_ENABLED=true               # Global kill switch
```

---

## 4. The Gitwise Agent

The Gitwise Agent is the official first-party agent. It ships with Gitwise and is pre-configured for intelligent code analysis. Users enable it on their repos via Settings → Agents.

### 4.1 What It Does

**On every push:**
1. Reads the diff (changed files, commit messages)
2. Builds context from agent-generated docs + relevant code
3. Performs code review — looks for bugs, security issues, performance problems, convention drift
4. Updates living documentation if the push changed architecture or APIs
5. Opens issues for problems it finds (with severity labels)
6. Optionally opens PRs for fixes it can make autonomously (configurable)

**On schedule (configurable, e.g., weekly):**
1. Full codebase analysis — architecture review, dependency audit, tech debt assessment
2. Regenerates documentation from scratch (ensures accuracy)
3. Opens summary issue with findings

**On demand (via chat or manual trigger):**
1. User asks a question → agent answers with repo context
2. User requests a review → agent reviews specific files/PRs
3. User asks for documentation → agent generates it

### 4.2 Living Documentation

The agent maintains a set of documents per repo:

| Document Type | Content | Updated |
|--------------|---------|---------|
| `architecture` | High-level overview: what this project does, how it's structured, key design decisions | On significant structural changes |
| `components` | Per-directory/module summaries: purpose, public API, dependencies | When relevant files change |
| `api` | API endpoint documentation (if applicable) | When route/handler files change |
| `dependencies` | Dependency analysis: what's used, why, version status | When dependency files change |
| `conventions` | Coding patterns, naming conventions, test patterns detected | Periodically |
| `onboarding` | "Start here" guide for new contributors | Periodically |

These documents are:
- Stored in `agent_documents` table with embeddings
- Used as context for chat and code review
- Viewable in the repo's Agents tab
- Updated incrementally (agent sees what changed, updates only affected docs)

### 4.3 Agent Identity

The Gitwise Agent operates as a bot user account:
- Username: `gitwise-bot` (or `gitwise-agent`)
- `is_bot = true` flag on the users table
- `[bot]` badge displayed next to username in UI
- Issues/PRs/reviews created by the agent show this bot as the author
- The bot account is created automatically on first agent activation

### 4.4 Agent Configuration (per-repo)

Users configure the agent in **Repo Settings → Agents**:

```json
{
  "trigger_events": ["push"],
  "review_on_push": true,
  "open_issues": true,
  "open_prs": false,
  "auto_update_docs": true,
  "schedule": "weekly",
  "severity_threshold": "medium",
  "ignore_patterns": ["*.test.ts", "vendor/**"],
  "custom_instructions": "This is a Go backend. Focus on security and performance. We use slog for logging."
}
```

The `custom_instructions` field is the repo's equivalent of CLAUDE.md — free-text instructions that are prepended to every agent prompt.

---

## 5. Chat Assistant

Every repo gets a chat interface. The assistant has full context: the repo's code, agent-generated docs, issues, PRs, reviews, and commit history.

### 5.1 User Experience

- **Access:** Floating chat button on repo pages (bottom-right), opens a slide-out panel
- **Conversations:** Persistent, listed in the Agents tab. Users can have multiple conversations per repo.
- **Streaming:** Responses stream in real-time via WebSocket
- **Context:** The assistant automatically pulls relevant context based on the question — if you ask about authentication, it finds auth-related files and docs
- **Actions:** The assistant can read code, search issues, reference PRs. Future: open issues, create PRs (with user confirmation)

### 5.2 Context Assembly

When a user sends a message, the context builder assembles:

1. **Agent docs** — relevant documents from `agent_documents` (semantic search by message embedding)
2. **Code** — relevant files from the repo (semantic search over code index + file path matching)
3. **Issues/PRs** — relevant discussions (semantic search over issue/PR embeddings)
4. **Conversation history** — previous messages in this conversation
5. **Repo metadata** — description, language stats, structure overview

The context builder respects the model's context window, prioritizing by relevance score. Agent-generated docs are prioritized because they're curated summaries rather than raw code.

### 5.3 Chat API

```
POST /api/v1/repos/{owner}/{repo}/chat              — start new conversation
GET  /api/v1/repos/{owner}/{repo}/chat               — list conversations
GET  /api/v1/repos/{owner}/{repo}/chat/{id}          — get conversation + messages
POST /api/v1/repos/{owner}/{repo}/chat/{id}/messages — send message (returns streaming)
DELETE /api/v1/repos/{owner}/{repo}/chat/{id}        — delete conversation
```

Message streaming via WebSocket or SSE on the messages endpoint.

---

## 6. Agent Framework

While the Gitwise Agent is the flagship, the system supports custom agents.

### 6.1 Agent Types

| Type | Description | Example |
|------|------------|---------|
| **Official** | Ships with Gitwise, maintained by the project | Gitwise Agent |
| **Custom** | User-defined agents with custom prompts and triggers | "Security Scanner", "Changelog Generator" |

### 6.2 Custom Agent Definition

Users can create custom agents with:
- **Name and description**
- **System prompt** — the agent's personality and instructions
- **Trigger events** — push, schedule, manual
- **Actions** — what the agent is allowed to do (review, open issues, open PRs, update docs)
- **File filters** — which files the agent should pay attention to

Custom agents are defined per-user and can be installed on any repo the user owns.

### 6.3 Agent Marketplace (Future)

> **Deferred to V3.** V2 ships official + custom agents only. A public marketplace where users share agent configurations requires trust/moderation infrastructure.

---

## 7. Agent Queue & Runtime

### 7.1 Queue Architecture

```
Redis Stream: gitwise:agent:tasks

Task schema:
{
  "id": "uuid",
  "repo_id": "uuid",
  "agent_id": "uuid",
  "provider": "anthropic|ollama_cloud|ollama_local",
  "priority": 1-3,          // 1=chat (interactive), 2=push (near-realtime), 3=schedule (background)
  "trigger": "push|schedule|manual|chat",
  "payload": { ... },
  "created_at": "timestamp"
}
```

### 7.2 Concurrency Control

| Provider | Max Concurrent | Queue Behavior |
|----------|---------------|----------------|
| Anthropic | `GITWISE_AGENT_QUEUE_WORKERS` (default 4) | Parallel execution |
| Ollama Cloud | `GITWISE_AGENT_QUEUE_WORKERS` (default 4) | Parallel execution |
| Ollama Local | **1** (hardcoded) | Strictly sequential — one task at a time |

The queue consumer checks the provider before dispatching:
- If provider is `ollama_local`, acquire a global mutex before execution
- If provider is `anthropic` or `ollama_cloud`, dispatch to the worker pool

### 7.3 Priority

Interactive tasks (chat) always jump the queue. Push-triggered reviews are next. Scheduled background tasks (full doc regen, weekly audit) are lowest priority.

---

## 8. Semantic Search Enhancement

V1 has the infrastructure. V2 makes it useful.

### 8.1 Natural Language Search

The search endpoint gains a `mode=semantic` option that:
1. Takes a natural language query ("that PR where we fixed the login timeout last month")
2. Embeds the query
3. Searches across all entity embeddings (issues, PRs, commits, code, agent docs)
4. Returns results ranked by semantic similarity blended with recency
5. Includes agent-generated docs in search results (often the most useful hits)

### 8.2 "Ask" Search Mode

A new `mode=ask` option that:
1. Takes a question ("how does authentication work in this repo?")
2. Searches for relevant context (same as chat context assembly)
3. Generates a synthesized answer using the LLM
4. Returns the answer with citations (links to files, issues, PRs)

This is essentially a one-shot chat without conversation persistence.

---

## 9. Frontend Changes

### 9.1 Agents Tab (Repo Page)

A new tab alongside Code, Issues, PRs, Commits:

**When no agents are installed:**
- Empty state: "No agents configured. Enable agents in Settings to get AI-powered code review, documentation, and more."
- Link to Settings → Agents

**When agents are active:**
- **Activity feed** — recent agent actions: reviews submitted, issues opened, docs updated
- **Documents** — browsable list of agent-generated docs (architecture, components, API, etc.)
- **Tasks** — recent task log: trigger, status, duration, token usage
- **Chat conversations** — list of user conversations with the assistant

### 9.2 Chat Panel

- Floating button (bottom-right) on all repo pages
- Opens a slide-out panel (right side, ~400px wide)
- Message input at bottom, conversation above
- Streaming responses with markdown rendering
- Code blocks with syntax highlighting
- References to files/issues/PRs are clickable links
- "New conversation" button
- Conversation history accessible from Agents tab

### 9.3 Repo Settings → Agents Section

New section in repo settings:

- **Enable/Disable** toggle for the Gitwise Agent
- **Provider selector** — which LLM provider to use for this repo
- **Custom instructions** — textarea for repo-specific agent instructions
- **Trigger configuration** — checkboxes for push, schedule, manual
- **Behavior toggles** — review on push, open issues, open PRs, update docs
- **Ignore patterns** — file patterns the agent should skip
- **Add Custom Agent** — button to install a custom agent

### 9.4 Bot Badge

Throughout the UI, bot-created content gets a `[bot]` badge:
- Issue author: `gitwise-bot [bot]`
- PR author: `gitwise-bot [bot]`
- Review author: `gitwise-bot [bot]`
- Comment author: `gitwise-bot [bot]`

### 9.5 Admin Panel Extensions

- **AI Settings** — global LLM provider configuration, API key management
- **Usage Dashboard** — token usage per repo, per agent, per user
- **Queue Monitor** — live view of agent task queue, running tasks, failures

---

## 10. API Additions

### 10.1 Agent Management

```
GET    /api/v1/agents                              — list available agents (official + user's custom)
POST   /api/v1/agents                              — create custom agent
GET    /api/v1/agents/{slug}                       — get agent details
PUT    /api/v1/agents/{slug}                       — update custom agent
DELETE /api/v1/agents/{slug}                       — delete custom agent
```

### 10.2 Repo Agent Installation

```
GET    /api/v1/repos/{owner}/{repo}/agents         — list installed agents
POST   /api/v1/repos/{owner}/{repo}/agents         — install agent on repo
PUT    /api/v1/repos/{owner}/{repo}/agents/{slug}  — update agent config
DELETE /api/v1/repos/{owner}/{repo}/agents/{slug}  — uninstall agent
POST   /api/v1/repos/{owner}/{repo}/agents/{slug}/trigger — manually trigger agent
```

### 10.3 Agent Documents

```
GET    /api/v1/repos/{owner}/{repo}/docs           — list agent-generated docs
GET    /api/v1/repos/{owner}/{repo}/docs/{id}      — get document content
```

### 10.4 Agent Tasks

```
GET    /api/v1/repos/{owner}/{repo}/tasks          — list agent task history
GET    /api/v1/repos/{owner}/{repo}/tasks/{id}     — get task details
```

### 10.5 Chat

```
POST   /api/v1/repos/{owner}/{repo}/chat           — start conversation
GET    /api/v1/repos/{owner}/{repo}/chat            — list conversations
GET    /api/v1/repos/{owner}/{repo}/chat/{id}       — get conversation
POST   /api/v1/repos/{owner}/{repo}/chat/{id}/messages — send message
DELETE /api/v1/repos/{owner}/{repo}/chat/{id}       — delete conversation
```

### 10.6 Enhanced Search

```
POST   /api/v1/search?mode=semantic                — natural language search
POST   /api/v1/search?mode=ask                     — question-answering search
```

---

## 11. Open Questions

These are documented design questions to be resolved through experimentation:

### 11.1 Agent Document Types
What kinds of documents should the Gitwise Agent generate? Options:
- Architecture overview (what the codebase does, how it's structured)
- Component/module summaries (per-directory)
- API documentation (auto-generated from code)
- Dependency analysis (what's used, why, version status)
- Conventions guide (detected patterns, naming, test structure)
- Onboarding guide ("start here" for new contributors)
- All of the above as a living knowledge base?

The initial implementation should start with architecture + components and expand based on user feedback.

### 11.2 Push-Triggered Actions
What should the agent do on every push? Options:
- Review the diff for bugs, security issues, performance problems → open issues
- Spot TODOs/FIXMEs → open issues
- Identify refactoring opportunities → open PRs with changes
- Notice missing tests → open issue or PR
- Detect convention drift → open PR to fix
- Update relevant documentation

**Noise concern:** Every push generating multiple issues/PRs could be overwhelming. The agent needs a severity threshold and deduplication logic. It should batch findings and be conservative — only surface things that genuinely matter.

### 11.3 Agent Autonomy Level
How much should the agent do without human approval?
- **Read-only:** Reviews, comments, documentation only (safest)
- **Issues only:** Can open issues but not PRs (medium)
- **Full autonomy:** Can open issues AND PRs with code changes (most powerful, most risky)
- **Configurable per-repo** — let the user decide

The initial implementation should default to read-only (reviews + docs + issues) with full autonomy as an opt-in.

### 11.4 Chat Assistant Capabilities
What can the chat assistant do?
- **Read-only:** Answer questions about code, search issues/PRs, explain architecture
- **Interactive:** Above + open issues, create PRs, respond to comments (with user confirmation)
- **Agentic:** Above + make multi-step changes autonomously

Start with read-only, add interactive actions in a follow-up release.

### 11.5 Custom Agent Security
Custom agents run user-defined prompts against repo code. Risks:
- Prompt injection via code content (malicious code in a PR could manipulate the agent)
- Custom agents accessing repos they shouldn't (enforce repo-level permissions)
- Token cost abuse (rate limit per user, per repo)

These need mitigation strategies before custom agents ship broadly.

### 11.6 Agent-Generated PR Quality
If the agent opens PRs with code changes:
- How do we ensure the code compiles/passes tests?
- Should the agent run in a sandbox with build/test capability?
- Or should it only suggest changes and let humans verify?

This is the hardest problem. Start with issue-only and defer agent PRs to a later phase.

---

## 12. Implementation Phases

### Phase 1: Foundation (LLM Gateway + Queue)
- LLM Gateway with Anthropic + Ollama support
- Redis-backed task queue with provider-aware concurrency
- Bot user account system (`is_bot` flag)
- Agent/repo_agents/agent_tasks tables + migrations
- Configuration (env vars, admin settings)

### Phase 2: Gitwise Agent — Documentation
- Agent document generation (architecture, components)
- Post-push hook triggers agent
- Document storage + embedding
- Agents tab UI (document viewer)
- Repo Settings → Agents configuration UI

### Phase 3: Gitwise Agent — Code Review
- Push-triggered code review
- Agent opens issues for findings
- Review appears in PR thread (if push is to a PR branch)
- Severity labeling and deduplication
- Noise control (threshold, batching)

### Phase 4: Chat Assistant
- Chat backend with context assembly
- Streaming responses via WebSocket
- Chat panel UI (slide-out)
- Conversation persistence
- Agent docs as primary context source

### Phase 5: Enhanced Search
- Natural language search mode
- "Ask" mode with synthesized answers
- Agent docs included in search results

### Phase 6: Custom Agents
- Custom agent CRUD
- Per-repo installation with config overrides
- Custom system prompts and triggers

### Phase 7: Agent Autonomy (Experimental)
- Agent-generated PRs with code changes
- Build/test verification (if CI is available)
- User approval workflow before merge

---

## 13. Success Metrics

### 13.1 V2 Launch Criteria
- Gitwise Agent can be enabled on a repo and generates useful documentation within 5 minutes
- Push-triggered reviews surface real issues (not noise) on > 70% of activations
- Chat assistant answers codebase questions accurately using agent-generated context
- Local Ollama mode works without external network access
- Token usage is tracked and visible to admins
- The system degrades gracefully when the LLM provider is unavailable (queue tasks, retry later)

### 13.2 Quality Metrics
- Agent-generated docs are accurate enough that a new contributor can onboard using them
- Agent code reviews catch at least one real issue per 10 pushes (not false positives)
- Chat assistant response latency < 3 seconds to first token (streaming)
- Agent task queue processes push events within 60 seconds of the push

---

## 14. Non-Goals (V2)

- **CI/CD integration** — agents don't run builds or tests (yet)
- **Multi-repo agents** — each agent installation is per-repo
- **Agent marketplace** — no public sharing of agent configurations
- **Fine-tuning** — we use foundation models as-is, no custom training
- **Image/diagram generation** — text-only outputs
- **Code generation from scratch** — agents modify existing code, they don't create projects

---

*End of V2 Specification*

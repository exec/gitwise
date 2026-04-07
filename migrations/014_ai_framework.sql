-- Migration 014: AI Framework tables
-- Adds agent definitions, installations, documents, tasks, chat, and bot user support.

-- Bot user flag on existing users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_bot BOOLEAN NOT NULL DEFAULT false;

-- Agent definitions (built-in + custom)
CREATE TABLE agents (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        VARCHAR(100) NOT NULL,
    slug        VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    is_official BOOLEAN NOT NULL DEFAULT false,
    author_id   UUID REFERENCES users(id),
    config      JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agents_slug ON agents (slug);
CREATE INDEX idx_agents_author_id ON agents (author_id);

-- Agent installations on repos
CREATE TABLE repo_agents (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    repo_id        UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    agent_id       UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    enabled        BOOLEAN NOT NULL DEFAULT true,
    config         JSONB NOT NULL DEFAULT '{}',
    instructions   TEXT DEFAULT '',
    trigger_events TEXT[] DEFAULT '{push}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (repo_id, agent_id)
);

CREATE INDEX idx_repo_agents_repo_id ON repo_agents (repo_id);
CREATE INDEX idx_repo_agents_agent_id ON repo_agents (agent_id);

-- Agent-generated documents (living knowledge base)
CREATE TABLE agent_documents (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    repo_id           UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    agent_id          UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    title             VARCHAR(500) NOT NULL,
    content           TEXT NOT NULL,
    content_embedding VECTOR(1536),
    doc_type          VARCHAR(50) NOT NULL,
    metadata          JSONB DEFAULT '{}',
    version           INT NOT NULL DEFAULT 1,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_documents_repo_id ON agent_documents (repo_id);
CREATE INDEX idx_agent_documents_agent_id ON agent_documents (agent_id);
CREATE INDEX idx_agent_documents_doc_type ON agent_documents (doc_type);

-- Agent task log
CREATE TABLE agent_tasks (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    repo_id       UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    agent_id      UUID NOT NULL REFERENCES agents(id),
    trigger_event VARCHAR(50) NOT NULL,
    trigger_ref   TEXT,
    status        VARCHAR(20) NOT NULL DEFAULT 'queued',
    provider      VARCHAR(20) NOT NULL,
    input_tokens  INT DEFAULT 0,
    output_tokens INT DEFAULT 0,
    duration_ms   INT DEFAULT 0,
    result        JSONB DEFAULT '{}',
    error         TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at  TIMESTAMPTZ
);

CREATE INDEX idx_agent_tasks_repo_id ON agent_tasks (repo_id);
CREATE INDEX idx_agent_tasks_agent_id ON agent_tasks (agent_id);
CREATE INDEX idx_agent_tasks_status ON agent_tasks (status);
CREATE INDEX idx_agent_tasks_created_at ON agent_tasks (created_at);

-- Chat conversations
CREATE TABLE chat_conversations (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    repo_id    UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_chat_conversations_repo_id ON chat_conversations (repo_id);
CREATE INDEX idx_chat_conversations_user_id ON chat_conversations (user_id);

-- Chat messages
CREATE TABLE chat_messages (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    conversation_id   UUID NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
    role              VARCHAR(20) NOT NULL,
    content           TEXT NOT NULL,
    content_embedding VECTOR(1536),
    metadata          JSONB DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_chat_messages_conversation_id ON chat_messages (conversation_id);
CREATE INDEX idx_chat_messages_created_at ON chat_messages (created_at);

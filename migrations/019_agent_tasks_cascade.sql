-- Migration 019: agent_tasks.agent_id — add ON DELETE CASCADE
--
-- 014_ai_framework.sql:63 created agent_tasks.agent_id with a plain FK
-- (no delete action), so deleting an agent would fail if tasks exist.
-- We replace the constraint with ON DELETE CASCADE.

BEGIN;

ALTER TABLE agent_tasks DROP CONSTRAINT IF EXISTS agent_tasks_agent_id_fkey;
ALTER TABLE agent_tasks
    ADD CONSTRAINT agent_tasks_agent_id_fkey
    FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;

COMMIT;

-- Migration 021: Miscellaneous indexes and constraints
--
-- Covers:
--   a) webhook_deliveries.event_type — add index (filter-by-event was a seq scan)
--   b) code_files.content — add CHECK constraint (length < 1 MB) to prevent
--      unbounded content on monorepos; uses NOT VALID + VALIDATE pattern

BEGIN;

-- ----------------------------------------------------------------
-- a) Index on webhook_deliveries.event_type
-- ----------------------------------------------------------------
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_event_type
    ON webhook_deliveries (event_type);

-- ----------------------------------------------------------------
-- b) CHECK constraint on code_files.content length (< 1 000 000 bytes)
--
-- Step 1: truncate any existing offending rows to avoid a validation failure
--         (operator note: if you prefer to DELETE instead of truncate, swap the
--          UPDATE for DELETE FROM code_files WHERE length(content) >= 1000000)
UPDATE code_files
SET content = left(content, 999999)
WHERE length(content) >= 1000000;

-- Step 2: add the constraint as NOT VALID (instant, no table scan)
ALTER TABLE code_files
    ADD CONSTRAINT chk_code_files_content_length
    CHECK (length(content) < 1000000) NOT VALID;

-- Step 3: validate online (ShareUpdateExclusiveLock, non-blocking)
-- Comment out and run manually during low-traffic window if the table is large.
ALTER TABLE code_files VALIDATE CONSTRAINT chk_code_files_content_length;

COMMIT;

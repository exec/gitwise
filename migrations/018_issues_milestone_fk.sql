-- Migration 018: Add FK on issues.milestone_id → milestones(id)
--
-- issues.milestone_id was declared as a nullable UUID (001_initial.sql:131) with
-- no FOREIGN KEY constraint, so stale / dangling values are possible.
--
-- Strategy:
--   1. Null-out any issues whose milestone_id does not exist in milestones
--      (UPDATE instead of DELETE to preserve issues).
--   2. Add the FK NOT VALID so the ALTER TABLE is instant (no full table scan
--      while holding an exclusive lock).
--   3. Validate the constraint separately — can be run online with lower lock
--      requirements. It is included here so the migration is self-contained,
--      but an operator may comment it out and run it manually during low traffic.

BEGIN;

-- Step 1: clean up any dangling references before adding the constraint
UPDATE issues
SET milestone_id = NULL
WHERE milestone_id IS NOT NULL
  AND milestone_id NOT IN (SELECT id FROM milestones);

-- Step 2: add FK — NOT VALID skips the initial scan (fast, safe on large tables)
ALTER TABLE issues
    ADD CONSTRAINT issues_milestone_id_fkey
    FOREIGN KEY (milestone_id) REFERENCES milestones(id) ON DELETE SET NULL
    NOT VALID;

-- Step 3: validate (acquires ShareUpdateExclusiveLock, does not block reads/writes)
-- Comment this out and run manually if the table is very large.
ALTER TABLE issues VALIDATE CONSTRAINT issues_milestone_id_fkey;

COMMIT;

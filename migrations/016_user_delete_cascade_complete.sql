-- Migration 016: Complete user-delete cascade
--
-- Migration 013 added ON DELETE CASCADE to repositories.owner_id,
-- issues.author_id, and pull_requests.author_id but missed:
--   • comments.author_id        — CASCADE (deleting user removes their comments)
--   • reviews.author_id         — CASCADE (deleting user removes their reviews)
--   • commit_metadata.author_id — SET NULL (commits are historical records;
--                                  preserve the commit, just unlink the user)
--   • oauth_accounts.user_id    — CASCADE (also fixed in migration 016 here;
--                                  migration 009 already had CASCADE but we
--                                  verify via drop-then-add for certainty)
--
-- For commit_metadata.author_id: the column is already nullable (see 001_initial.sql:226),
-- so SET NULL is safe — it preserves commit history while allowing user deletion.

BEGIN;

-- ----------------------------------------------------------------
-- 1. comments.author_id → ON DELETE CASCADE
-- ----------------------------------------------------------------
ALTER TABLE comments DROP CONSTRAINT IF EXISTS comments_author_id_fkey;
ALTER TABLE comments
    ADD CONSTRAINT comments_author_id_fkey
    FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE CASCADE;

-- ----------------------------------------------------------------
-- 2. reviews.author_id → ON DELETE CASCADE
-- ----------------------------------------------------------------
ALTER TABLE reviews DROP CONSTRAINT IF EXISTS reviews_author_id_fkey;
ALTER TABLE reviews
    ADD CONSTRAINT reviews_author_id_fkey
    FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE CASCADE;

-- ----------------------------------------------------------------
-- 3. commit_metadata.author_id → ON DELETE SET NULL
--    (preserve historical commits; author_id is already nullable)
-- ----------------------------------------------------------------
ALTER TABLE commit_metadata DROP CONSTRAINT IF EXISTS commit_metadata_author_id_fkey;
ALTER TABLE commit_metadata
    ADD CONSTRAINT commit_metadata_author_id_fkey
    FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE SET NULL;

COMMIT;

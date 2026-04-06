-- Add ON DELETE CASCADE to FKs that reference users(id) so admin user deletion works.
-- repositories.owner_id, issues.author_id, pull_requests.author_id

ALTER TABLE repositories DROP CONSTRAINT IF EXISTS repositories_owner_id_fkey;
ALTER TABLE repositories ADD CONSTRAINT repositories_owner_id_fkey
    FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE issues DROP CONSTRAINT IF EXISTS issues_author_id_fkey;
ALTER TABLE issues ADD CONSTRAINT issues_author_id_fkey
    FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE pull_requests DROP CONSTRAINT IF EXISTS pull_requests_author_id_fkey;
ALTER TABLE pull_requests ADD CONSTRAINT pull_requests_author_id_fkey
    FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE CASCADE;

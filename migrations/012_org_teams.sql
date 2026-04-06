-- Organization teams with repository-level permissions

CREATE TABLE org_teams (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_id      UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name        VARCHAR(100) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    permission  VARCHAR(20) NOT NULL DEFAULT 'read',  -- read, triage, write, admin
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, name),
    CONSTRAINT org_teams_permission_check CHECK (permission IN ('read', 'triage', 'write', 'admin'))
);

CREATE INDEX idx_org_teams_org ON org_teams (org_id);

CREATE TABLE org_team_members (
    team_id UUID NOT NULL REFERENCES org_teams(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (team_id, user_id)
);

CREATE INDEX idx_org_team_members_user ON org_team_members (user_id);

CREATE TABLE org_team_repos (
    team_id UUID NOT NULL REFERENCES org_teams(id) ON DELETE CASCADE,
    repo_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    PRIMARY KEY (team_id, repo_id)
);

CREATE INDEX idx_org_team_repos_repo ON org_team_repos (repo_id);

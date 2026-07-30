-- Migration 001: Create tasks table
-- Run: psql $DATABASE_URL -f migrations/001_create_tasks.sql

CREATE TABLE IF NOT EXISTS tasks (
    id          TEXT        PRIMARY KEY,
    title       TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    status      TEXT        NOT NULL DEFAULT 'open',
    project_id  TEXT        NOT NULL,
    assignees   TEXT[]      NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for project-scoped queries (the most common read path)
CREATE INDEX IF NOT EXISTS idx_tasks_project_id ON tasks (project_id, created_at ASC);

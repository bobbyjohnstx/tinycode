-- tinycode database schema
-- Matches the Drizzle ORM schema from the TypeScript implementation

CREATE TABLE IF NOT EXISTS project (
    id TEXT PRIMARY KEY,
    worktree TEXT NOT NULL,
    vcs TEXT,
    name TEXT,
    icon_url TEXT,
    icon_url_override TEXT,
    icon_color TEXT,
    time_created INTEGER NOT NULL,
    time_updated INTEGER NOT NULL,
    time_initialized INTEGER,
    sandboxes TEXT NOT NULL DEFAULT '[]',
    commands TEXT
);

CREATE TABLE IF NOT EXISTS session (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
    workspace_id TEXT,
    parent_id TEXT,
    slug TEXT NOT NULL,
    directory TEXT NOT NULL,
    path TEXT,
    title TEXT NOT NULL,
    version TEXT NOT NULL,
    share_url TEXT,
    summary_additions INTEGER,
    summary_deletions INTEGER,
    summary_files INTEGER,
    summary_diffs TEXT,
    cost REAL NOT NULL DEFAULT 0,
    tokens_input INTEGER NOT NULL DEFAULT 0,
    tokens_output INTEGER NOT NULL DEFAULT 0,
    tokens_reasoning INTEGER NOT NULL DEFAULT 0,
    tokens_cache_read INTEGER NOT NULL DEFAULT 0,
    tokens_cache_write INTEGER NOT NULL DEFAULT 0,
    revert TEXT,
    permission TEXT,
    agent TEXT,
    model TEXT,
    time_created INTEGER NOT NULL,
    time_updated INTEGER NOT NULL,
    time_compacting INTEGER,
    time_archived INTEGER
);

CREATE INDEX IF NOT EXISTS session_project_idx ON session(project_id);
CREATE INDEX IF NOT EXISTS session_workspace_idx ON session(workspace_id);
CREATE INDEX IF NOT EXISTS session_parent_idx ON session(parent_id);

CREATE TABLE IF NOT EXISTS message (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES session(id) ON DELETE CASCADE,
    time_created INTEGER NOT NULL,
    time_updated INTEGER NOT NULL,
    data TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS message_session_time_created_id_idx ON message(session_id, time_created, id);

CREATE TABLE IF NOT EXISTS part (
    id TEXT PRIMARY KEY,
    message_id TEXT NOT NULL REFERENCES message(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL,
    time_created INTEGER NOT NULL,
    time_updated INTEGER NOT NULL,
    data TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS part_message_id_id_idx ON part(message_id, id);
CREATE INDEX IF NOT EXISTS part_session_idx ON part(session_id);

CREATE TABLE IF NOT EXISTS todo (
    session_id TEXT NOT NULL REFERENCES session(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    status TEXT NOT NULL,
    priority TEXT NOT NULL,
    position INTEGER NOT NULL,
    time_created INTEGER NOT NULL,
    time_updated INTEGER NOT NULL,
    PRIMARY KEY (session_id, position)
);

CREATE INDEX IF NOT EXISTS todo_session_idx ON todo(session_id);

CREATE TABLE IF NOT EXISTS permission (
    project_id TEXT PRIMARY KEY REFERENCES project(id) ON DELETE CASCADE,
    time_created INTEGER NOT NULL,
    time_updated INTEGER NOT NULL,
    data TEXT NOT NULL
);

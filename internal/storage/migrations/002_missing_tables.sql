-- Missing tables for full feature parity

CREATE TABLE IF NOT EXISTS session_message (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES session(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    tool_call_id TEXT,
    tool_calls TEXT,
    token_count INTEGER NOT NULL DEFAULT 0,
    time_created INTEGER NOT NULL,
    time_updated INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS session_message_session_idx ON session_message(session_id);
CREATE INDEX IF NOT EXISTS session_message_session_time_idx ON session_message(session_id, time_created);

CREATE TABLE IF NOT EXISTS workspace (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    directory TEXT NOT NULL,
    time_created INTEGER NOT NULL,
    time_updated INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS workspace_project_idx ON workspace(project_id);

CREATE TABLE IF NOT EXISTS account (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    email TEXT,
    name TEXT,
    avatar_url TEXT,
    token TEXT,
    refresh_token TEXT,
    token_expiry INTEGER,
    time_created INTEGER NOT NULL,
    time_updated INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS account_state (
    account_id TEXT PRIMARY KEY REFERENCES account(id) ON DELETE CASCADE,
    state TEXT NOT NULL,
    metadata TEXT,
    time_created INTEGER NOT NULL,
    time_updated INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS event_sequence (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES session(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL DEFAULT 0,
    time_created INTEGER NOT NULL,
    time_updated INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS event_sequence_session_idx ON event_sequence(session_id);

CREATE TABLE IF NOT EXISTS event (
    id TEXT PRIMARY KEY,
    sequence_id TEXT NOT NULL REFERENCES event_sequence(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL,
    type TEXT NOT NULL,
    data TEXT NOT NULL,
    seq INTEGER NOT NULL,
    time_created INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS event_sequence_id_seq_idx ON event(sequence_id, seq);
CREATE INDEX IF NOT EXISTS event_session_idx ON event(session_id);

CREATE TABLE IF NOT EXISTS data_migration (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    error TEXT,
    time_started INTEGER,
    time_completed INTEGER,
    time_created INTEGER NOT NULL
);

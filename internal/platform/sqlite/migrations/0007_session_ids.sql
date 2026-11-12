ALTER TABLE sessions ADD COLUMN session_id TEXT NOT NULL DEFAULT '';
UPDATE sessions SET session_id = lower(hex(randomblob(16))) WHERE session_id = '';
CREATE UNIQUE INDEX sessions_session_id_idx ON sessions(session_id);

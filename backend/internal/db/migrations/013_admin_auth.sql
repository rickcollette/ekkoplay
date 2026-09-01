CREATE TABLE admins (id INTEGER PRIMARY KEY,username TEXT NOT NULL UNIQUE COLLATE NOCASE,password_hash TEXT NOT NULL,active INTEGER NOT NULL DEFAULT 1,created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE refresh_sessions (id TEXT PRIMARY KEY,admin_id INTEGER NOT NULL REFERENCES admins(id) ON DELETE CASCADE,family_id TEXT NOT NULL,token_hash TEXT NOT NULL UNIQUE,expires_at TEXT NOT NULL,revoked_at TEXT,replaced_by TEXT,created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,last_used_at TEXT);
CREATE INDEX idx_refresh_sessions_admin ON refresh_sessions(admin_id);
CREATE INDEX idx_refresh_sessions_family ON refresh_sessions(family_id);

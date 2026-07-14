CREATE TABLE IF NOT EXISTS user_sessions (
    tg_chat_id   INTEGER NOT NULL,
    tg_user_id   INTEGER NOT NULL,
    tg_thread_id INTEGER NOT NULL DEFAULT 0,  -- forum topic / subchat; 0 outside topics
    data         TEXT    NOT NULL,            -- json column
    version      INTEGER NOT NULL,
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL,
    expires_at   INTEGER,
    PRIMARY KEY (tg_chat_id, tg_user_id, tg_thread_id)
);

CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions (expires_at);

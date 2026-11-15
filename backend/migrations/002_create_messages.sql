-- 002_create_messages.sql
-- Creates the messages table (direct messages between two users).
-- Idempotent: safe to run more than once.

CREATE TABLE IF NOT EXISTS messages (
    id          BIGSERIAL   PRIMARY KEY,
    sender_id   BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    receiver_id BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    content     TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT messages_sender_receiver_differ CHECK (sender_id <> receiver_id)
);

-- Supports ListConversation:
--   WHERE (sender_id = $1 AND receiver_id = $2) OR (sender_id = $2 AND receiver_id = $1)
--   ORDER BY created_at DESC, id DESC
CREATE INDEX IF NOT EXISTS idx_messages_conversation
    ON messages (sender_id, receiver_id, created_at DESC, id DESC);

-- Lets ON DELETE CASCADE from users find rows by receiver_id efficiently
-- (sender_id is already covered by the leading column above).
CREATE INDEX IF NOT EXISTS idx_messages_receiver_id
    ON messages (receiver_id);

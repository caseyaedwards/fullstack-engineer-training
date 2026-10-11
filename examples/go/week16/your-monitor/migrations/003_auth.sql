-- Week 11: the people who can change monitors, and their logins.

-- region: users
-- One row per person. password_hash holds a bcrypt hash, never the
-- password: 60 characters such as $2a$12$..., with the salt inside.
CREATE TABLE users (
    id            bigserial PRIMARY KEY,
    email         text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
-- endregion: users

-- region: sessions
-- One row per login. id is the SHA-256 hash of the cookie value, in hex:
-- a copy of this table, in a backup for example, logs nobody in.
-- Deleting a user deletes their sessions, which logs them out at once.
CREATE TABLE sessions (
    id         text PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

-- The cascade above finds the sessions of one user by user_id.
CREATE INDEX sessions_user ON sessions (user_id);
-- endregion: sessions

CREATE TABLE IF NOT EXISTS reflecto_score_events (
    id          SERIAL PRIMARY KEY,
    user_id     INT NOT NULL,
    post_id     INT NOT NULL,
    action_type TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, post_id, action_type)
);
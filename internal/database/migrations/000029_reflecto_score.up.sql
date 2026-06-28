CREATE TABLE reflecto_scores (
    id             SERIAL PRIMARY KEY,
    user_id        INTEGER NOT NULL UNIQUE,
    score          INTEGER NOT NULL DEFAULT 0,
    last_post_date DATE,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_reflecto_scores_user_id ON reflecto_scores(user_id);
CREATE INDEX idx_reflecto_scores_last_post_date ON reflecto_scores(last_post_date);

-- Optional: drop old streaks table once migrated
-- DROP TABLE streaks;
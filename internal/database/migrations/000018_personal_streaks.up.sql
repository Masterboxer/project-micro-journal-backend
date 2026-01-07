DROP TABLE IF EXISTS streaks;

CREATE TABLE streaks (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    streak_count INT NOT NULL DEFAULT 0,
    last_post_date DATE,
    longest_streak INT NOT NULL DEFAULT 0,
    started_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(user_id)
);

CREATE INDEX idx_streaks_user_id ON streaks(user_id);

CREATE TABLE IF NOT EXISTS streaks (
    id SERIAL PRIMARY KEY,
    user_id_1 INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_id_2 INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    streak_count INTEGER NOT NULL DEFAULT 0,
    last_contribution_date_user1 DATE,
    last_contribution_date_user2 DATE,
    started_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),

    CONSTRAINT unique_user_pair UNIQUE (user_id_1, user_id_2),
    CONSTRAINT user_ids_ordered CHECK (user_id_1 < user_id_2),
    CONSTRAINT different_users CHECK (user_id_1 != user_id_2)
);

CREATE INDEX IF NOT EXISTS idx_streaks_user1 ON streaks(user_id_1);
CREATE INDEX IF NOT EXISTS idx_streaks_user2 ON streaks(user_id_2);
CREATE INDEX IF NOT EXISTS idx_streaks_count ON streaks(streak_count DESC);
CREATE INDEX IF NOT EXISTS idx_streaks_updated ON streaks(updated_at DESC);

CREATE OR REPLACE FUNCTION update_streak_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_update_streak_timestamp ON streaks;
CREATE TRIGGER trigger_update_streak_timestamp
    BEFORE UPDATE ON streaks
    FOR EACH ROW
    EXECUTE FUNCTION update_streak_timestamp();
UPDATE users SET is_private = true WHERE is_private IS NULL;
ALTER TABLE users ALTER COLUMN is_private SET NOT NULL;
ALTER TABLE users ALTER COLUMN is_private SET DEFAULT true;
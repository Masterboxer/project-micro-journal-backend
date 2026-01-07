ALTER TABLE users ALTER COLUMN is_private SET DEFAULT true;

UPDATE users SET is_private = true WHERE is_private = false;

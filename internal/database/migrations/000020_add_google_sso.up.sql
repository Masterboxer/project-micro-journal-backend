ALTER TABLE users 
  ADD COLUMN google_id VARCHAR(255) UNIQUE,
  ADD COLUMN auth_provider VARCHAR(50) DEFAULT 'local';

-- Make password nullable
ALTER TABLE users ALTER COLUMN password DROP NOT NULL;
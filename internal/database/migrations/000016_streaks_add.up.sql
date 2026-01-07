ALTER TABLE users 
ADD COLUMN IF NOT EXISTS timezone VARCHAR(100) DEFAULT 'UTC';

UPDATE users 
SET timezone = 'Asia/Kolkata' 
WHERE timezone = 'UTC';

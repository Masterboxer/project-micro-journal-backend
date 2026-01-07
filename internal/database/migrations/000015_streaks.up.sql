ALTER TABLE posts
ADD COLUMN journal_date DATE;

UPDATE posts
SET journal_date = DATE(created_at)
WHERE journal_date IS NULL;

ALTER TABLE posts
ALTER COLUMN journal_date SET NOT NULL;

CREATE UNIQUE INDEX uniq_user_journal_date
ON posts(user_id, journal_date);

DELETE FROM posts p
USING posts p2
WHERE p.user_id = p2.user_id
  AND DATE(p.created_at) = DATE(p2.created_at)
  AND p.created_at > p2.created_at;

ALTER TABLE users
ADD COLUMN IF NOT EXISTS last_checkin_at timestamptz;

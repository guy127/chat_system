DROP INDEX IF EXISTS idx_messages_image_id;
ALTER TABLE messages DROP COLUMN IF EXISTS image_id;
DROP TABLE IF EXISTS images;

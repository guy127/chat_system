CREATE TABLE images (
  id           UUID PRIMARY KEY,
  room_id      UUID NOT NULL REFERENCES rooms(id),
  member_id    UUID NOT NULL REFERENCES room_members(id),
  content_type TEXT NOT NULL,  -- image/jpeg | image/png | image/gif, sniffed from the bytes
  size_bytes   INT NOT NULL,
  width        INT NOT NULL,
  height       INT NOT NULL,
  storage_key  TEXT NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_images_room_id ON images (room_id);

-- A message may carry one image; an image belongs to at most one message.
-- Uploaded images that never get attached are deleted by a background job.
ALTER TABLE messages ADD COLUMN image_id UUID REFERENCES images(id);
CREATE UNIQUE INDEX idx_messages_image_id ON messages (image_id) WHERE image_id IS NOT NULL;

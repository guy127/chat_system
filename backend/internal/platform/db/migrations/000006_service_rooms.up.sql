-- Rooms created by a trusted backend through /service/v1, e.g. one per claims
-- ticket channel. NULL = a normal room joined through invite links.
ALTER TABLE rooms ADD COLUMN external_ref TEXT UNIQUE;

-- Members of service rooms are keyed by the caller's own user id, so the same
-- person always maps to the same member. label is a role shown beside the name.
ALTER TABLE room_members
  ADD COLUMN external_user_id TEXT,
  ADD COLUMN label TEXT NOT NULL DEFAULT '',
  ADD CONSTRAINT room_members_room_external_user_key UNIQUE (room_id, external_user_id);

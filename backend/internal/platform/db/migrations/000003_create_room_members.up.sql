CREATE TABLE room_members (
  id           UUID PRIMARY KEY,
  room_id      UUID NOT NULL REFERENCES rooms(id),
  display_name TEXT NOT NULL,
  role         TEXT NOT NULL DEFAULT 'member', -- owner | member
  joined_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  kicked_at    TIMESTAMPTZ,                     -- removed; may rejoin through a valid invite as a new member
  banned_at    TIMESTAMPTZ
);

CREATE INDEX idx_room_members_room_id ON room_members (room_id);

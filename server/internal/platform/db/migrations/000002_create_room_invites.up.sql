CREATE TABLE room_invites (
  id          UUID PRIMARY KEY,
  room_id     UUID NOT NULL REFERENCES rooms(id),
  token_hash  TEXT NOT NULL UNIQUE,  -- SHA-256 of the token, never the token itself
  expires_at  TIMESTAMPTZ NOT NULL,
  max_uses    INT,                   -- NULL = unlimited
  used_count  INT NOT NULL DEFAULT 0,
  revoked_at  TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_room_invites_room_id ON room_invites (room_id);

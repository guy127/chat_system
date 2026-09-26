CREATE TABLE messages (
  id            TEXT COLLATE "C" PRIMARY KEY, -- ULID, sortable by time; byte order regardless of locale
  room_id       UUID NOT NULL REFERENCES rooms(id),
  member_id     UUID NOT NULL REFERENCES room_members(id),
  client_msg_id UUID NOT NULL,
  body          TEXT NOT NULL CHECK (char_length(body) <= 2000),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (room_id, client_msg_id)
);

CREATE INDEX idx_messages_room_id ON messages (room_id, id DESC);

CREATE TABLE rooms (
  id               UUID PRIMARY KEY,
  name             TEXT NOT NULL,
  owner_token_hash TEXT NOT NULL,
  status           TEXT NOT NULL DEFAULT 'active', -- active | closed
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

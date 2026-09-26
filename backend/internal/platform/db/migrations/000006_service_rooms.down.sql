ALTER TABLE room_members
  DROP CONSTRAINT room_members_room_external_user_key,
  DROP COLUMN label,
  DROP COLUMN external_user_id;

ALTER TABLE rooms DROP COLUMN external_ref;

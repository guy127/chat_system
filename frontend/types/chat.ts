// Single source of truth for the REST and WebSocket contract (snake_case, RFC 3339 UTC).

export type Role = "owner" | "member";

export type ErrorCode =
  | "not_found"
  | "unauthorized"
  | "forbidden"
  | "invalid_input"
  | "invite_invalid"
  | "invite_expired"
  | "invite_exhausted"
  | "room_closed"
  | "room_full"
  | "rate_limited"
  | "banned"
  | "kicked"
  | "message_too_long"
  | "image_too_large"
  | "image_unsupported"
  | "internal";

export interface Session {
  room_id: string;
  member_id: string;
  role: Role;
  jwt: string;
  expires_at: string;
  /** Only the host has this; it lets them get a new JWT later. */
  owner_token?: string;
  room_name?: string;
}

export interface Room {
  id: string;
  name: string;
  status: "active" | "closed";
  created_at: string;
}

export interface Member {
  id: string;
  display_name: string;
  role: Role;
  joined_at: string;
  online: boolean;
}

export interface ImageRef {
  id: string;
  content_type: string;
  width: number;
  height: number;
}

export interface ChatMessage {
  id: string; // ULID; sorts by time
  member_id: string;
  display_name: string;
  client_msg_id: string;
  body: string; // may be empty when image is set
  image: ImageRef | null;
  created_at: string;
}

export interface UploadedImage extends ImageRef {
  size_bytes: number;
}

export type RoomState = "active" | "kicked" | "banned" | "unauthorized" | "not_found";

export interface RoomSummary {
  room_id: string;
  state: RoomState;
  name?: string;
  status?: "active" | "closed";
  last_message: ChatMessage | null;
  unread: number; // capped at 99
}

export interface MessagePage {
  messages: ChatMessage[]; // oldest first
  next_cursor: string | null;
}

export interface CreatedInvite {
  id: string;
  token: string;
  invite_url: string;
  expires_at: string;
  max_uses: number | null;
}

export interface InviteView {
  id: string;
  expires_at: string;
  max_uses: number | null;
  used_count: number;
  created_at: string;
}

// ---- WebSocket frames ----

export type ClientFrame =
  | { type: "send"; client_msg_id: string; body: string; image_id?: string }
  | { type: "resume"; last_message_id: string }
  | { type: "ping" };

export type ServerFrame =
  | ({ type: "message" } & ChatMessage)
  | { type: "ack"; client_msg_id: string; id: string; created_at: string }
  | { type: "presence"; member_id: string; display_name: string; online: boolean }
  | { type: "error"; code: ErrorCode; message: string; client_msg_id?: string }
  | { type: "kicked"; reason: "kicked" | "banned" }
  | { type: "room_closed"; reason: string }
  | { type: "pong" };

/** Why the chat can no longer continue; the socket stops reconnecting. */
export type EndReason = "kicked" | "banned" | "room_closed" | "unauthorized";

export const MAX_BODY_LENGTH = 2000;
export const MAX_IMAGE_BYTES = 10 * 1024 * 1024;

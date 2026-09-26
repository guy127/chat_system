import type { ChatMessage } from "@/types/chat";

export interface PendingImage {
  blob: Blob;
  previewURL: string;
  width: number;
  height: number;
  uploadedId?: string; // set once the upload finished; retries skip re-uploading
  progress: number; // 0..1
}

export interface PendingMessage {
  client_msg_id: string;
  body: string;
  image?: PendingImage;
  status: "uploading" | "sending" | "failed";
  error?: string;
}

export interface MessagesState {
  confirmed: ChatMessage[]; // sorted by id (ULID) ascending, unique
  pending: PendingMessage[]; // sent by this tab, not yet stored
}

export type MessagesAction =
  | { type: "merge"; messages: ChatMessage[] }
  | { type: "queued"; client_msg_id: string; body: string; image?: PendingImage }
  | { type: "progress"; client_msg_id: string; progress: number }
  | { type: "uploaded"; client_msg_id: string; image_id: string }
  | { type: "failed"; client_msg_id: string; error: string }
  | { type: "retry"; client_msg_id: string };

export const initialMessages: MessagesState = { confirmed: [], pending: [] };

function updatePending(state: MessagesState, id: string, fn: (p: PendingMessage) => PendingMessage): MessagesState {
  return { ...state, pending: state.pending.map((p) => (p.client_msg_id === id ? fn(p) : p)) };
}

export function messagesReducer(state: MessagesState, action: MessagesAction): MessagesState {
  switch (action.type) {
    case "merge": {
      if (action.messages.length === 0) return state;
      const byId = new Map(state.confirmed.map((m) => [m.id, m]));
      for (const m of action.messages) byId.set(m.id, m);
      const confirmed = [...byId.values()].sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
      const arrived = new Set(action.messages.map((m) => m.client_msg_id));
      return { confirmed, pending: state.pending.filter((p) => !arrived.has(p.client_msg_id)) };
    }
    case "queued":
      return {
        ...state,
        pending: [
          ...state.pending,
          {
            client_msg_id: action.client_msg_id,
            body: action.body,
            image: action.image,
            status: action.image ? "uploading" : "sending",
          },
        ],
      };
    case "progress":
      return updatePending(state, action.client_msg_id, (p) =>
        p.image ? { ...p, image: { ...p.image, progress: action.progress } } : p,
      );
    case "uploaded":
      return updatePending(state, action.client_msg_id, (p) =>
        p.image ? { ...p, status: "sending", image: { ...p.image, uploadedId: action.image_id, progress: 1 } } : p,
      );
    case "failed":
      return updatePending(state, action.client_msg_id, (p) => ({ ...p, status: "failed", error: action.error }));
    case "retry":
      return updatePending(state, action.client_msg_id, (p) => ({
        ...p,
        status: p.image && !p.image.uploadedId ? "uploading" : "sending",
        error: undefined,
      }));
  }
}

/** Labels duplicate names by join order: "สมชาย", "สมชาย (2)", ... */
export function memberLabels(members: { id: string; display_name: string; joined_at: string }[]): Map<string, string> {
  const sorted = [...members].sort((a, b) => a.joined_at.localeCompare(b.joined_at) || a.id.localeCompare(b.id));
  const seen = new Map<string, number>();
  const labels = new Map<string, string>();
  for (const m of sorted) {
    const n = (seen.get(m.display_name) ?? 0) + 1;
    seen.set(m.display_name, n);
    labels.set(m.id, n === 1 ? m.display_name : `${m.display_name} (${n})`);
  }
  return labels;
}

/** One-line preview for room lists. */
export function previewText(m: ChatMessage): string {
  if (m.image) return m.body.trim() ? `📷 ${m.body}` : "📷 รูปภาพ";
  return m.body;
}

import type { ChatMessage } from "@/types/chat";

export interface PendingMessage {
  client_msg_id: string;
  body: string;
  status: "sending" | "failed";
  error?: string;
}

export interface MessagesState {
  confirmed: ChatMessage[]; // sorted by id (ULID) ascending, unique
  pending: PendingMessage[]; // sent by this tab, not yet stored
}

export type MessagesAction =
  | { type: "merge"; messages: ChatMessage[] }
  | { type: "queued"; client_msg_id: string; body: string }
  | { type: "failed"; client_msg_id: string; error: string }
  | { type: "retry"; client_msg_id: string };

export const initialMessages: MessagesState = { confirmed: [], pending: [] };

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
        pending: [...state.pending, { client_msg_id: action.client_msg_id, body: action.body, status: "sending" }],
      };
    case "failed":
    case "retry":
      return {
        ...state,
        pending: state.pending.map((p) =>
          p.client_msg_id !== action.client_msg_id
            ? p
            : action.type === "failed"
              ? { ...p, status: "failed", error: action.error }
              : { ...p, status: "sending", error: undefined },
        ),
      };
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

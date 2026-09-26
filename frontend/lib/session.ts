import { api } from "@/lib/api";
import type { CreatedInvite, Session } from "@/types/chat";

// Sessions live in localStorage so a refresh or a closed tab can come back.
// Every access is guarded: storage can be unavailable (private mode, blocked).

const PREFIX = "smalltalk:";
const sessionKey = (roomId: string) => `${PREFIX}session:${roomId}`;
const inviteKey = (roomId: string) => `${PREFIX}invite:${roomId}`;
const readKey = (roomId: string) => `${PREFIX}read:${roomId}`;
const NAME_KEY = `${PREFIX}display_name`;

function read<T>(key: string): T | null {
  try {
    const raw = localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as T) : null;
  } catch {
    return null;
  }
}

function write(key: string, value: unknown) {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // storage unavailable: the session only lasts for this page
  }
  // Same-tab listeners (useStored) only hear about changes through this event.
  window.dispatchEvent(new Event("smalltalk-storage"));
}

export const loadSession = (roomId: string) => read<Session>(sessionKey(roomId));
export const saveSession = (s: Session) => write(sessionKey(s.room_id), s);

export function isExpired(s: Session): boolean {
  return Date.parse(s.expires_at) - 60_000 < Date.now();
}

/** Every room this browser has a session for. */
export function allSessions(): Session[] {
  const out: Session[] = [];
  try {
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i);
      if (!key?.startsWith(`${PREFIX}session:`)) continue;
      const s = read<Session>(key);
      if (s) out.push(s);
    }
  } catch {
    return [];
  }
  return out;
}

/** Removes a room from this browser's list; it does not leave the room on the server. */
export function forgetRoom(roomId: string) {
  write(sessionKey(roomId), null);
  write(inviteKey(roomId), null);
  write(readKey(roomId), null);
}

/**
 * Returns a usable session: unchanged if still valid, refreshed with the owner
 * token for a host, or null for a guest whose JWT expired (needs a new link).
 */
export async function freshSession(s: Session): Promise<Session | null> {
  if (!isExpired(s)) return s;
  if (!s.owner_token) return null;
  try {
    const fresh = await api<Session>(`/rooms/${s.room_id}/owner-session`, {
      method: "POST",
      body: { owner_token: s.owner_token },
    });
    const next = { ...s, ...fresh };
    saveSession(next);
    return next;
  } catch {
    return null;
  }
}

export const loadLastRead = (roomId: string) => read<string>(readKey(roomId)) ?? "";
export function saveLastRead(roomId: string, id: string) {
  if (id > loadLastRead(roomId)) write(readKey(roomId), id); // ULIDs sort by time
}

export const loadInvite = (roomId: string) => read<CreatedInvite>(inviteKey(roomId));
export const saveInvite = (roomId: string, inv: CreatedInvite | null) => write(inviteKey(roomId), inv);

export const loadDisplayName = () => read<string>(NAME_KEY) ?? "";
export const saveDisplayName = (name: string) => write(NAME_KEY, name);

/** Absolute join link; the server returns a path when APP_BASE_URL is unset. */
export function inviteLink(inv: CreatedInvite): string {
  return inv.invite_url.startsWith("/") ? window.location.origin + inv.invite_url : inv.invite_url;
}

/** Pulls the invite token out of a pasted link (or accepts a bare token). */
export function tokenFromLink(input: string): string | null {
  const text = input.trim();
  const m = text.match(/\/j\/([A-Za-z0-9_-]{16,64})\/?(?:[?#].*)?$/);
  if (m) return m[1];
  return /^[A-Za-z0-9_-]{16,64}$/.test(text) ? text : null;
}

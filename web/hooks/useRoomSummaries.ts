"use client";

import { useEffect, useMemo, useState } from "react";

import { useStored } from "@/hooks/useStored";
import { api } from "@/lib/api";
import { allSessions, freshSession, loadLastRead } from "@/lib/session";
import type { RoomSummary, Session } from "@/types/chat";

export interface RoomEntry {
  session: Session;
  summary?: RoomSummary;
}

const POLL_MS = 15_000;
const noSessions: Session[] = [];

async function fetchSummaries(sessions: Session[]): Promise<Map<string, RoomSummary>> {
  // Hosts get a new JWT from their owner token; expired guests are reported as unauthorized.
  const fresh = await Promise.all(sessions.map((s) => freshSession(s)));
  const rooms = sessions.map((s, i) => ({
    room_id: s.room_id,
    jwt: fresh[i]?.jwt ?? "",
    last_read_id: loadLastRead(s.room_id),
  }));
  const res = await api<{ rooms: RoomSummary[] }>("/rooms/summaries", { method: "POST", body: { rooms } });
  return new Map(res.rooms.map((r) => [r.room_id, r]));
}

/** Every room this browser has joined, with its latest message and unread count. */
export function useRoomSummaries(): RoomEntry[] {
  const sessions = useStored(allSessions, noSessions);
  const [summaries, setSummaries] = useState<Map<string, RoomSummary>>(new Map());
  const roomKey = sessions.map((s) => s.room_id).join(",");

  useEffect(() => {
    if (sessions.length === 0) return;
    let alive = true;
    const refresh = () => {
      if (document.visibilityState !== "visible") return;
      fetchSummaries(sessions)
        .then((m) => alive && setSummaries(m))
        .catch(() => {}); // keep the last good list; the next poll retries
    };
    refresh();
    const timer = setInterval(refresh, POLL_MS);
    document.addEventListener("visibilitychange", refresh);
    return () => {
      alive = false;
      clearInterval(timer);
      document.removeEventListener("visibilitychange", refresh);
    };
    // Re-run when rooms are added or removed, not on every session refresh.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [roomKey]);

  return useMemo(() => {
    const entries = sessions.map((s) => ({ session: s, summary: summaries.get(s.room_id) }));
    // Newest activity first (message ids are ULIDs, so they sort by time).
    return entries.sort((a, b) => {
      const x = a.summary?.last_message?.id ?? "";
      const y = b.summary?.last_message?.id ?? "";
      return x === y ? 0 : x < y ? 1 : -1;
    });
  }, [sessions, summaries]);
}

"use client";

import Link from "next/link";
import { use, useEffect, useState } from "react";

import ChatRoom from "@/components/ChatRoom";
import { api } from "@/lib/api";
import { isExpired, loadSession, saveSession } from "@/lib/session";
import type { Session } from "@/types/chat";

type State = { kind: "loading" } | { kind: "ready"; session: Session } | { kind: "missing" } | { kind: "expired" };

export default function RoomPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const [state, setState] = useState<State>({ kind: "loading" });

  useEffect(() => {
    let cancelled = false;
    const resolve = async (): Promise<State> => {
      const s = loadSession(id);
      if (!s) return { kind: "missing" };
      if (!isExpired(s)) return { kind: "ready", session: s };
      if (!s.owner_token) return { kind: "expired" };
      // The host can always come back with the owner token.
      try {
        const fresh = await api<Session>(`/rooms/${id}/owner-session`, {
          method: "POST",
          body: { owner_token: s.owner_token },
        });
        const next = { ...s, ...fresh };
        saveSession(next);
        return { kind: "ready", session: next };
      } catch {
        return { kind: "expired" };
      }
    };
    resolve().then((st) => !cancelled && setState(st));
    return () => {
      cancelled = true;
    };
  }, [id]);

  if (state.kind === "ready") return <ChatRoom session={state.session} />;
  if (state.kind === "loading") return <main className="center muted">กำลังโหลด…</main>;
  return (
    <main className="center">
      <div className="card">
        <h1>{state.kind === "expired" ? "สิทธิ์เข้าห้องหมดอายุ" : "ยังไม่ได้เข้าห้องนี้"}</h1>
        <p>สแกน QR จากเจ้าของห้องอีกครั้งเพื่อเข้าร่วม</p>
        <Link href="/">กลับหน้าแรก</Link>
      </div>
    </main>
  );
}

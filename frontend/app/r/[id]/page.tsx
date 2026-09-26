"use client";

import { use, useEffect, useState } from "react";

import ChatRoom from "@/components/ChatRoom";
import DeadEnd from "@/components/DeadEnd";
import Mascot from "@/components/Mascot";
import { freshSession, loadSession } from "@/lib/session";
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
      // A host can always come back with the owner token; a guest needs a new link.
      const fresh = await freshSession(s);
      return fresh ? { kind: "ready", session: fresh } : { kind: "expired" };
    };
    resolve().then((st) => !cancelled && setState(st));
    return () => {
      cancelled = true;
    };
  }, [id]);

  // key: switching rooms from the sidebar remounts the chat with fresh state
  if (state.kind === "ready") return <ChatRoom key={state.session.room_id} session={state.session} />;
  if (state.kind === "loading") {
    return (
      <main className="center loading">
        <Mascot size={64} className="bounce" />
        <span className="muted">กำลังโหลด…</span>
      </main>
    );
  }
  return (
    <DeadEnd title={state.kind === "expired" ? "สิทธิ์เข้าห้องหมดอายุแล้ว" : "ยังไม่ได้เข้าห้องนี้นะ"}>
      ขอลิงก์เชิญ (หรือ QR) ใหม่จากเจ้าของห้อง แล้วกลับมาคุยกันต่อ
    </DeadEnd>
  );
}

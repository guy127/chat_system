"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { useStored } from "@/hooks/useStored";
import { api } from "@/lib/api";
import { errorText } from "@/lib/errors";
import { hostedRooms, loadDisplayName, saveDisplayName, saveInvite, saveSession } from "@/lib/session";
import type { CreatedInvite, Session } from "@/types/chat";

const noRooms: Session[] = [];

export default function HomePage() {
  const router = useRouter();
  const [roomName, setRoomName] = useState("");
  const savedName = useStored(loadDisplayName, "");
  const [typedName, setHostName] = useState<string | null>(null);
  const hostName = typedName ?? savedName;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const rooms = useStored(hostedRooms, noRooms);

  const create = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const created = await api<Session>("/rooms", {
        method: "POST",
        body: { name: roomName, display_name: hostName },
      });
      saveDisplayName(hostName.trim());
      saveSession({ ...created, room_name: roomName.trim() });
      // Issue the first QR right away (24 h, unlimited) so the host can share immediately.
      const inv = await api<CreatedInvite>(`/rooms/${created.room_id}/invites`, { method: "POST", jwt: created.jwt });
      saveInvite(created.room_id, inv);
      router.push(`/r/${created.room_id}`);
    } catch (err) {
      setError(errorText(err));
      setBusy(false);
    }
  };

  return (
    <main className="center">
      <form className="card" onSubmit={create}>
        <h1>QR Chat</h1>
        <p className="muted">สร้างห้อง แล้วให้คนสแกน QR เข้ามาคุยได้ทันที ไม่ต้องลงแอป ไม่ต้องสมัคร</p>
        <label>
          ชื่อห้อง
          <input
            value={roomName}
            onChange={(e) => setRoomName(e.target.value)}
            maxLength={80}
            required
            placeholder="เช่น ทีมงานอีเวนต์"
          />
        </label>
        <label>
          ชื่อของคุณ (Host)
          <input
            value={hostName}
            onChange={(e) => setHostName(e.target.value)}
            maxLength={32}
            required
            placeholder="เช่น กาย"
          />
        </label>
        <button className="primary wide" disabled={busy || !roomName.trim() || !hostName.trim()}>
          {busy ? "กำลังสร้าง…" : "สร้างห้อง"}
        </button>
        {error && <p className="error">{error}</p>}

        {rooms.length > 0 && (
          <>
            <hr />
            <h2>ห้องที่คุณเป็น Host</h2>
            <ul className="room-links">
              {rooms.map((r) => (
                <li key={r.room_id}>
                  <Link href={`/r/${r.room_id}`}>{r.room_name ?? r.room_id}</Link>
                </li>
              ))}
            </ul>
          </>
        )}
      </form>
    </main>
  );
}

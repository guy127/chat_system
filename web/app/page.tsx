"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import RoomList from "@/components/RoomList";
import { useStored } from "@/hooks/useStored";
import { api } from "@/lib/api";
import { errorText } from "@/lib/errors";
import { loadDisplayName, saveDisplayName, saveInvite, saveSession, tokenFromLink } from "@/lib/session";
import type { CreatedInvite, Session } from "@/types/chat";

export default function HomePage() {
  const router = useRouter();
  const [roomName, setRoomName] = useState("");
  const savedName = useStored(loadDisplayName, "");
  const [typedName, setHostName] = useState<string | null>(null);
  const hostName = typedName ?? savedName;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [link, setLink] = useState("");
  const [linkError, setLinkError] = useState("");

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
      // Issue the first invite link right away (24 h, unlimited) so the host can share immediately.
      const inv = await api<CreatedInvite>(`/rooms/${created.room_id}/invites`, { method: "POST", jwt: created.jwt });
      saveInvite(created.room_id, inv);
      router.push(`/r/${created.room_id}`);
    } catch (err) {
      setError(errorText(err));
      setBusy(false);
    }
  };

  const openLink = (e: FormEvent) => {
    e.preventDefault();
    const token = tokenFromLink(link);
    if (!token) {
      setLinkError("ลิงก์ไม่ถูกต้อง ตัวอย่าง: https://…/j/abc123…");
      return;
    }
    router.push(`/j/${token}`);
  };

  return (
    <main className="home">
      <header className="home-header">
        <h1>smalltalk</h1>
        <p className="muted">ห้องแชทที่ชวนเพื่อนเข้าได้ด้วยลิงก์หรือ QR ไม่ต้องลงแอป ไม่ต้องสมัคร</p>
      </header>

      <section className="card">
        <h2>ห้องของฉัน</h2>
        <RoomList />
      </section>

      <form className="card" onSubmit={openLink}>
        <h2>เข้าห้องด้วยลิงก์เชิญ</h2>
        <div className="inline-form">
          <input
            value={link}
            onChange={(e) => {
              setLink(e.target.value);
              setLinkError("");
            }}
            placeholder="วางลิงก์เชิญที่ได้รับมา"
            aria-label="ลิงก์เชิญ"
            inputMode="url"
          />
          <button className="primary" disabled={!link.trim()}>
            เปิด
          </button>
        </div>
        {linkError && <p className="error">{linkError}</p>}
      </form>

      <form className="card" onSubmit={create}>
        <h2>สร้างห้องใหม่</h2>
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
      </form>
    </main>
  );
}

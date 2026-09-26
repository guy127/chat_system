"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import Avatar from "@/components/Avatar";
import Icon from "@/components/Icon";
import Logo from "@/components/Logo";
import RoomList from "@/components/RoomList";
import { useStored } from "@/hooks/useStored";
import { api } from "@/lib/api";
import { errorText } from "@/lib/errors";
import { allSessions, loadDisplayName, saveDisplayName, saveInvite, saveSession, tokenFromLink } from "@/lib/session";
import type { CreatedInvite, Session } from "@/types/chat";

const ROOM_IDEAS = ["🎉 ปาร์ตี้วันเกิด", "📚 ติวสอบ", "🍜 เย็นนี้กินไรดี", "💼 ทีมงาน", "✈️ ทริปเที่ยว"];
const noSessions: Session[] = [];

export default function HomePage() {
  const router = useRouter();
  const hasRooms = useStored(allSessions, noSessions).length > 0;
  const [tab, setTab] = useState<"create" | "join">("create");
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
      setLinkError("ลิงก์นี้ดูแปลกๆ นะ ลองก๊อปมาใหม่อีกที (หน้าตาแบบ https://…/j/abc123…)");
      return;
    }
    router.push(`/j/${token}`);
  };

  const rooms = (
    <section className="card">
      <h2 className="section-title">ห้องของฉัน 💬</h2>
      <RoomList />
    </section>
  );

  return (
    <main className="home">
      <header className="brand">
        <Logo size={40} />
        <span className="wordmark">smalltalk</span>
      </header>

      {!hasRooms && (
        <section className="hero">
          <Logo size={128} className="float" />
          <h1>
            แชทกันง่ายๆ
            <br />
            <span className="gradient-text">แค่แชร์ลิงก์</span> ✨
          </h1>
          <p className="muted">ไม่ต้องลงแอป ไม่ต้องสมัคร เปิดห้อง ส่งลิงก์หรือ QR ให้เพื่อน แล้วคุยกันได้เลย</p>
        </section>
      )}

      {hasRooms && rooms}

      <section className="card start-card">
        <div className="segmented" role="tablist" aria-label="เริ่มแชท">
          <button role="tab" type="button" aria-selected={tab === "create"} onClick={() => setTab("create")}>
            <Icon name="sparkles" size={18} /> เปิดห้องใหม่
          </button>
          <button role="tab" type="button" aria-selected={tab === "join"} onClick={() => setTab("join")}>
            <Icon name="link" size={18} /> มีลิงก์เชิญ
          </button>
        </div>

        {tab === "create" ? (
          <form onSubmit={create} className="stack">
            <label>
              ตั้งชื่อห้อง
              <input
                value={roomName}
                onChange={(e) => setRoomName(e.target.value)}
                maxLength={80}
                required
                placeholder="ห้องนี้คุยเรื่องอะไรดี?"
              />
            </label>
            <div className="chips" aria-label="ไอเดียชื่อห้อง">
              {ROOM_IDEAS.map((idea) => (
                <button type="button" key={idea} className="chip" onClick={() => setRoomName(idea)}>
                  {idea}
                </button>
              ))}
            </div>
            <label>
              ชื่อเล่นของคุณ
              <span className="name-field">
                <Avatar id="me" name={hostName} size="sm" />
                <input
                  value={hostName}
                  onChange={(e) => setHostName(e.target.value)}
                  maxLength={32}
                  required
                  placeholder="เพื่อนๆ เรียกคุณว่าอะไร?"
                />
              </span>
            </label>
            <button className="primary wide big" disabled={busy || !roomName.trim() || !hostName.trim()}>
              {busy ? "กำลังเปิดห้อง…" : "เปิดห้องเลย 🚀"}
            </button>
            {error && <p className="error">{error}</p>}
          </form>
        ) : (
          <form onSubmit={openLink} className="stack">
            <label>
              ลิงก์เชิญที่เพื่อนส่งมา
              <input
                value={link}
                onChange={(e) => {
                  setLink(e.target.value);
                  setLinkError("");
                }}
                placeholder="วางลิงก์ตรงนี้เลย"
                inputMode="url"
              />
            </label>
            <button className="primary wide big" disabled={!link.trim()}>
              ไปที่ห้อง →
            </button>
            {linkError && <p className="error">{linkError}</p>}
            <p className="muted small-text hint">📷 ได้ QR มา? สแกนด้วยกล้องมือถือได้เลย ไม่ต้องวางลิงก์</p>
          </form>
        )}
      </section>

      {!hasRooms && (
        <ul className="perks">
          <li>🙅 ไม่ต้องสมัคร</li>
          <li>📱 สแกน QR เข้าห้อง</li>
          <li>🖼️ ส่งรูปได้</li>
        </ul>
      )}
    </main>
  );
}

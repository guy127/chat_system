"use client";

import Link from "next/link";

import Avatar from "@/components/Avatar";
import Icon from "@/components/Icon";
import { useRoomSummaries, type RoomEntry } from "@/hooks/useRoomSummaries";
import { previewText } from "@/lib/messages";
import { forgetRoom } from "@/lib/session";

interface Props {
  activeRoomId?: string;
}

const timeFmt = new Intl.DateTimeFormat("th-TH", { hour: "2-digit", minute: "2-digit" });
const dayFmt = new Intl.DateTimeFormat("th-TH", { day: "numeric", month: "short" });

function when(iso: string): string {
  const d = new Date(iso);
  return d.toDateString() === new Date().toDateString() ? timeFmt.format(d) : dayFmt.format(d);
}

function stateText(e: RoomEntry): string | null {
  const s = e.summary;
  if (!s) return null;
  switch (s.state) {
    case "kicked":
      return "คุณถูกเชิญออกจากห้องนี้";
    case "banned":
      return "คุณถูกแบนจากห้องนี้";
    case "unauthorized":
      return "สิทธิ์หมดอายุ ขอลิงก์เชิญใหม่";
    case "not_found":
      return "ไม่พบห้องนี้";
  }
  return s.status === "closed" ? "ห้องปิดแล้ว" : null;
}

export default function RoomList({ activeRoomId }: Props) {
  const entries = useRoomSummaries();

  if (entries.length === 0) {
    return <p className="muted small-text">ยังไม่มีห้องเลย ลองเปิดห้องใหม่หรือใช้ลิงก์เชิญจากเพื่อนดูสิ</p>;
  }

  return (
    <ul className="room-list">
      {entries.map((e) => {
        const { session: s, summary } = e;
        const name = summary?.name ?? s.room_name ?? "ห้องแชท";
        const blocked = summary !== undefined && summary.state !== "active";
        const note = stateText(e);
        const last = summary?.last_message;
        const unread = s.room_id === activeRoomId ? 0 : (summary?.unread ?? 0);
        const content = (
          <>
            <Avatar id={s.room_id} name={name} size="md" />
            <span className="room-text">
              <span className="room-row">
                <span className="room-name">
                  {name}
                  {s.role === "owner" && <span className="badge">👑</span>}
                </span>
                {last && <span className="muted small-text">{when(last.created_at)}</span>}
              </span>
              <span className="room-row">
                <span className={note ? "room-preview muted" : "room-preview"}>
                  {note ?? (last ? `${last.display_name}: ${previewText(last)}` : "ยังไม่มีข้อความ")}
                </span>
                {unread > 0 && <span className="unread">{unread >= 99 ? "99+" : unread}</span>}
              </span>
            </span>
          </>
        );
        return (
          <li key={s.room_id} className={s.room_id === activeRoomId ? "active" : undefined}>
            {blocked ? (
              <div className="room-link disabled">{content}</div>
            ) : (
              <Link className="room-link" href={`/r/${s.room_id}`}>
                {content}
              </Link>
            )}
            <button
              className="forget"
              title="ลบออกจากรายการ"
              aria-label={`ลบ ${name} ออกจากรายการ`}
              onClick={() => {
                const warn =
                  s.role === "owner"
                    ? "ลบห้องนี้ออกจากรายการ? คุณจะกลับมาจัดการห้องนี้ในฐานะ Host ไม่ได้อีก"
                    : "ลบห้องนี้ออกจากรายการ? ถ้าจะกลับเข้ามาต้องใช้ลิงก์เชิญใหม่";
                if (confirm(warn)) forgetRoom(s.room_id);
              }}
            >
              <Icon name="close" size={16} />
            </button>
          </li>
        );
      })}
    </ul>
  );
}

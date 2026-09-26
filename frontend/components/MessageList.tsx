"use client";

import { Fragment, useLayoutEffect, useRef, useState } from "react";

import Avatar from "@/components/Avatar";
import ChatImage, { imageBoxStyle } from "@/components/ChatImage";
import Icon from "@/components/Icon";
import Mascot from "@/components/Mascot";
import { isJumbo, type PendingMessage } from "@/lib/messages";
import type { ChatMessage } from "@/types/chat";

interface Props {
  roomId: string;
  jwt: string;
  onOpenImage: (url: string) => void;
  messages: ChatMessage[];
  pending: PendingMessage[];
  selfId: string;
  labels: Map<string, string>;
  hasMore: boolean;
  loadingOlder: boolean;
  onLoadOlder: () => void;
  onRetry: (clientMsgId: string) => void;
  /** Sends a canned greeting from the empty state; omitted when sending is not possible. */
  onQuickSend?: (text: string) => void;
}

const GREETINGS = ["หวัดดีทุกคน 👋", "มีใครอยู่มั้ย 👀", "ยินดีที่ได้รู้จักนะ 🫶"];
// Messages from the same person within this window share one bubble group.
const GROUP_MS = 5 * 60_000;

const timeFmt = new Intl.DateTimeFormat("th-TH", { hour: "2-digit", minute: "2-digit" });
const dateFmt = new Intl.DateTimeFormat("th-TH", { weekday: "short", day: "numeric", month: "short" });

const dayKey = (iso: string) => new Date(iso).toDateString();

function dayLabel(iso: string): string {
  const d = new Date(iso);
  const today = new Date();
  const yesterday = new Date(today);
  yesterday.setDate(today.getDate() - 1);
  if (d.toDateString() === today.toDateString()) return "วันนี้";
  if (d.toDateString() === yesterday.toDateString()) return "เมื่อวาน";
  return dateFmt.format(d);
}

function sameGroup(a: ChatMessage | undefined, b: ChatMessage | undefined): boolean {
  return (
    !!a &&
    !!b &&
    a.member_id === b.member_id &&
    dayKey(a.created_at) === dayKey(b.created_at) &&
    Date.parse(b.created_at) - Date.parse(a.created_at) < GROUP_MS
  );
}

const bubbleClass = (body: string, extra = "") => `${isJumbo(body) ? "bubble jumbo" : "bubble"}${extra}`;

export default function MessageList({
  roomId,
  jwt,
  onOpenImage,
  messages,
  pending,
  selfId,
  labels,
  hasMore,
  loadingOlder,
  onLoadOlder,
  onRetry,
  onQuickSend,
}: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const prev = useRef({ firstId: "", lastKey: "", height: 0, nearBottom: true });
  const [away, setAway] = useState(false);

  const firstId = messages[0]?.id ?? "";
  const lastKey = (messages.at(-1)?.id ?? "") + ":" + pending.length;

  // Keep the reading position stable when older messages are prepended, and
  // follow new messages only if the reader was already at the bottom.
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const p = prev.current;
    if (p.firstId && firstId !== p.firstId && lastKey === p.lastKey) {
      el.scrollTop += el.scrollHeight - p.height;
    } else if (lastKey !== p.lastKey && p.nearBottom) {
      el.scrollTop = el.scrollHeight;
    }
    prev.current = { firstId, lastKey, height: el.scrollHeight, nearBottom: p.nearBottom };
  }, [firstId, lastKey]);

  const onScroll = () => {
    const el = ref.current;
    if (!el) return;
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
    prev.current.nearBottom = nearBottom;
    prev.current.height = el.scrollHeight;
    setAway(el.scrollHeight - el.scrollTop - el.clientHeight > 400);
    if (el.scrollTop < 40 && hasMore && !loadingOlder) onLoadOlder();
  };

  const toBottom = () => ref.current?.scrollTo({ top: ref.current.scrollHeight, behavior: "smooth" });

  const empty = messages.length === 0 && pending.length === 0;
  const lastMine = messages.at(-1)?.member_id === selfId;

  return (
    <div className="messages-wrap">
      <div className="messages" ref={ref} onScroll={onScroll} role="log" aria-live="polite">
        {hasMore && (
          <button className="load-older" onClick={onLoadOlder} disabled={loadingOlder}>
            {loadingOlder ? "กำลังโหลด…" : "ดูข้อความก่อนหน้า"}
          </button>
        )}
        {empty && (
          <div className="empty">
            <Mascot size={88} className="float" />
            <p className="empty-title">ห้องยังเงียบอยู่เลย 🦗</p>
            <p className="muted small-text">เริ่มทักก่อนเลย แตะเพื่อส่งได้ทันที</p>
            {onQuickSend && (
              <div className="chips center-chips">
                {GREETINGS.map((g) => (
                  <button key={g} type="button" className="chip" onClick={() => onQuickSend(g)}>
                    {g}
                  </button>
                ))}
              </div>
            )}
          </div>
        )}
        {messages.map((m, i) => {
          const mine = m.member_id === selfId;
          const before = messages[i - 1];
          const first = !sameGroup(before, m);
          const last = !sameGroup(m, messages[i + 1]) && !(mine && i === messages.length - 1 && pending.length > 0);
          const newDay = !before || dayKey(before.created_at) !== dayKey(m.created_at);
          const name = labels.get(m.member_id) ?? m.display_name;
          const time = timeFmt.format(new Date(m.created_at));
          return (
            <Fragment key={m.id}>
              {newDay && (
                <div className="day-sep">
                  <span>{dayLabel(m.created_at)}</span>
                </div>
              )}
              <div className={`msg${mine ? " mine" : ""}${first ? " first" : ""}${last ? " last" : ""}`}>
                {!mine && <div className="msg-avatar">{last && <Avatar id={m.member_id} name={name} size="sm" />}</div>}
                <div className="msg-stack">
                  {!mine && first && <div className="msg-name">{name}</div>}
                  {m.image && <ChatImage roomId={roomId} jwt={jwt} image={m.image} onOpen={onOpenImage} />}
                  {m.body.trim() !== "" && (
                    <div className={bubbleClass(m.body)} title={time}>
                      {m.body}
                    </div>
                  )}
                  {last && <div className="msg-meta">{time}</div>}
                </div>
              </div>
            </Fragment>
          );
        })}
        {pending.map((p, i) => (
          <div
            key={p.client_msg_id}
            className={`msg mine${i === 0 && !lastMine ? " first" : ""}${i === pending.length - 1 || p.status === "failed" ? " last" : ""}`}
          >
            <div className="msg-stack">
              {p.image && (
                <div className="chat-image pending" style={imageBoxStyle(p.image.width, p.image.height)}>
                  {/* eslint-disable-next-line @next/next/no-img-element -- local preview */}
                  <img src={p.image.previewURL} alt="รูปที่กำลังส่ง" />
                  {p.status === "uploading" && (
                    <span className="upload-progress" style={{ width: `${(p.image.progress ?? 0) * 100}%` }} />
                  )}
                </div>
              )}
              {p.body.trim() !== "" && (
                <div className={bubbleClass(p.body, p.status === "failed" ? " failed" : " sending")}>{p.body}</div>
              )}
              <div className="msg-meta">
                {p.status === "uploading" ? (
                  `กำลังอัปโหลดรูป ${Math.round((p.image?.progress ?? 0) * 100)}%`
                ) : p.status === "sending" ? (
                  "กำลังส่ง…"
                ) : (
                  <span className="failed-meta">
                    ส่งไม่สำเร็จ{p.error ? ` · ${p.error}` : ""}{" "}
                    <button className="retry" onClick={() => onRetry(p.client_msg_id)}>
                      <Icon name="refresh" size={14} /> ลองอีกครั้ง
                    </button>
                  </span>
                )}
              </div>
            </div>
          </div>
        ))}
      </div>
      {away && (
        <button className="jump" onClick={toBottom} aria-label="ไปข้อความล่าสุด">
          <Icon name="down" size={18} />
        </button>
      )}
    </div>
  );
}

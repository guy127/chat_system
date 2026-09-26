"use client";

import { useLayoutEffect, useRef } from "react";

import type { PendingMessage } from "@/lib/messages";
import type { ChatMessage } from "@/types/chat";

interface Props {
  messages: ChatMessage[];
  pending: PendingMessage[];
  selfId: string;
  labels: Map<string, string>;
  hasMore: boolean;
  loadingOlder: boolean;
  onLoadOlder: () => void;
  onRetry: (clientMsgId: string) => void;
}

const timeFmt = new Intl.DateTimeFormat("th-TH", { hour: "2-digit", minute: "2-digit" });

export default function MessageList({
  messages,
  pending,
  selfId,
  labels,
  hasMore,
  loadingOlder,
  onLoadOlder,
  onRetry,
}: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const prev = useRef({ firstId: "", lastKey: "", height: 0, nearBottom: true });

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
    prev.current.nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
    prev.current.height = el.scrollHeight;
    if (el.scrollTop < 40 && hasMore && !loadingOlder) onLoadOlder();
  };

  return (
    <div className="messages" ref={ref} onScroll={onScroll} role="log" aria-live="polite">
      {hasMore && (
        <button className="load-older" onClick={onLoadOlder} disabled={loadingOlder}>
          {loadingOlder ? "กำลังโหลด…" : "โหลดข้อความก่อนหน้า"}
        </button>
      )}
      {messages.length === 0 && pending.length === 0 && <p className="empty">ยังไม่มีข้อความ เริ่มคุยได้เลย</p>}
      {messages.map((m, i) => {
        const mine = m.member_id === selfId;
        const showName = !mine && messages[i - 1]?.member_id !== m.member_id;
        return (
          <div key={m.id} className={mine ? "msg mine" : "msg"}>
            {showName && <div className="msg-name">{labels.get(m.member_id) ?? m.display_name}</div>}
            <div className="bubble">{m.body}</div>
            <div className="msg-meta">{timeFmt.format(new Date(m.created_at))}</div>
          </div>
        );
      })}
      {pending.map((p) => (
        <div key={p.client_msg_id} className="msg mine">
          <div className={p.status === "failed" ? "bubble failed" : "bubble sending"}>{p.body}</div>
          <div className="msg-meta">
            {p.status === "sending" ? (
              "กำลังส่ง…"
            ) : (
              <>
                ส่งไม่สำเร็จ{p.error ? ` · ${p.error}` : ""}{" "}
                <button className="link" onClick={() => onRetry(p.client_msg_id)}>
                  ลองอีกครั้ง
                </button>
              </>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}

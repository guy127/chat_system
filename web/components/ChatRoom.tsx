"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useReducer, useRef, useState } from "react";

import Composer from "@/components/Composer";
import HostPanel from "@/components/HostPanel";
import MemberList from "@/components/MemberList";
import MessageList from "@/components/MessageList";
import { useChatSocket } from "@/hooks/useChatSocket";
import { api, ApiError } from "@/lib/api";
import { codeText, errorText } from "@/lib/errors";
import { initialMessages, memberLabels, messagesReducer } from "@/lib/messages";
import { uuid } from "@/lib/uuid";
import type { EndReason, Member, MessagePage, Room, ServerFrame, Session } from "@/types/chat";

const endText: Record<EndReason, string> = {
  kicked: "คุณถูกเชิญออกจากห้องนี้",
  banned: "คุณถูกแบนจากห้องนี้",
  room_closed: "ห้องนี้ปิดแล้ว",
  unauthorized: "สิทธิ์เข้าห้องหมดอายุ กรุณาสแกน QR ใหม่",
};

const statusText = {
  connecting: "กำลังเชื่อมต่อ…",
  open: "เชื่อมต่อแล้ว",
  reconnecting: "กำลังเชื่อมต่อใหม่…",
  ended: "ตัดการเชื่อมต่อ",
};

export default function ChatRoom({ session }: { session: Session }) {
  const roomId = session.room_id;
  const isHost = session.role === "owner";

  const [room, setRoom] = useState<Room | null>(null);
  const [members, setMembers] = useState<Member[]>([]);
  const [msgs, dispatch] = useReducer(messagesReducer, initialMessages);
  const [cursor, setCursor] = useState<string | null>(null);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [ready, setReady] = useState(false);
  const [loadError, setLoadError] = useState<string>("");
  const [showSide, setShowSide] = useState(false);
  const [notice, setNotice] = useState("");

  const lastId = useRef<string | undefined>(undefined);
  useEffect(() => {
    lastId.current = msgs.confirmed.at(-1)?.id;
  }, [msgs.confirmed]);

  const fetchMembers = useCallback(
    () => api<{ members: Member[] }>(`/rooms/${roomId}/members`, { jwt: session.jwt }).then((r) => r.members),
    [roomId, session.jwt],
  );
  const loadMembers = useCallback(() => fetchMembers().then(setMembers), [fetchMembers]);

  const fetchLatest = useCallback(
    () => api<MessagePage>(`/rooms/${roomId}/messages?limit=50`, { jwt: session.jwt }),
    [roomId, session.jwt],
  );
  const loadLatest = useCallback(
    () => fetchLatest().then((page) => dispatch({ type: "merge", messages: page.messages })),
    [fetchLatest],
  );

  // History first, then the socket, so resume has an id to start from.
  useEffect(() => {
    let cancelled = false;
    Promise.all([api<Room>(`/rooms/${roomId}`, { jwt: session.jwt }), fetchLatest(), fetchMembers()])
      .then(([r, page, list]) => {
        if (cancelled) return;
        dispatch({ type: "merge", messages: page.messages });
        setMembers(list);
        setRoom(r);
        setCursor(page.next_cursor);
        setReady(true);
      })
      .catch((e) => {
        if (cancelled) return;
        if (e instanceof ApiError && (e.code === "kicked" || e.code === "banned" || e.code === "unauthorized")) {
          setLoadError(codeText(e.code));
        } else {
          setLoadError(errorText(e));
        }
      });
    return () => {
      cancelled = true;
    };
  }, [roomId, session.jwt, fetchLatest, fetchMembers]);

  // Presence changes can mean joins or removals; refetch the list, debounced.
  const membersTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const scheduleMembers = useCallback(() => {
    clearTimeout(membersTimer.current);
    membersTimer.current = setTimeout(() => loadMembers().catch(() => {}), 300);
  }, [loadMembers]);
  useEffect(() => () => clearTimeout(membersTimer.current), []);

  const onFrame = useCallback(
    (f: ServerFrame) => {
      switch (f.type) {
        case "message":
          dispatch({
            type: "merge",
            messages: [
              {
                id: f.id,
                member_id: f.member_id,
                display_name: f.display_name,
                client_msg_id: f.client_msg_id,
                body: f.body,
                created_at: f.created_at,
              },
            ],
          });
          break;
        case "presence":
          setMembers((prev) => prev.map((m) => (m.id === f.member_id ? { ...m, online: f.online } : m)));
          scheduleMembers();
          break;
        case "error":
          if (f.client_msg_id) dispatch({ type: "failed", client_msg_id: f.client_msg_id, error: codeText(f.code) });
          else if (f.code === "rate_limited") setNotice(codeText(f.code));
          break;
        case "room_closed":
          setRoom((r) => (r ? { ...r, status: "closed" } : r));
          break;
      }
    },
    [scheduleMembers],
  );

  const onOpen = useCallback(
    (resumed: boolean) => {
      // Nothing to resume from (empty room): re-read the latest page instead.
      if (!resumed) loadLatest().catch(() => {});
      scheduleMembers();
    },
    [loadLatest, scheduleMembers],
  );

  const roomClosed = room?.status === "closed";
  const socket = useChatSocket({
    roomId,
    jwt: ready && !roomClosed ? session.jwt : null,
    getLastMessageId: () => lastId.current,
    onFrame,
    onOpen,
  });

  const labels = useMemo(() => memberLabels(members), [members]);

  const send = (body: string) => {
    const id = uuid();
    dispatch({ type: "queued", client_msg_id: id, body });
    socket.send(id, body);
  };

  const retry = (clientMsgId: string) => {
    const p = msgs.pending.find((x) => x.client_msg_id === clientMsgId);
    if (!p) return;
    dispatch({ type: "retry", client_msg_id: clientMsgId });
    socket.send(clientMsgId, p.body);
  };

  const loadOlder = async () => {
    if (!cursor || loadingOlder) return;
    setLoadingOlder(true);
    try {
      const page = await api<MessagePage>(`/rooms/${roomId}/messages?limit=50&before=${cursor}`, { jwt: session.jwt });
      dispatch({ type: "merge", messages: page.messages });
      setCursor(page.next_cursor);
    } catch (e) {
      setNotice(errorText(e));
    } finally {
      setLoadingOlder(false);
    }
  };

  const removeMember = async (m: Member, ban: boolean) => {
    const name = labels.get(m.id) ?? m.display_name;
    if (!confirm(ban ? `แบน ${name}? คนนี้จะส่งข้อความในห้องไม่ได้อีก` : `เตะ ${name} ออกจากห้อง?`)) return;
    try {
      await api(`/rooms/${roomId}/members/${m.id}/kick`, { method: "POST", jwt: session.jwt, body: { ban } });
      await loadMembers();
    } catch (e) {
      setNotice(errorText(e));
    }
  };

  const closeRoom = async () => {
    try {
      await api(`/rooms/${roomId}/close`, { method: "POST", jwt: session.jwt });
      setRoom((r) => (r ? { ...r, status: "closed" } : r));
    } catch (e) {
      setNotice(errorText(e));
    }
  };

  useEffect(() => {
    if (!notice) return;
    const t = setTimeout(() => setNotice(""), 4000);
    return () => clearTimeout(t);
  }, [notice]);

  if (loadError) {
    return (
      <main className="center">
        <div className="card">
          <h1>เข้าห้องไม่ได้</h1>
          <p>{loadError}</p>
          <Link href="/">กลับหน้าแรก</Link>
        </div>
      </main>
    );
  }

  const ended: string | null = roomClosed
    ? endText.room_closed
    : socket.endReason && socket.endReason !== "room_closed"
      ? endText[socket.endReason]
      : null;

  return (
    <div className="chat-layout">
      <header className="chat-header">
        <div className="title">
          <h1>{room?.name ?? session.room_name ?? "…"}</h1>
          <span className={`status ${ended ? "ended" : socket.status}`}>
            {ended ?? (ready ? statusText[socket.status] : "กำลังโหลด…")}
          </span>
        </div>
        <button className="small side-toggle" onClick={() => setShowSide((v) => !v)} aria-expanded={showSide}>
          {showSide ? "กลับไปแชท" : isHost ? "QR / สมาชิก" : `สมาชิก (${members.filter((m) => m.online).length})`}
        </button>
      </header>

      <div className={showSide ? "chat-body show-side" : "chat-body"}>
        <main className="chat-main">
          <MessageList
            messages={msgs.confirmed}
            pending={msgs.pending}
            selfId={session.member_id}
            labels={labels}
            hasMore={cursor !== null}
            loadingOlder={loadingOlder}
            onLoadOlder={loadOlder}
            onRetry={retry}
          />
          {notice && <div className="toast">{notice}</div>}
          {ended ? <div className="ended-bar">{ended}</div> : <Composer disabled={!ready} onSend={send} />}
        </main>

        <aside className="chat-side">
          {isHost && <HostPanel session={session} roomClosed={roomClosed} onClose={closeRoom} />}
          <MemberList
            members={members}
            labels={labels}
            selfId={session.member_id}
            canModerate={isHost && !roomClosed}
            onRemove={removeMember}
          />
        </aside>
      </div>
    </div>
  );
}

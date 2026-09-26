"use client";

import { QRCodeSVG } from "qrcode.react";
import { useCallback, useEffect, useState } from "react";

import { api } from "@/lib/api";
import { errorText } from "@/lib/errors";
import { inviteLink, loadInvite, saveInvite } from "@/lib/session";
import type { CreatedInvite, InviteView, Session } from "@/types/chat";

interface Props {
  session: Session;
  roomClosed: boolean;
  onClose: () => void;
}

const TTL_OPTIONS = [
  { label: "1 ชั่วโมง", minutes: 60 },
  { label: "24 ชั่วโมง", minutes: 24 * 60 },
  { label: "7 วัน", minutes: 7 * 24 * 60 },
];

const dateFmt = new Intl.DateTimeFormat("th-TH", { dateStyle: "short", timeStyle: "short" });

export default function HostPanel({ session, roomClosed, onClose }: Props) {
  const roomId = session.room_id;
  const [current, setCurrent] = useState<CreatedInvite | null>(null);
  const [active, setActive] = useState<InviteView[]>([]);
  const [ttl, setTtl] = useState(24 * 60);
  const [maxUses, setMaxUses] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState(false);

  const fetchInvites = useCallback(async () => {
    const res = await api<{ invites: InviteView[] }>(`/rooms/${roomId}/invites`, { jwt: session.jwt });
    // The QR can only be redrawn from the token we saved at creation time.
    const saved = loadInvite(roomId);
    const stillUsable = saved !== null && res.invites.some((i) => i.id === saved.id);
    if (!stillUsable && saved) saveInvite(roomId, null);
    return { invites: res.invites, current: stillUsable ? saved : null };
  }, [roomId, session.jwt]);

  const apply = ({ invites, current }: { invites: InviteView[]; current: CreatedInvite | null }) => {
    setActive(invites);
    setCurrent(current);
  };
  const refresh = () => fetchInvites().then(apply);

  useEffect(() => {
    if (roomClosed) return;
    fetchInvites()
      .then(apply)
      .catch((e) => setError(errorText(e)));
  }, [fetchInvites, roomClosed]);

  /** Revokes every active invite, then issues a fresh one (F3). */
  const issueNew = async () => {
    setBusy(true);
    setError("");
    try {
      await Promise.all(
        active.map((i) => api(`/rooms/${roomId}/invites/${i.id}`, { method: "DELETE", jwt: session.jwt })),
      );
      const n = maxUses.trim() === "" ? null : Number(maxUses);
      const inv = await api<CreatedInvite>(`/rooms/${roomId}/invites`, {
        method: "POST",
        jwt: session.jwt,
        body: { expires_in_minutes: ttl, max_uses: n },
      });
      saveInvite(roomId, inv);
      await refresh();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  };

  const revoke = async (id: string) => {
    setError("");
    try {
      await api(`/rooms/${roomId}/invites/${id}`, { method: "DELETE", jwt: session.jwt });
      await refresh();
    } catch (e) {
      setError(errorText(e));
    }
  };

  const copy = async (url: string) => {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // clipboard needs a secure context; the link is still visible to copy by hand
    }
  };

  if (roomClosed) {
    return (
      <section className="panel">
        <h2>ห้องปิดแล้ว</h2>
        <p className="muted">ไม่สามารถออก QR หรือส่งข้อความได้อีก ประวัติแชทยังเปิดอ่านได้</p>
      </section>
    );
  }

  const url = current ? inviteLink(current) : "";

  return (
    <section className="panel">
      <h2>QR เชิญเข้าห้อง</h2>
      {current ? (
        <div className="qr">
          <QRCodeSVG value={url} size={200} marginSize={2} bgColor="#ffffff" fgColor="#000000" />
          <button className="link url" onClick={() => copy(url)} title="คัดลอกลิงก์">
            {copied ? "คัดลอกแล้ว ✓" : url}
          </button>
          <p className="muted small-text">
            หมดอายุ {dateFmt.format(new Date(current.expires_at))}
            {current.max_uses != null &&
              ` · ใช้แล้ว ${active.find((i) => i.id === current.id)?.used_count ?? 0}/${current.max_uses}`}
          </p>
        </div>
      ) : (
        <p className="muted">ยังไม่มี QR ที่ใช้งานได้ กดออก QR ใหม่เพื่อเชิญคนเข้าห้อง</p>
      )}

      <div className="invite-form">
        <label>
          อายุ QR
          <select value={ttl} onChange={(e) => setTtl(Number(e.target.value))}>
            {TTL_OPTIONS.map((o) => (
              <option key={o.minutes} value={o.minutes}>
                {o.label}
              </option>
            ))}
          </select>
        </label>
        <label>
          ใช้ได้กี่ครั้ง
          <input
            type="number"
            min={1}
            max={200}
            inputMode="numeric"
            placeholder="ไม่จำกัด"
            value={maxUses}
            onChange={(e) => setMaxUses(e.target.value)}
          />
        </label>
      </div>
      <button className="primary wide" onClick={issueNew} disabled={busy}>
        {active.length > 0 ? "ยกเลิก QR เดิมและออก QR ใหม่" : "ออก QR ใหม่"}
      </button>

      {active.length > (current ? 1 : 0) && (
        <ul className="invites">
          {active
            .filter((i) => i.id !== current?.id)
            .map((i) => (
              <li key={i.id}>
                <span className="muted small-text">
                  QR อื่นที่ยังใช้ได้ · หมดอายุ {dateFmt.format(new Date(i.expires_at))}
                </span>
                <button className="small" onClick={() => revoke(i.id)}>
                  ยกเลิก
                </button>
              </li>
            ))}
        </ul>
      )}
      {current && (
        <button className="link danger-text" onClick={() => revoke(current.id)}>
          ยกเลิก QR นี้
        </button>
      )}

      {error && <p className="error">{error}</p>}

      <hr />
      <button
        className="danger wide"
        onClick={() => {
          if (confirm("ปิดห้องนี้? ทุกคนจะถูกตัดออกและเข้าห้องไม่ได้อีก")) onClose();
        }}
      >
        ปิดห้อง
      </button>
    </section>
  );
}

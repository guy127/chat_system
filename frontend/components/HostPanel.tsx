"use client";

import { QRCodeSVG } from "qrcode.react";
import { useCallback, useEffect, useState } from "react";

import Icon from "@/components/Icon";
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
  { label: "1 ชม.", minutes: 60 },
  { label: "24 ชม.", minutes: 24 * 60 },
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
  const [showQR, setShowQR] = useState(false);

  const fetchInvites = useCallback(async () => {
    const res = await api<{ invites: InviteView[] }>(`/rooms/${roomId}/invites`, { jwt: session.jwt });
    // The link can only be shown again from the token we saved at creation time.
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
        <h2 className="section-title">ห้องปิดแล้ว 🌙</h2>
        <p className="muted small-text">เชิญคนเพิ่มหรือส่งข้อความไม่ได้แล้ว แต่ยังย้อนอ่านแชทเก่าได้นะ</p>
      </section>
    );
  }

  const url = current ? inviteLink(current) : "";

  const canShare = typeof navigator !== "undefined" && typeof navigator.share === "function";
  const share = async () => {
    try {
      await navigator.share({ title: "เข้าห้องแชท smalltalk", url });
    } catch {
      // cancelled by the user
    }
  };

  const settings = (
    <>
      <div className="field-label">ลิงก์ใช้ได้นาน</div>
      <div className="segmented small" role="radiogroup" aria-label="อายุลิงก์">
        {TTL_OPTIONS.map((o) => (
          <button
            key={o.minutes}
            type="button"
            role="radio"
            aria-checked={ttl === o.minutes}
            onClick={() => setTtl(o.minutes)}
          >
            {o.label}
          </button>
        ))}
      </div>
      <label>
        ใช้ได้กี่ครั้ง
        <input
          type="number"
          min={1}
          max={200}
          inputMode="numeric"
          placeholder="ไม่จำกัด ♾️"
          value={maxUses}
          onChange={(e) => setMaxUses(e.target.value)}
        />
      </label>
      <button className="primary wide" onClick={issueNew} disabled={busy}>
        <Icon name="sparkles" size={18} />{" "}
        {active.length > 0 ? "สร้างลิงก์ใหม่ (ลิงก์เดิมจะใช้ไม่ได้)" : "สร้างลิงก์เชิญ"}
      </button>
    </>
  );

  return (
    <section className="panel">
      <h2 className="section-title">ชวนเพื่อนเข้าห้อง 💌</h2>
      {current ? (
        <div className="invite">
          <div className="invite-ticket">
            <span className="invite-url" title={url}>
              {url.replace(/^https?:\/\//, "")}
            </span>
            <span className="invite-meta">
              ⏳ หมดอายุ {dateFmt.format(new Date(current.expires_at))}
              {current.max_uses != null &&
                ` · ใช้ไป ${active.find((i) => i.id === current.id)?.used_count ?? 0}/${current.max_uses}`}
            </span>
          </div>
          <div className="invite-actions">
            <button className={copied ? "primary copied" : "primary"} onClick={() => copy(url)}>
              <Icon name={copied ? "check" : "copy"} size={18} /> {copied ? "ก๊อปแล้ว!" : "ก๊อปลิงก์"}
            </button>
            {canShare && (
              <button onClick={share} aria-label="แชร์">
                <Icon name="share" size={18} /> แชร์
              </button>
            )}
            <button
              className={showQR ? "active" : undefined}
              onClick={() => setShowQR((v) => !v)}
              aria-expanded={showQR}
              aria-label={showQR ? "ซ่อน QR" : "แสดง QR"}
            >
              <Icon name="qr" size={18} /> QR
            </button>
          </div>
          {showQR && (
            <div className="qr">
              <QRCodeSVG value={url} size={200} marginSize={2} bgColor="#ffffff" fgColor="#2a1f3d" />
              <span className="muted small-text">ให้เพื่อนสแกนด้วยกล้องมือถือได้เลย 📷</span>
            </div>
          )}
          <details className="more">
            <summary>ตั้งค่าลิงก์ใหม่</summary>
            <div className="stack">{settings}</div>
          </details>
        </div>
      ) : (
        <div className="stack">
          <p className="muted small-text">ยังไม่มีลิงก์เชิญที่ใช้ได้ สร้างใหม่เพื่อชวนเพื่อนเข้าห้องกัน</p>
          {settings}
        </div>
      )}

      {active.length > (current ? 1 : 0) && (
        <ul className="invites">
          {active
            .filter((i) => i.id !== current?.id)
            .map((i) => (
              <li key={i.id}>
                <span className="muted small-text">
                  ลิงก์อื่นที่ยังใช้ได้ · หมดอายุ {dateFmt.format(new Date(i.expires_at))}
                </span>
                <button className="small" onClick={() => revoke(i.id)}>
                  ยกเลิก
                </button>
              </li>
            ))}
        </ul>
      )}

      {error && <p className="error">{error}</p>}

      <div className="danger-zone">
        {current && (
          <button className="small ghost" onClick={() => revoke(current.id)}>
            ยกเลิกลิงก์นี้
          </button>
        )}
        <button
          className="small danger"
          onClick={() => {
            if (confirm("ปิดห้องนี้? ทุกคนจะถูกตัดออกและเข้าห้องไม่ได้อีก")) onClose();
          }}
        >
          <Icon name="power" size={16} /> ปิดห้อง
        </button>
      </div>
    </section>
  );
}

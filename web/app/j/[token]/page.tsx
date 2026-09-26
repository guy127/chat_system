"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { use, useState, type FormEvent } from "react";

import { useStored } from "@/hooks/useStored";
import { api, ApiError } from "@/lib/api";
import { errorText } from "@/lib/errors";
import { loadDisplayName, saveDisplayName, saveSession } from "@/lib/session";
import type { Session } from "@/types/chat";

// Errors that mean this QR will never work; hide the form and explain.
const deadEnds = new Set(["invite_invalid", "invite_expired", "invite_exhausted", "room_closed", "room_full"]);

export default function JoinPage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = use(params);
  const router = useRouter();
  const savedName = useStored(loadDisplayName, "");
  const [typedName, setName] = useState<string | null>(null);
  const name = typedName ?? savedName;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [deadEnd, setDeadEnd] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const joined = await api<Session>("/join", { method: "POST", body: { token, display_name: name } });
      saveDisplayName(name.trim());
      saveSession(joined);
      router.replace(`/r/${joined.room_id}`);
    } catch (err) {
      setError(errorText(err));
      setDeadEnd(err instanceof ApiError && deadEnds.has(err.code));
      setBusy(false);
    }
  };

  return (
    <main className="center">
      <form className="card" onSubmit={submit}>
        <h1>เข้าร่วมห้องแชท</h1>
        {deadEnd ? (
          <>
            <p className="error">{error}</p>
            <Link href="/">กลับหน้าแรก</Link>
          </>
        ) : (
          <>
            <label>
              ชื่อเล่นของคุณ
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                maxLength={32}
                required
                autoFocus
                placeholder="เช่น สมชาย"
              />
            </label>
            <button className="primary wide" disabled={busy || name.trim() === ""}>
              {busy ? "กำลังเข้าห้อง…" : "เข้าห้อง"}
            </button>
            {error && <p className="error">{error}</p>}
            <p className="muted small-text">ไม่ต้องสมัครสมาชิก ชื่อนี้จะแสดงให้คนในห้องเห็น</p>
          </>
        )}
      </form>
    </main>
  );
}

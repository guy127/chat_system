"use client";

import type { Member } from "@/types/chat";

interface Props {
  members: Member[];
  labels: Map<string, string>;
  selfId: string;
  canModerate: boolean;
  onRemove: (member: Member, ban: boolean) => void;
}

export default function MemberList({ members, labels, selfId, canModerate, onRemove }: Props) {
  const sorted = [...members].sort((a, b) => Number(b.online) - Number(a.online));
  const online = members.filter((m) => m.online).length;

  return (
    <section className="panel">
      <h2>
        สมาชิก{" "}
        <span className="muted">
          · ออนไลน์ {online}/{members.length}
        </span>
      </h2>
      <ul className="members">
        {sorted.map((m) => (
          <li key={m.id}>
            <span className={m.online ? "dot on" : "dot"} aria-label={m.online ? "ออนไลน์" : "ออฟไลน์"} />
            <span className="member-name">
              {labels.get(m.id) ?? m.display_name}
              {m.role === "owner" && <span className="badge">Host</span>}
              {m.id === selfId && <span className="muted"> (คุณ)</span>}
            </span>
            {canModerate && m.role !== "owner" && (
              <span className="member-actions">
                <button className="small" onClick={() => onRemove(m, false)}>
                  เตะ
                </button>
                <button className="small danger" onClick={() => onRemove(m, true)}>
                  แบน
                </button>
              </span>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

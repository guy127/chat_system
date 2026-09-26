"use client";

import Avatar from "@/components/Avatar";
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
      <h2 className="section-title">
        สมาชิก
        <span className="pill online-pill">
          ออนไลน์ {online}/{members.length}
        </span>
      </h2>
      <ul className="members">
        {sorted.map((m) => {
          const name = labels.get(m.id) ?? m.display_name;
          return (
            <li key={m.id} className={m.online ? undefined : "offline"}>
              <Avatar id={m.id} name={name} size="md" online={m.online} />
              <span className="member-name">
                {name}
                {m.role === "owner" && <span className="badge">👑 Host</span>}
                {m.id === selfId && <span className="badge soft">คุณ</span>}
              </span>
              <span className="sr-only">{m.online ? "ออนไลน์" : "ออฟไลน์"}</span>
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
          );
        })}
      </ul>
    </section>
  );
}

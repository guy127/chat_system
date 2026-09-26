import Link from "next/link";

import Mascot from "@/components/Mascot";

/** A friendly full-page stop: the room can't be opened, so point people home. */
export default function DeadEnd({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <main className="center">
      <div className="card dead-end">
        <Mascot size={88} mood="sad" className="float" />
        <h1>{title}</h1>
        <p className="muted">{children}</p>
        <Link href="/" className="button primary wide">
          กลับหน้าแรก
        </Link>
      </div>
    </main>
  );
}

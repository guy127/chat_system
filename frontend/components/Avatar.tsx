import type { CSSProperties } from "react";

import { hueOf, initials } from "@/lib/avatar";

interface Props {
  id: string;
  name: string;
  size?: "sm" | "md" | "lg";
  online?: boolean;
}

export default function Avatar({ id, name, size = "md", online }: Props) {
  return (
    <span className={`avatar ${size}`} style={{ "--h": hueOf(id) } as CSSProperties} aria-hidden="true">
      {initials(name)}
      {online !== undefined && <span className={online ? "presence on" : "presence"} />}
    </span>
  );
}

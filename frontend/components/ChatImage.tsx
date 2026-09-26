"use client";

import { useEffect, useState } from "react";

import { loadImageURL } from "@/lib/images";
import type { ImageRef } from "@/types/chat";

interface Props {
  roomId: string;
  jwt: string;
  image: ImageRef;
  onOpen: (url: string) => void;
}

/** Reserves the image's aspect ratio before it loads so the chat does not jump. */
export function imageBoxStyle(width: number, height: number) {
  return { aspectRatio: `${width} / ${height}`, width: `min(260px, ${width}px, 60vw)` };
}

export default function ChatImage({ roomId, jwt, image, onOpen }: Props) {
  const [url, setUrl] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let alive = true;
    loadImageURL(roomId, image.id, jwt)
      .then((u) => alive && setUrl(u))
      .catch(() => alive && setFailed(true));
    return () => {
      alive = false;
    };
  }, [roomId, image.id, jwt]);

  return (
    <button
      type="button"
      className="chat-image"
      style={imageBoxStyle(image.width, image.height)}
      onClick={() => url && onOpen(url)}
      disabled={!url}
      aria-label="ดูรูปขนาดเต็ม"
    >
      {url ? (
        // eslint-disable-next-line @next/next/no-img-element -- object URL from an authenticated fetch
        <img src={url} alt="รูปภาพในแชท" />
      ) : (
        <span className="muted small-text">{failed ? "โหลดรูปไม่ได้" : "กำลังโหลดรูป…"}</span>
      )}
    </button>
  );
}

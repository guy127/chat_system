"use client";

import { useEffect } from "react";

interface Props {
  url: string;
  onClose: () => void;
}

export default function ImageViewer({ url, onClose }: Props) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div className="viewer" role="dialog" aria-modal="true" aria-label="รูปภาพ" onClick={onClose}>
      {/* eslint-disable-next-line @next/next/no-img-element -- object URL from an authenticated fetch */}
      <img src={url} alt="รูปภาพขนาดเต็ม" onClick={(e) => e.stopPropagation()} />
      <div className="viewer-actions" onClick={(e) => e.stopPropagation()}>
        <a className="button" href={url} download="smalltalk-image">
          ดาวน์โหลด
        </a>
        <button onClick={onClose}>ปิด</button>
      </div>
    </div>
  );
}

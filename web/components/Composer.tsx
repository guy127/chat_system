"use client";

import { useRef, useState, type ClipboardEvent, type FormEvent, type KeyboardEvent } from "react";

import { codeText } from "@/lib/errors";
import { ACCEPT_IMAGES, ImageError, prepareImage, type PreparedImage } from "@/lib/images";
import { MAX_BODY_LENGTH } from "@/types/chat";

interface Props {
  disabled: boolean;
  placeholder?: string;
  onSend: (body: string, image: PreparedImage | null) => void;
  onError: (message: string) => void;
}

export default function Composer({ disabled, placeholder = "พิมพ์ข้อความ…", onSend, onError }: Props) {
  const [body, setBody] = useState("");
  const [image, setImage] = useState<PreparedImage | null>(null);
  const [preparing, setPreparing] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);

  const length = [...body].length; // count characters, not UTF-16 units
  const tooLong = length > MAX_BODY_LENGTH;
  const canSend = !disabled && !preparing && !tooLong && (body.trim() !== "" || image !== null);

  const attach = async (file: File) => {
    setPreparing(true);
    try {
      const prepared = await prepareImage(file);
      if (image) URL.revokeObjectURL(image.previewURL);
      setImage(prepared);
    } catch (e) {
      onError(codeText(e instanceof ImageError ? e.code : "image_unsupported"));
    } finally {
      setPreparing(false);
    }
  };

  const clearImage = () => {
    if (image) URL.revokeObjectURL(image.previewURL);
    setImage(null);
  };

  const submit = (e?: FormEvent) => {
    e?.preventDefault();
    if (!canSend) return;
    onSend(body, image); // the pending message now owns the preview URL
    setBody("");
    setImage(null);
  };

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    // Enter sends, Shift+Enter adds a line; ignore Enter while an IME is composing (Thai input)
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      submit();
    }
  };

  const onPaste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
    const file = Array.from(e.clipboardData.files).find((f) => f.type.startsWith("image/"));
    if (file) {
      e.preventDefault();
      void attach(file);
    }
  };

  return (
    <form className="composer" onSubmit={submit}>
      {(image || preparing) && (
        <div className="attachment">
          {image ? (
            <>
              {/* eslint-disable-next-line @next/next/no-img-element -- local preview */}
              <img src={image.previewURL} alt="รูปที่จะส่ง" />
              <button type="button" className="small" onClick={clearImage} aria-label="เอารูปออก">
                ✕
              </button>
            </>
          ) : (
            <span className="muted small-text">กำลังเตรียมรูป…</span>
          )}
        </div>
      )}
      <div className="composer-row">
        <button
          type="button"
          className="icon"
          onClick={() => fileInput.current?.click()}
          disabled={disabled || preparing}
          aria-label="แนบรูป"
          title="แนบรูป (ไม่เกิน 10 MB)"
        >
          🖼️
        </button>
        <input
          ref={fileInput}
          type="file"
          accept={ACCEPT_IMAGES}
          hidden
          onChange={(e) => {
            const file = e.target.files?.[0];
            e.target.value = ""; // allow picking the same file again
            if (file) void attach(file);
          }}
        />
        <textarea
          value={body}
          onChange={(e) => setBody(e.target.value)}
          onKeyDown={onKeyDown}
          onPaste={onPaste}
          placeholder={image ? "เพิ่มคำบรรยาย (ไม่บังคับ)…" : placeholder}
          disabled={disabled}
          rows={1}
          aria-label="ข้อความ"
        />
        {length > MAX_BODY_LENGTH - 200 && (
          <span className={tooLong ? "counter over" : "counter"}>
            {length}/{MAX_BODY_LENGTH}
          </span>
        )}
        <button type="submit" className="primary" disabled={!canSend}>
          ส่ง
        </button>
      </div>
    </form>
  );
}

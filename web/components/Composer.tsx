"use client";

import { useState, type FormEvent, type KeyboardEvent } from "react";

import { MAX_BODY_LENGTH } from "@/types/chat";

interface Props {
  disabled: boolean;
  placeholder?: string;
  onSend: (body: string) => void;
}

export default function Composer({ disabled, placeholder = "พิมพ์ข้อความ…", onSend }: Props) {
  const [body, setBody] = useState("");
  const length = [...body].length; // count characters, not UTF-16 units
  const tooLong = length > MAX_BODY_LENGTH;
  const canSend = !disabled && !tooLong && body.trim() !== "";

  const submit = (e?: FormEvent) => {
    e?.preventDefault();
    if (!canSend) return;
    onSend(body);
    setBody("");
  };

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    // Enter sends, Shift+Enter adds a line; ignore Enter while an IME is composing (Thai input)
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      submit();
    }
  };

  return (
    <form className="composer" onSubmit={submit}>
      <textarea
        value={body}
        onChange={(e) => setBody(e.target.value)}
        onKeyDown={onKeyDown}
        placeholder={placeholder}
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
    </form>
  );
}

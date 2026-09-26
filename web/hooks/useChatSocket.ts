"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { wsURL } from "@/lib/api";
import type { ClientFrame, EndReason, ServerFrame } from "@/types/chat";

export type SocketStatus = "connecting" | "open" | "reconnecting" | "ended";

interface Options {
  roomId: string;
  jwt: string | null;
  /** Newest stored message id; sent as `resume` so the server replays what we missed. */
  getLastMessageId: () => string | undefined;
  onFrame: (frame: ServerFrame) => void;
  /** Called on every (re)connect; `resumed` is false when there was nothing to resume from. */
  onOpen?: (resumed: boolean) => void;
}

const PING_INTERVAL = 25_000; // server drops a silent connection after 60 s
const MAX_BACKOFF = 30_000;

const closeCodeReason: Record<number, EndReason> = {
  4001: "unauthorized",
  4003: "kicked",
  4004: "room_closed",
};

/**
 * All WebSocket behaviour lives here: auth via subprotocol, resume after
 * reconnect, exponential backoff (1, 2, 4, 8 … 30 s), keep-alive pings and an
 * outbox that re-sends unacknowledged messages with the same client_msg_id.
 */
export function useChatSocket({ roomId, jwt, getLastMessageId, onFrame, onOpen }: Options) {
  const [status, setStatus] = useState<SocketStatus>("connecting");
  const [endReason, setEndReason] = useState<EndReason | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const outbox = useRef(new Map<string, ClientFrame>());
  const handlers = useRef({ getLastMessageId, onFrame, onOpen });
  const reconnectNow = useRef<() => void>(() => {});

  useEffect(() => {
    handlers.current = { getLastMessageId, onFrame, onOpen };
  });

  useEffect(() => {
    if (!jwt) return;
    let stopped = false;
    let attempt = 0;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    let pingTimer: ReturnType<typeof setInterval> | undefined;

    const end = (reason: EndReason) => {
      stopped = true;
      setEndReason((prev) => prev ?? reason);
    };

    const connect = () => {
      clearTimeout(retryTimer);
      const ws = new WebSocket(wsURL(roomId), ["chat", jwt]);
      wsRef.current = ws;

      ws.onopen = () => {
        attempt = 0;
        setStatus("open");
        const last = handlers.current.getLastMessageId();
        if (last) ws.send(JSON.stringify({ type: "resume", last_message_id: last } satisfies ClientFrame));
        handlers.current.onOpen?.(Boolean(last));
        for (const frame of outbox.current.values()) ws.send(JSON.stringify(frame));
        pingTimer = setInterval(() => {
          if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: "ping" } satisfies ClientFrame));
        }, PING_INTERVAL);
      };

      ws.onmessage = (ev) => {
        let frame: ServerFrame;
        try {
          frame = JSON.parse(ev.data as string) as ServerFrame;
        } catch {
          return;
        }
        if (frame.type === "ack") outbox.current.delete(frame.client_msg_id);
        if (frame.type === "error" && frame.client_msg_id) outbox.current.delete(frame.client_msg_id);
        if (frame.type === "kicked") end(frame.reason);
        if (frame.type === "room_closed") end("room_closed");
        if (
          frame.type === "error" &&
          (frame.code === "kicked" || frame.code === "banned" || frame.code === "room_closed")
        ) {
          end(frame.code);
        }
        handlers.current.onFrame(frame);
      };

      ws.onclose = (ev) => {
        clearInterval(pingTimer);
        if (wsRef.current === ws) wsRef.current = null;
        const reason = closeCodeReason[ev.code];
        if (reason) end(reason);
        if (stopped) {
          setStatus("ended");
          return;
        }
        setStatus("reconnecting");
        const delay = Math.min(1000 * 2 ** attempt, MAX_BACKOFF);
        attempt++;
        retryTimer = setTimeout(connect, delay);
      };
    };

    // Skip the backoff wait when the network comes back or the tab is shown again.
    reconnectNow.current = () => {
      if (stopped || wsRef.current) return;
      attempt = 0;
      connect();
    };
    const onVisible = () => document.visibilityState === "visible" && reconnectNow.current();
    const onOnline = () => reconnectNow.current();
    window.addEventListener("online", onOnline);
    document.addEventListener("visibilitychange", onVisible);

    connect();
    return () => {
      stopped = true;
      clearTimeout(retryTimer);
      clearInterval(pingTimer);
      window.removeEventListener("online", onOnline);
      document.removeEventListener("visibilitychange", onVisible);
      wsRef.current?.close(1000);
      wsRef.current = null;
    };
  }, [roomId, jwt]);

  const send = useCallback((clientMsgId: string, body: string) => {
    const frame: ClientFrame = { type: "send", client_msg_id: clientMsgId, body };
    outbox.current.set(clientMsgId, frame);
    const ws = wsRef.current;
    if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify(frame));
  }, []);

  return { status: jwt ? status : ("ended" as const), endReason, send };
}

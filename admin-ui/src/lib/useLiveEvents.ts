import { useEffect, useRef } from "react";

export type LiveEvent = { type: string; data?: unknown };

export function useLiveEvents(
  handler: (event: LiveEvent) => void,
  types: string[],
  throttleMs = 750,
) {
  const handlerRef = useRef(handler);
  handlerRef.current = handler;
  const typeKey = [...types].sort().join("|");

  useEffect(() => {
    let closed = false;
    let socket: WebSocket | undefined;
    let reconnect: number | undefined;
    let trailing: number | undefined;
    let latest: LiveEvent | undefined;
    let lastRun = 0;
    let attempts = 0;
    const accepted = new Set(typeKey.split("|").filter(Boolean));
    const dispatch = (event: LiveEvent) => {
      latest = event;
      const elapsed = Date.now() - lastRun;
      if (elapsed >= throttleMs) {
        lastRun = Date.now();
        handlerRef.current(event);
        latest = undefined;
      } else if (trailing === undefined) {
        trailing = window.setTimeout(() => {
          trailing = undefined;
          if (latest) {
            lastRun = Date.now();
            handlerRef.current(latest);
            latest = undefined;
          }
        }, throttleMs - elapsed);
      }
    };
    const connect = () => {
      if (closed) return;
      const protocol = location.protocol === "https:" ? "wss" : "ws";
      socket = new WebSocket(`${protocol}://${location.host}/admin/ws`);
      socket.onopen = () => {
        attempts = 0;
        dispatch({ type: "sync" });
      };
      socket.onmessage = (message) => {
        try {
          const event = JSON.parse(message.data) as LiveEvent;
          if (accepted.has(event.type)) dispatch(event);
        } catch {
          // A reconnect and periodic sync repair malformed or missed messages.
        }
      };
      socket.onerror = () => socket?.close();
      socket.onclose = () => {
        if (closed) return;
        const delay = Math.min(750 * 1.8 ** attempts++, 12000);
        reconnect = window.setTimeout(connect, delay);
      };
    };
    connect();
    const fallback = window.setInterval(
      () => dispatch({ type: "sync" }),
      15000,
    );
    return () => {
      closed = true;
      socket?.close();
      if (reconnect !== undefined) clearTimeout(reconnect);
      if (trailing !== undefined) clearTimeout(trailing);
      clearInterval(fallback);
    };
  }, [typeKey, throttleMs]);
}

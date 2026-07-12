import { useEffect, useRef } from "react";

interface UseSSEOptions {
  /** Absolute or root-relative URL for the SSE endpoint (`/api/v1/events/stream`). */
  url: string;
  enabled?: boolean;
  /** Called for every named event the server sends (`metrics`, `players`, `event`). */
  onMessage: (eventName: string, data: unknown) => void;
  /** Invoked immediately and whenever the stream is unavailable, so callers can keep polling. */
  onFallback?: () => void;
}

/**
 * Subscribes to the panel's Server-Sent Events stream when the browser supports it and the
 * endpoint is reachable; otherwise (or on any stream error) calls `onFallback` so the caller's
 * existing polling (react-query `refetchInterval`) keeps the UI live. EventSource's native
 * reconnect remains enabled after errors; polling covers the disconnected interval.
 */
export function useSSE({ url, enabled = true, onMessage, onFallback }: UseSSEOptions): void {
  const onMessageRef = useRef(onMessage);
  const onFallbackRef = useRef(onFallback);
  onMessageRef.current = onMessage;
  onFallbackRef.current = onFallback;

  useEffect(() => {
    if (!enabled) return;
    if (typeof EventSource === "undefined") {
      onFallbackRef.current?.();
      return;
    }

    let es: EventSource | null = null;
    try {
      es = new EventSource(url, { withCredentials: true });
      const handle = (evt: MessageEvent<string>) => {
        try {
          onMessageRef.current(evt.type, JSON.parse(evt.data));
        } catch {
          // malformed payload — ignore this event
        }
      };
      es.addEventListener("metrics", handle);
      es.addEventListener("players", handle);
      es.addEventListener("event", handle);
      es.onerror = () => {
        onFallbackRef.current?.();
      };
    } catch {
      onFallbackRef.current?.();
    }

    return () => es?.close();
  }, [url, enabled]);
}

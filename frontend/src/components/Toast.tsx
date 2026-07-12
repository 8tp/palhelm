import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from "react";

export type ToastTone = "info" | "ok" | "danger";

interface ToastItem {
  id: number;
  tone: ToastTone;
  message: string;
}

interface ToastContextValue {
  push: (message: string, tone?: ToastTone) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

const TOAST_MS = 4200;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const idRef = useRef(0);
  // WCAG 2.2.1: pausable timing. Hovering/focusing a toast clears its dismiss timer;
  // leaving reschedules a fresh TOAST_MS window rather than a precise remainder.
  const timers = useRef(new Map<number, number>());

  const dismiss = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
    timers.current.delete(id);
  }, []);

  const schedule = useCallback(
    (id: number) => {
      timers.current.set(id, window.setTimeout(() => dismiss(id), TOAST_MS));
    },
    [dismiss],
  );

  const push = useCallback(
    (message: string, tone: ToastTone = "info") => {
      const id = (idRef.current += 1);
      setToasts((prev) => [...prev, { id, tone, message }]);
      schedule(id);
    },
    [schedule],
  );

  return (
    <ToastContext.Provider value={{ push }}>
      {children}
      <div className="toast-stack">
        {toasts.map((t) => (
          <div
            key={t.id}
            className={`toast toast-${t.tone}`}
            // Danger toasts get their own assertive region so a rapid failure can't be
            // swallowed by the next info/ok toast sharing a single polite region.
            role={t.tone === "danger" ? "alert" : "status"}
            aria-live={t.tone === "danger" ? "assertive" : "polite"}
            tabIndex={0}
            onMouseEnter={() => window.clearTimeout(timers.current.get(t.id))}
            onMouseLeave={() => schedule(t.id)}
            onFocus={() => window.clearTimeout(timers.current.get(t.id))}
            onBlur={() => schedule(t.id)}
          >
            {t.message}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used within ToastProvider");
  return ctx;
}

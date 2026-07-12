import { createContext, useContext, useMemo, useRef, useState, type ReactNode } from "react";

export type PlayerActionKind = "kick" | "ban" | "unban";

interface PlayerActionRequest {
  kind: PlayerActionKind;
  uid: string;
  /** Unique per call so the same uid+kind requested twice in a row still re-fires. */
  nonce: number;
}

interface ConsoleInsertRequest {
  command: string;
  nonce: number;
}

interface PaletteBridgeValue {
  /** Consumed by Players.tsx to open its existing kick/ban/unban ConfirmDialog — the
   *  command palette can request an action from any route without owning any dialog
   *  or mutation logic itself. */
  playerActionRequest: PlayerActionRequest | null;
  requestPlayerAction: (kind: PlayerActionKind, uid: string) => void;
  clearPlayerActionRequest: () => void;

  /** Consumed by Console.tsx to insert (never execute) a saved command into its input. */
  consoleInsertRequest: ConsoleInsertRequest | null;
  requestConsoleInsert: (command: string) => void;
  clearConsoleInsertRequest: () => void;

  /** HelmStrip is always mounted while authenticated, so its Broadcast dialog opener
   *  registers itself here once and the palette can call it directly — no navigation
   *  or duplicate dialog needed. */
  registerOpenBroadcast: (fn: () => void) => () => void;
  openBroadcast: () => void;
}

const PaletteBridgeContext = createContext<PaletteBridgeValue | null>(null);

export function PaletteBridgeProvider({ children }: { children: ReactNode }) {
  const [playerActionRequest, setPlayerActionRequest] = useState<PlayerActionRequest | null>(null);
  const [consoleInsertRequest, setConsoleInsertRequest] = useState<ConsoleInsertRequest | null>(null);
  // Imperative registration only, so a ref rather than state: HelmStrip's registering effect
  // (`useEffect(() => bridge.registerOpenBroadcast(...), [bridge])`) would otherwise re-fire
  // forever — storing the opener in state changes `value`'s identity on every register call,
  // which re-triggers that same effect, which registers again, ad infinitum.
  const openBroadcastRef = useRef<(() => void) | null>(null);

  const value = useMemo<PaletteBridgeValue>(
    () => ({
      playerActionRequest,
      requestPlayerAction: (kind, uid) => setPlayerActionRequest({ kind, uid, nonce: Date.now() + Math.random() }),
      clearPlayerActionRequest: () => setPlayerActionRequest(null),

      consoleInsertRequest,
      requestConsoleInsert: (command) => setConsoleInsertRequest({ command, nonce: Date.now() + Math.random() }),
      clearConsoleInsertRequest: () => setConsoleInsertRequest(null),

      registerOpenBroadcast: (fn) => {
        openBroadcastRef.current = fn;
        return () => {
          if (openBroadcastRef.current === fn) openBroadcastRef.current = null;
        };
      },
      openBroadcast: () => openBroadcastRef.current?.(),
    }),
    [playerActionRequest, consoleInsertRequest],
  );

  return <PaletteBridgeContext.Provider value={value}>{children}</PaletteBridgeContext.Provider>;
}

export function usePaletteBridge(): PaletteBridgeValue {
  const ctx = useContext(PaletteBridgeContext);
  if (!ctx) throw new Error("usePaletteBridge must be used within PaletteBridgeProvider");
  return ctx;
}

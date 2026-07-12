import { Fragment } from "react";

/**
 * Renders an event message with a tiny inline markup so fixture/API text can carry the same
 * emphasis the mockups use: `**name**` for a bolded player name, `` `value` `` for a mono data
 * value (byte size, world id), and `~value~` for a muted mono value (Steam id).
 */
export function EventMessage({ text }: { text: string }) {
  const parts = text.split(/(\*\*[^*]+\*\*|`[^`]+`|~[^~]+~)/g).filter((p) => p !== "");
  return (
    <>
      {parts.map((part, i) => {
        if (part.startsWith("**") && part.endsWith("**")) {
          return <strong key={i}>{part.slice(2, -2)}</strong>;
        }
        if (part.startsWith("`") && part.endsWith("`")) {
          return (
            <span key={i} className="num">
              {part.slice(1, -1)}
            </span>
          );
        }
        if (part.startsWith("~") && part.endsWith("~")) {
          return (
            <span key={i} className="num" style={{ color: "var(--ink-3)" }}>
              {part.slice(1, -1)}
            </span>
          );
        }
        return <Fragment key={i}>{part}</Fragment>;
      })}
    </>
  );
}

import type { ReactNode } from "react";

export function CodeWell({ children }: { children: ReactNode }) {
  return <div className="code-well">{children}</div>;
}

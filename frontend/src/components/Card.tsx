import type { ReactNode } from "react";

export function Card({
  children,
  className = "",
  span2 = false,
  style,
}: {
  children: ReactNode;
  className?: string;
  span2?: boolean;
  style?: React.CSSProperties;
}) {
  return (
    <div className={["card", span2 ? "span-2" : "", className].filter(Boolean).join(" ")} style={style}>
      {children}
    </div>
  );
}

export function CardHead({ title, hint, children }: { title: ReactNode; hint?: ReactNode; children?: ReactNode }) {
  return (
    <div className="card-head">
      <h2>{title}</h2>
      {hint && <span className="hint">{hint}</span>}
      <div className="spacer" />
      {children}
    </div>
  );
}

export function CardBody({
  children,
  flush = false,
  chart = false,
  className = "",
  style,
}: {
  children: ReactNode;
  flush?: boolean;
  chart?: boolean;
  className?: string;
  style?: React.CSSProperties;
}) {
  const cls = ["card-body", flush ? "flush" : "", chart ? "chart" : "", className].filter(Boolean).join(" ");
  return (
    <div className={cls} style={style}>
      {children}
    </div>
  );
}

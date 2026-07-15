// Inline SVG icons extracted from the design mockups. 16px, stroke currentColor unless noted.
import type { SVGProps } from "react";

export type IconProps = SVGProps<SVGSVGElement>;

function base(props: IconProps) {
  return { width: 16, height: 16, viewBox: "0 0 16 16", fill: "none", "aria-hidden": true, ...props };
}

/**
 * The sphere-wheel mark (the sphere-wheel mark): the helm wheel's 8 radial
 * ticks + ring, fused with a capture sphere — a diagonal split with a leaf-green (var(--sphere))
 * upper-left half and a small offset diamond window. No equatorial band, no center button, no
 * gold (--brass retired from the mark). Line work (ring / ticks / split) takes the context color
 * var(--accent), as the old mark did. Geometry follows the sphere-wheel mark — the
 * rail uses 26px, login's hero 72px; pass `strokeWidth` to tune line weight. `dotRadius` is a
 * retained no-op prop (the v3 mark has no center dot) so existing call sites need no change. Wrap
 * in `wheelClassName="wheel"` to target the group for the login stamp-press / page-loader spin
 * animations (see Login.css / app.css `.page-loader`).
 */
export function HelmMark({
  size = 26,
  strokeWidth = 1.8,
  dotRadius: _dotRadius = 1.2,
  wheelClassName,
  ...props
}: IconProps & { size?: number; strokeWidth?: number; dotRadius?: number; wheelClassName?: string }) {
  const geometry = (
    <>
      <path d="M6.99 19.01 A8.5 8.5 0 1 1 19.01 6.99 Z" fill="var(--sphere)" />
      <path d="M6.99 19.01 L19.01 6.99" stroke="var(--accent)" strokeWidth={strokeWidth} strokeLinecap="round" />
      <rect x="15.1" y="15.1" width="3" height="3" rx="0.6" transform="rotate(45 16.6 16.6)" fill="var(--sphere)" />
      <circle cx="13" cy="13" r="8.5" stroke="var(--accent)" strokeWidth={strokeWidth} />
      <g stroke="var(--accent)" strokeWidth={strokeWidth} strokeLinecap="round">
        <path d="M13 1.5v4" />
        <path d="M13 20.5v4" />
        <path d="M1.5 13h4" />
        <path d="M20.5 13h4" />
        <path d="M4.87 4.87l2.83 2.83" />
        <path d="M18.3 18.3l2.83 2.83" />
        <path d="M21.13 4.87l-2.83 2.83" />
        <path d="M7.7 18.3l-2.83 2.83" />
      </g>
    </>
  );
  return (
    <svg width={size} height={size} viewBox="0 0 26 26" fill="none" aria-hidden="true" {...props}>
      {wheelClassName ? <g className={wheelClassName}>{geometry}</g> : geometry}
    </svg>
  );
}

export function IconOverview(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <path d="M2 9a6 6 0 1 1 12 0" />
      <path d="M8 9l3-3" />
      <path d="M2.5 12h11" />
    </svg>
  );
}

export function IconPlayers(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <circle cx="6" cy="5" r="2.5" />
      <path d="M1.5 13.5c0-2.5 2-4 4.5-4s4.5 1.5 4.5 4" />
      <path d="M11 3.2a2.5 2.5 0 0 1 0 4.6" />
      <path d="M12.5 9.8c1.2.6 2 1.7 2 3.2" />
    </svg>
  );
}

export function IconActivity(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M2 13.5V2.5M2 13.5h12" />
      <path d="m4 10 2.5-3 2.2 1.8L12.5 4" />
      <circle cx="4" cy="10" r=".7" fill="currentColor" stroke="none" />
      <circle cx="6.5" cy="7" r=".7" fill="currentColor" stroke="none" />
      <circle cx="8.7" cy="8.8" r=".7" fill="currentColor" stroke="none" />
      <circle cx="12.5" cy="4" r=".7" fill="currentColor" stroke="none" />
    </svg>
  );
}

export function IconMap(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round">
      <path d="M1.5 3.5l4-1.5 5 1.5 4-1.5v10l-4 1.5-5-1.5-4 1.5z" />
      <path d="M5.5 2v10.5M10.5 3.5V14" />
    </svg>
  );
}

export function IconMapPlayer(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <circle cx="8" cy="5" r="2.25" />
      <path d="M3.7 13.5c.2-2.5 1.9-4 4.3-4s4.1 1.5 4.3 4" />
    </svg>
  );
}

export function IconPals(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round">
      <path d="M4.2 5.3 2.8 2.2l3.4 1.5M11.8 5.3l1.4-3.1-3.4 1.5" />
      <path d="M3.6 7.4c0-2.5 1.9-4.2 4.4-4.2s4.4 1.7 4.4 4.2v1.3c0 2.6-1.9 4.5-4.4 4.5s-4.4-1.9-4.4-4.5z" />
      <path d="M5.8 8.2h.1M10.1 8.2h.1M6.4 10.6c1 .7 2.2.7 3.2 0" />
    </svg>
  );
}

export function IconMapWorker(props: IconProps) {
  return <IconPals {...props} />;
}

export function IconMapBase(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="m2.2 7.2 5.8-5 5.8 5" />
      <path d="M3.8 6.2v7.3h8.4V6.2M6.5 13.5V9.8h3v3.7" />
    </svg>
  );
}

export function IconMapPalBox(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <rect x="2.2" y="4" width="11.6" height="9.2" rx="1.2" />
      <path d="M5 4V2.5h6V4M2.2 7.1h11.6M6.4 9.7h3.2" />
    </svg>
  );
}

export function IconZoomIn(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <circle cx="7" cy="7" r="4.5" />
      <path d="m10.3 10.3 3.4 3.4M7 4.8v4.4M4.8 7h4.4" />
    </svg>
  );
}

export function IconZoomOut(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <circle cx="7" cy="7" r="4.5" />
      <path d="m10.3 10.3 3.4 3.4M4.8 7h4.4" />
    </svg>
  );
}

export function IconFitView(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M6 2.5H2.5V6M10 2.5h3.5V6M6 13.5H2.5V10M10 13.5h3.5V10" />
      <circle cx="8" cy="8" r="1.5" />
    </svg>
  );
}

export function IconConsole(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <path d="M3 5l3 3-3 3" />
      <path d="M8 11.5h5" />
    </svg>
  );
}

export function IconEvents(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <path d="M3 2.5h10v11H3z" />
      <path d="M5.5 5h5M5.5 8h5M5.5 11h3" />
    </svg>
  );
}

export function IconBackups(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <rect x="2" y="2.5" width="12" height="3.5" rx="0.5" />
      <path d="M3 6v6.5a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1V6" />
      <path d="M6.5 9h3" />
    </svg>
  );
}

export function IconConfig(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <path d="M2 5h8M12.5 5H14M2 11h2M6.5 11H14" />
      <circle cx="10.5" cy="5" r="1.8" />
      <circle cx="4.5" cy="11" r="1.8" />
    </svg>
  );
}

export function IconSettings(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <circle cx="8" cy="8" r="2.2" />
      <path d="M8 1.8v2M8 12.2v2M1.8 8h2M12.2 8h2M3.6 3.6l1.4 1.4M11 11l1.4 1.4M12.4 3.6L11 5M5 11l-1.4 1.4" />
    </svg>
  );
}

export function IconSearch(props: IconProps) {
  return (
    <svg width={14} height={14} viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true" {...props}>
      <circle cx="6" cy="6" r="4.2" />
      <path d="M9.5 9.5L13 13" strokeLinecap="round" />
    </svg>
  );
}

export function IconWarn(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M8 1.5l7 12.5H1z" />
      <path d="M8 6.2v3.2" />
      <circle cx="8" cy="11.6" r="0.6" fill="currentColor" stroke="none" />
    </svg>
  );
}

export function IconInfo(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <circle cx="8" cy="8" r="6.3" />
      <path d="M8 7.2v4M8 5.1v.05" />
    </svg>
  );
}

export function IconKebab(props: IconProps) {
  return (
    <svg {...base(props)} fill="currentColor" aria-hidden="true">
      <circle cx="8" cy="3.4" r="1.3" />
      <circle cx="8" cy="8" r="1.3" />
      <circle cx="8" cy="12.6" r="1.3" />
    </svg>
  );
}

export function IconClose(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <path d="M3.5 3.5l9 9M12.5 3.5l-9 9" />
    </svg>
  );
}

export function IconChevronDown(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M4 6l4 4 4-4" />
    </svg>
  );
}

export function IconChevronLeft(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M10 4l-4 4 4 4" />
    </svg>
  );
}

export function IconChevronRight(props: IconProps) {
  return (
    <svg {...base(props)} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M6 4l4 4-4 4" />
    </svg>
  );
}

export function IconMapEmpty(props: IconProps) {
  return (
    <svg width={40} height={40} viewBox="0 0 40 40" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" aria-hidden="true" {...props}>
      <path d="M4 9l11-4 14 4 11-4v26l-11 4-14-4-11 4z" />
      <path d="M15 5v26M26 9v26" />
      <path d="M8 34l24-24" strokeDasharray="2 3" />
    </svg>
  );
}

export function IconEmptyGeneric(props: IconProps) {
  return (
    <svg width={40} height={40} viewBox="0 0 40 40" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" aria-hidden="true" {...props}>
      <rect x="6" y="10" width="28" height="22" rx="2" />
      <path d="M6 17h28" />
      <path d="M13 24h6M13 28h10" />
    </svg>
  );
}

export function IconTerminal(props: IconProps) {
  return (
    <svg width={40} height={40} viewBox="0 0 40 40" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" aria-hidden="true" {...props}>
      <rect x="4" y="7" width="32" height="26" rx="2" />
      <path d="M11 17l6 5-6 5" strokeLinecap="round" />
      <path d="M21 27h8" strokeLinecap="round" />
    </svg>
  );
}

export function IconArchive(props: IconProps) {
  return (
    <svg width={40} height={40} viewBox="0 0 40 40" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" aria-hidden="true" {...props}>
      <rect x="5" y="6" width="30" height="8" rx="1" />
      <path d="M7 14v16a2 2 0 0 0 2 2h22a2 2 0 0 0 2-2V14" />
      <path d="M16 21h8" strokeLinecap="round" />
    </svg>
  );
}

export function IconSliders(props: IconProps) {
  return (
    <svg width={40} height={40} viewBox="0 0 40 40" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" aria-hidden="true" {...props}>
      <path d="M6 13h20M30 13h4M6 27h6M16 27h18" />
      <circle cx="26" cy="13" r="4" />
      <circle cx="12" cy="27" r="4" />
    </svg>
  );
}

export function IconGear(props: IconProps) {
  return (
    <svg width={40} height={40} viewBox="0 0 40 40" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" aria-hidden="true" {...props}>
      <circle cx="20" cy="20" r="5.5" />
      <path d="M20 4.5v5M20 30.5v5M4.5 20h5M30.5 20h5M9 9l3.5 3.5M27.5 27.5L31 31M31 9l-3.5 3.5M12.5 27.5L9 31" />
    </svg>
  );
}

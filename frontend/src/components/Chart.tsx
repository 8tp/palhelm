import { useEffect, useLayoutEffect, useRef } from "react";
import uPlot from "uplot";
import "uplot/dist/uPlot.min.css";

export interface ChartProps {
  /** uPlot aligned data: `[xs, ys]`. */
  data: uPlot.AlignedData;
  height?: number;
  ariaLabel: string;
  xFormat?: (v: number) => string;
  yFormat?: (v: number) => string;
  /** Fixed y-axis range, e.g. `[0, 70]` for an FPS chart. Omit to auto-range from data. */
  yRange?: [number, number];
  /** Data index + label to annotate in-chart (axis ink, no extra color) — e.g. "48 fps · world save". */
  annotation?: { index: number; text: string };
  /** Hide the bottom time axis — for compact secondary charts stacked under a primary chart. */
  xAxis?: boolean;
}

function readVar(name: string, fallback: string): string {
  if (typeof window === "undefined") return fallback;
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return v || fallback;
}

/**
 * uPlot wrapper matching the mockup chart aesthetic (design/mockups/ui.css `.chart`): faint grid,
 * mono axis labels, a 2px accent line with soft area fill under it, and an emphasized dot on the
 * most recent point. One y-axis only — never dual-axis (see design/README.md dataviz rules).
 */
export function Chart({ data, height = 180, ariaLabel, xFormat, yFormat, yRange, annotation, xAxis = true }: ChartProps) {
  const hostRef = useRef<HTMLDivElement>(null);
  const plotRef = useRef<uPlot | null>(null);
  const annotationRef = useRef<HTMLDivElement>(null);
  const annotationInfoRef = useRef(annotation);
  annotationInfoRef.current = annotation;

  useLayoutEffect(() => {
    const host = hostRef.current;
    if (!host) return;

    const chartLine = readVar("--chart-line", "#5c7030");
    const chartFill = readVar("--chart-fill", "rgba(92,112,48,0.14)");
    const chartGrid = readVar("--chart-grid", "rgba(42,36,20,0.10)");
    const inkMuted = readVar("--ink-3", "#665c40");
    const surface = readVar("--surface", "#faf5e6");
    const fontMono = '10px "IBM Plex Mono", ui-monospace, monospace';

    const width = host.clientWidth || 600;

    const opts: uPlot.Options = {
      width,
      height,
      cursor: { points: { show: true }, drag: { x: false, y: false } },
      legend: { show: false },
      scales: {
        x: { time: false },
        y: yRange ? { range: yRange } : { range: (_u, min, max) => [Math.min(0, min), max * 1.08] },
      },
      axes: [
        {
          show: xAxis,
          stroke: inkMuted,
          grid: { show: false },
          ticks: { show: false },
          font: fontMono,
          values: (_u, splits) => splits.map((s) => (xFormat ? xFormat(s) : String(s))),
        },
        {
          stroke: inkMuted,
          grid: { stroke: chartGrid, width: 1 },
          ticks: { show: false },
          size: 34,
          font: fontMono,
          values: (_u, splits) => splits.map((s) => (yFormat ? yFormat(s) : String(s))),
        },
      ],
      series: [
        {},
        {
          stroke: chartLine,
          width: 2,
          fill: chartFill,
          points: { show: false },
        },
      ],
      hooks: {
        draw: [
          (u) => {
            const xs = u.data[0];
            const ys = u.data[1];
            if (!xs || xs.length === 0) return;
            const lastIdx = xs.length - 1;
            const xVal = xs[lastIdx];
            const yVal = ys?.[lastIdx];
            if (xVal == null || yVal == null) return;
            const cx = u.valToPos(xVal, "x", true);
            const cy = u.valToPos(yVal as number, "y", true);
            const ctx = u.ctx;
            ctx.save();
            ctx.beginPath();
            ctx.arc(cx, cy, 3.5, 0, Math.PI * 2);
            ctx.fillStyle = chartLine;
            ctx.fill();
            ctx.lineWidth = 2;
            ctx.strokeStyle = surface;
            ctx.stroke();
            ctx.restore();

            const ann = annotationInfoRef.current;
            const el = annotationRef.current;
            if (el) {
              if (ann && xs[ann.index] != null && ys?.[ann.index] != null) {
                const ax = u.valToPos(xs[ann.index] as number, "x", true);
                const ay = u.valToPos(ys[ann.index] as number, "y", true);
                el.style.display = "block";
                el.style.left = `${ax}px`;
                // Below the annotated point — a dip leaves clear space beneath it (mockup places
                // "48 fps · world save" under the dip, never on the line).
                el.style.top = `${Math.min(u.height - 26, ay + 10)}px`;
                el.textContent = ann.text;
              } else {
                el.style.display = "none";
              }
            }
          },
        ],
      },
    };

    const plot = new uPlot(opts, data, host);
    plotRef.current = plot;
    host.setAttribute("role", "img");
    host.setAttribute("aria-label", ariaLabel);

    const ro = new ResizeObserver(() => {
      if (!hostRef.current) return;
      const w = hostRef.current.clientWidth;
      if (w > 0) plot.setSize({ width: w, height });
    });
    ro.observe(host);

    return () => {
      ro.disconnect();
      plot.destroy();
      plotRef.current = null;
    };
    // Rebuilt when the frame geometry, axis mode, or y-range changes; data updates flow through
    // setData below so we don't tear down (and lose the canvas) on every poll.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [height, ariaLabel, xAxis, yRange?.[0], yRange?.[1]]);

  useEffect(() => {
    plotRef.current?.setData(data);
  }, [data]);

  return (
    <div className="uplot-wrap">
      <div ref={hostRef} className="uplot-host" />
      <div ref={annotationRef} className="chart-annotation" aria-hidden="true" />
    </div>
  );
}

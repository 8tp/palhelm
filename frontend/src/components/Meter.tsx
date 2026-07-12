export function Meter({ value, max }: { value: number; max: number }) {
  const pct = max > 0 ? Math.max(0, Math.min(100, (value / max) * 100)) : 0;
  return (
    <div className="meter">
      <div className="fill" style={{ width: `${pct}%` }} />
    </div>
  );
}

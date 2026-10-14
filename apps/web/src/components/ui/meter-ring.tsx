export function MeterRing({ value, label }: { value: number; label: string }) {
  const amount = Math.min(100, Math.max(0, Number.isFinite(value) ? value : 0));
  return (
    <div className="meter-ring" role="img" aria-label={`${label}: ${Math.round(amount)}%`}>
      <svg viewBox="0 0 80 80" aria-hidden="true">
        <circle cx="40" cy="40" r="34" className="meter-track" />
        <circle
          cx="40"
          cy="40"
          r="34"
          className="meter-value"
          pathLength="100"
          strokeDasharray={`${amount} 100`}
        />
      </svg>
      <span aria-hidden="true">
        {Math.round(amount)}
        <small>%</small>
      </span>
    </div>
  );
}

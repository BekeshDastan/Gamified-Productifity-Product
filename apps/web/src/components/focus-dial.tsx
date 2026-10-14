import type { CSSProperties } from "react";

/** Display only: elapsed time and boundaries always come from the server clock. */
export function FocusDial({
  seconds,
  progress = 0,
  running = false,
  caption = "Время для себя",
  label = "Таймер фокуса",
}: {
  seconds: number;
  progress?: number;
  running?: boolean;
  caption?: string;
  label?: string;
}) {
  const safe = Math.max(0, Math.floor(Number.isFinite(seconds) ? seconds : 0));
  const hours = Math.floor(safe / 3600);
  const minutes = Math.floor((safe % 3600) / 60);
  const value = `${hours ? `${hours}:` : ""}${String(minutes).padStart(2, "0")}:${String(safe % 60).padStart(2, "0")}`;
  return (
    <div
      className={`focus-dial${running ? " is-running" : ""}`}
      style={{ "--dial-progress": Math.min(1, Math.max(0, progress)) } as CSSProperties}
    >
      <svg viewBox="0 0 320 320" aria-hidden="true">
        <circle className="dial-track" cx="160" cy="160" r="148" />
        <circle className="dial-progress" cx="160" cy="160" r="148" pathLength="1" />
        {Array.from({ length: 60 }, (_, i) => (
          <line
            key={i}
            className={i % 5 === 0 ? "dial-tick major" : "dial-tick"}
            x1="160"
            y1="28"
            x2="160"
            y2={i % 5 === 0 ? "40" : "34"}
            transform={`rotate(${i * 6} 160 160)`}
          />
        ))}
        <circle
          className="dial-orbit"
          cx="160"
          cy="12"
          r="6"
          transform={`rotate(${Math.min(1, Math.max(0, progress)) * 360} 160 160)`}
        />
      </svg>
      <div className="dial-center">
        <span className="dial-caption">{caption}</span>
        <output aria-label={label} aria-live="off">
          {value}
        </output>
        <span className="dial-bottom">
          <i />
          {running ? "Вы в потоке" : "Один шаг за раз"}
        </span>
      </div>
    </div>
  );
}

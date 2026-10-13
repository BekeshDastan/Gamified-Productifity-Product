import type { FocusSession } from "./focus";
// This projection is display-only. Every durable second comes from server segments.
export function projectFocus(f: FocusSession, serverMs: number, elapsedMs: number) {
  let active = f.active_seconds,
    rest = f.break_seconds,
    remaining = f.phase_remaining_seconds ?? 0,
    boundaryPassed = false;
  if (f.state === "running") {
    const open = f.segments.find((s) => s.ended_at === null),
      kind = open?.kind ?? f.phase;
    let cutoff = Date.parse(f.started_at) + 8 * 60 * 60 * 1000;
    if (kind === "work")
      cutoff = Math.min(cutoff, serverMs + Math.max(0, 4 * 60 * 60 - active) * 1000);
    if (f.phase_deadline) cutoff = Math.min(cutoff, Date.parse(f.phase_deadline));
    const now = serverMs + Math.max(0, elapsedMs),
      extra = Math.max(0, Math.floor((Math.min(now, cutoff) - serverMs) / 1000));
    if (kind === "work") active += extra;
    else rest += extra;
    remaining = Math.max(0, Math.ceil((cutoff - now) / 1000));
    boundaryPassed = now >= cutoff;
  }
  return { active, rest, remaining, boundaryPassed };
}
export function duration(seconds: number) {
  const n = Math.max(0, Math.floor(seconds)),
    h = Math.floor(n / 3600),
    m = Math.floor((n % 3600) / 60),
    s = n % 60;
  return h
    ? `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`
    : `${m}:${String(s).padStart(2, "0")}`;
}

import { describe, it, expect } from "vitest";
import { projectFocus, duration } from "@/lib/focus-clock";
import type { FocusSession } from "@/lib/focus";
const now = Date.parse("2026-10-12T12:00:00Z");
function session(patch: Partial<FocusSession> = {}): FocusSession {
  return {
    id: "11111111-1111-4111-8111-111111111111",
    state: "running",
    mode: "pomodoro",
    phase: "work",
    title: "Один шаг",
    quest_id: null,
    goal_id: null,
    area_id: null,
    timezone: "UTC",
    work_minutes: 5,
    break_minutes: 1,
    sound_enabled: false,
    cycle: 1,
    version: 1,
    started_at: new Date(now - 60000).toISOString(),
    updated_at: new Date(now).toISOString(),
    ended_at: null,
    phase_deadline: new Date(now + 240000).toISOString(),
    phase_remaining_seconds: null,
    result: "",
    comment: "",
    active_seconds: 60,
    break_seconds: 0,
    segments: [
      {
        id: 1,
        kind: "work",
        started_at: new Date(now - 60000).toISOString(),
        ended_at: null,
        seconds: 60,
      },
    ],
    ...patch,
  };
}
describe("focus display follows server bounds", () => {
  it("restores a running interval and clamps a long sleeping tab at its deadline", () => {
    const f = session();
    expect(projectFocus(f, now, 2000)).toEqual({
      active: 62,
      rest: 0,
      remaining: 238,
      boundaryPassed: false,
    });
    expect(projectFocus(f, now, 2 * 3600000)).toEqual({
      active: 300,
      rest: 0,
      remaining: 0,
      boundaryPassed: true,
    });
  });
  it("keeps paused time frozen and never adds breaks to work", () => {
    expect(
      projectFocus(
        session({ state: "paused", phase_deadline: null, phase_remaining_seconds: 240 }),
        now,
        9 * 3600000,
      ).active,
    ).toBe(60);
    const f = session({
      phase: "break",
      active_seconds: 300,
      break_seconds: 20,
      phase_deadline: new Date(now + 40000).toISOString(),
      segments: [
        {
          id: 2,
          kind: "break",
          started_at: new Date(now - 20000).toISOString(),
          ended_at: null,
          seconds: 20,
        },
      ],
    });
    expect(projectFocus(f, now, 120000)).toEqual({
      active: 300,
      rest: 60,
      remaining: 0,
      boundaryPassed: true,
    });
  });
  it("respects both work and wall caps for free sessions", () => {
    const f = session({ mode: "free", phase_deadline: null });
    expect(projectFocus(f, now, 10 * 3600000).active).toBe(14400);
    const long = session({
      mode: "free",
      phase_deadline: null,
      started_at: new Date(now - 7 * 3600000).toISOString(),
    });
    expect(projectFocus(long, now, 10 * 3600000).active).toBe(3660);
  });
  it("ignores negative local elapsed time and formats hours without resetting", () => {
    expect(projectFocus(session(), now, -10000).active).toBe(60);
    expect(duration(14400)).toBe("4:00:00");
    expect(duration(300)).toBe("5:00");
  });
});

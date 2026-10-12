import { describe, expect, it } from "vitest";
import { formatMoment, preferencesSchema } from "../src/lib/settings";
describe("account date preferences", () => {
  it("uses account zones across local midnight and daylight saving boundaries", () => {
    const base = { language: "en" as const, time_format: "24h" as const };
    expect(formatMoment("2026-10-12T22:30:00Z", { ...base, timezone: "UTC" })).toContain(
      "Oct 12, 2026",
    );
    expect(formatMoment("2026-10-12T22:30:00Z", { ...base, timezone: "Asia/Almaty" })).toContain(
      "Oct 13, 2026",
    );
    expect(
      formatMoment("2026-03-08T06:59:00Z", { ...base, timezone: "America/New_York" }),
    ).toContain("01:59");
    expect(
      formatMoment("2026-03-08T07:00:00Z", { ...base, timezone: "America/New_York" }),
    ).toContain("03:00");
  });
  it("rejects invalid timezone and keeps date preferences explicit", () => {
    const base = {
      privacy: {
        profile_visibility: "private",
        share_activity: false,
        share_achievements: false,
        discoverable: false,
      },
      timezone: "UTC",
      language: "ru",
      week_start: 1,
      time_format: "24h",
      work_intervals: [],
      rest_days: [],
    };
    expect(preferencesSchema.safeParse(base).success).toBe(true);
    expect(preferencesSchema.safeParse({ ...base, timezone: "Invalid/Zone" }).success).toBe(false);
  });
});

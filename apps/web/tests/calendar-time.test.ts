import { describe, it, expect } from "vitest";
import { fromWall, toWall } from "../src/lib/calendar-time";
describe("calendar wall time", () => {
  it("uses the account zone rather than the machine zone", () => {
    expect(fromWall("2026-10-12T09:00", "Asia/Almaty")).toBe("2026-10-12T04:00:00Z");
    expect(toWall("2026-10-12T04:00:00Z", "Asia/Almaty")).toBe("2026-10-12T09:00");
  });
  it("rejects a missing time and chooses the earlier fold", () => {
    expect(() => fromWall("2026-03-08T02:30", "America/New_York")).toThrow();
    expect(fromWall("2026-11-01T01:30", "America/New_York")).toBe("2026-11-01T05:30:00Z");
  });
});

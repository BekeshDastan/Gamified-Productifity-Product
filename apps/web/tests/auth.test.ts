import { describe, expect, it } from "vitest";
import { safeRedirect } from "../src/lib/auth/session";
import { passwordSchema } from "../src/components/auth-forms";
describe("authentication input boundaries", () => {
  it("accepts only internal app redirects", () => {
    for (const value of [
      null,
      "",
      "https://foreign.test/app",
      "//foreign.test/app",
      "/\\foreign.test/app",
      "javascript:alert(1)",
      "/app/../../auth/login",
      "/app/%2e%2e/auth",
      "/application",
      "/app\n/guide",
      "/app#secret",
    ])
      expect(safeRedirect(value)).toBe("/app");
    expect(safeRedirect("/app/guide?section=privacy")).toBe("/app/guide?section=privacy");
  });
  it("counts Unicode password characters and preserves spaces", () => {
    expect(passwordSchema.parse("  a valid password  ")).toBe("  a valid password  ");
    expect(passwordSchema.safeParse("🙂".repeat(128)).success).toBe(true);
    expect(passwordSchema.safeParse("🙂".repeat(129)).success).toBe(false);
    expect(passwordSchema.safeParse("short").success).toBe(false);
  });
});

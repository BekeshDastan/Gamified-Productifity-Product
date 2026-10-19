import { test, expect } from "@playwright/test";

test("forced colors retains visible keyboard focus at mobile width", async ({ page }) => {
  await page.emulateMedia({ forcedColors: "active", reducedMotion: "reduce" });
  await page.setViewportSize({ width: 320, height: 740 });
  await page.goto("/");
  await page.keyboard.press("Tab");
  const focus = page.locator(":focus-visible");
  await expect(focus).toHaveCount(1);
  await expect(focus).toHaveCSS("outline-style", "solid");
  await expect(focus).toHaveCSS("outline-width", "3px");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

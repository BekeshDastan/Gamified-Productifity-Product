import { test, expect } from "@playwright/test";

test("forced colors retains visible keyboard focus at mobile width", async ({ page }) => {
  await page.emulateMedia({ forcedColors: "active", reducedMotion: "reduce" });
  await page.setViewportSize({ width: 320, height: 740 });
  await page.route("**/api/v1/auth/session", (route) =>
    route.fulfill({
      status: 401,
      json: { error: { code: "UNAUTHORIZED", message: "Войдите", request_id: "contrast-fixture" } },
    }),
  );
  await page.goto("/");
  // Start from a hydrated control: Next's initial focus restoration can run
  // after navigation completes on a cold CI server.
  const theme = page.getByRole("combobox", { name: "Тема оформления" });
  await expect(theme).toBeEnabled();
  await theme.focus();
  await page.keyboard.press("Tab");
  const focus = page.locator(":focus-visible");
  await expect(focus).toHaveCount(1);
  await expect(page.getByRole("link", { name: "Войти", exact: true })).toBeFocused();
  await expect(focus).toHaveCSS("outline-style", "solid");
  await expect(focus).toHaveCSS("outline-width", "3px");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

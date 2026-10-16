import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

const ready = {
  data: { status: "ok" },
  meta: { request_id: "e2e-health", server_time: "2026-10-12T00:00:00Z" },
};

test("forms wait for client handlers before accepting input on a slow connection", async ({
  page,
}) => {
  let release!: () => void;
  const scripts = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/_next/static/**/*.js", async (route) => {
    await scripts;
    await route.continue();
  });
  await page.goto("/auth/register", { waitUntil: "commit" });
  try {
    await expect(page.getByLabel("Email", { exact: true })).toBeDisabled();
    await expect(page.getByRole("button", { name: "Создать аккаунт", exact: true })).toBeDisabled();
  } finally {
    release();
  }
  await expect(page.getByLabel("Email", { exact: true })).toBeEnabled();
  await page.getByLabel("Email", { exact: true }).fill("slow-connection@example.test");
  await page.waitForLoadState("networkidle");
  await expect(page.getByLabel("Email", { exact: true })).toHaveValue(
    "slow-connection@example.test",
  );
});
// UI response fixtures only. Real PostgreSQL/API behavior has a separate integration suite.
test.beforeEach(async ({ page }) => {
  await page.route("**/health/ready", (route) => route.fulfill({ json: ready }));
  await page.route("**/api/v1/auth/**", (route) =>
    route.request().url().endsWith("/csrf")
      ? route.fulfill({ json: { ...ready, data: { token: "a".repeat(43) } } })
      : route.fulfill({
          status: 401,
          json: { error: { code: "UNAUTHORIZED", message: "Войдите", request_id: "ui-fixture" } },
        }),
  );
});

for (const width of [360, 768, 1280, 1440]) {
  for (const theme of ["dark", "light"]) {
    test(`layout and WCAG scan at ${width}px / ${theme}`, async ({ page }, info) => {
      await page.setViewportSize({ width, height: 900 });
      await page.goto("/");
      await page.getByRole("combobox", { name: "Тема оформления" }).selectOption(theme);
      await expect(page.locator("html")).toHaveClass(new RegExp(theme));
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
      expect(
        (
          await new AxeBuilder({ page })
            .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
            .analyze()
        ).violations,
      ).toEqual([]);
      if (width === 1440 || width === 360)
        await page.screenshot({ path: info.outputPath("landing.png"), fullPage: true });
      await page.getByRole("link", { name: "Открыть пространство" }).click();
      await expect(page).toHaveURL("/app");
      await expect(
        page.getByRole("heading", { name: "Меньше держать в голове. Больше делать." }),
      ).toBeVisible();
      await expect(
        page.getByRole("link", { name: "Задачи", exact: true }).filter({ visible: true }),
      ).toHaveAttribute("href", "/app/quests");
      await expect(
        page.getByRole("link", { name: "Календарь", exact: true }).filter({ visible: true }),
      ).toHaveAttribute("href", "/app/calendar");
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
      expect(
        (
          await new AxeBuilder({ page })
            .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
            .analyze()
        ).violations,
      ).toEqual([]);
      if (width === 1440 || width === 360)
        await page.screenshot({ path: info.outputPath("app.png"), fullPage: true });
    });
  }
}

test("mobile menu traps keyboard focus and returns it to either opener", async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 800 });
  await page.goto("/app");
  for (const name of ["Открыть меню", "Ещё — открыть меню"]) {
    const trigger = page.getByRole("button", { name, exact: true });
    await trigger.focus();
    await page.keyboard.press("Enter");
    const dialog = page.getByRole("dialog", { name: "Разделы" });
    await expect(dialog).toBeVisible();
    for (let i = 0; i < 7; i++) {
      await page.keyboard.press("Tab");
      expect(await dialog.evaluate((el) => el.contains(document.activeElement))).toBe(true);
    }
    expect(
      (
        await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
          .analyze()
      ).violations,
    ).toEqual([]);
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(trigger).toBeFocused();
  }
  await page.getByRole("button", { name: "Ещё — открыть меню" }).click();
  await page.getByRole("dialog").getByRole("link", { name: "Настройки", exact: true }).focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL("/app/settings");
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("keyboard navigation, persisted theme and system theme", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await page.goto("/app");
  await expect(page.locator("html")).toHaveClass(/dark/);
  await page.keyboard.press("Tab");
  await expect(page.getByRole("link", { name: "К содержимому" })).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.locator("#main-content")).toBeFocused();
  await page.getByRole("link", { name: "Настройки", exact: true }).focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL("/app/settings");
  await page.getByRole("link", { name: /Оформление Тема/ }).click();
  await expect(page).toHaveURL("/app/settings/appearance");
  await page.getByRole("combobox", { name: "Тема оформления" }).last().selectOption("light");
  await page.reload();
  await expect(page.locator("html")).toHaveClass(/light/);
  await page.getByRole("combobox", { name: "Тема оформления" }).last().selectOption("system");
  await expect(page.locator("html")).toHaveClass(/dark/);
  await page.emulateMedia({ colorScheme: "light" });
  await expect(page.locator("html")).toHaveClass(/light/);
  expect(
    (
      await new AxeBuilder({ page })
        .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
        .analyze()
    ).violations,
  ).toEqual([]);
});

test("server failure can be retried without false success", async ({ page }) => {
  await page.unroute("**/health/ready");
  let attempts = 0;
  await page.route("**/health/ready", (route) =>
    ++attempts === 1
      ? route.fulfill({
          status: 429,
          headers: { "Retry-After": "10" },
          json: {
            error: {
              code: "RATE_LIMITED",
              message: "private exception must not render",
              request_id: "retry-test",
            },
          },
        })
      : route.fulfill({ json: ready }),
  );
  await page.goto("/app/guide");
  await expect(page.getByRole("main").getByRole("alert")).toContainText("Слишком много попыток");
  await expect(page.getByText("Соединение с сервером установлено")).toHaveCount(0);
  await expect(page.getByText(/private exception/)).toHaveCount(0);
  await expect(page.getByText("Код обращения: retry-test")).toBeVisible();
  await page.getByRole("button", { name: "Повторить" }).click();
  await expect(page.getByRole("status")).toContainText("Соединение с сервером установлено");
  expect(attempts).toBe(2);
});

test("reduced motion disables skeleton animation", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.unroute("**/health/ready");
  let release: (() => void) | undefined;
  await page.route("**/health/ready", async (route) => {
    await new Promise<void>((resolve) => {
      release = resolve;
    });
    await route.fulfill({ json: ready });
  });
  await page.goto("/app/guide");
  await expect(page.getByRole("status")).toContainText("Проверяем соединение");
  expect(
    await page
      .locator(".skeleton")
      .first()
      .evaluate((el) => parseFloat(getComputedStyle(el).animationDuration)),
  ).toBeLessThan(0.01);
  release?.();
  await expect(page.getByRole("status")).toContainText("установлено");
});

test("unknown routes return 404; auth routes disclose current availability", async ({
  page,
  request,
}) => {
  const response = await page.goto("/app/does-not-exist");
  expect(response?.status()).toBe(404);
  await expect(page.getByRole("heading", { name: "Страница не найдена" })).toBeVisible();
  await page.getByRole("link", { name: "Вернуться к своему дню" }).click();
  await expect(page).toHaveURL("/app");
  expect((await page.goto("/does-not-exist"))?.status()).toBe(404);
  await page.getByRole("link", { name: "На главную" }).click();
  await expect(page).toHaveURL("/");
  await page.getByRole("link", { name: "Войти" }).click();
  await expect(page.getByRole("heading", { name: "С возвращением" })).toBeVisible();
  expect(
    (
      await new AxeBuilder({ page })
        .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
        .analyze()
    ).violations,
  ).toEqual([]);
  expect((await request.get("/ascend-path.svg")).ok()).toBe(true);
  expect(await (await request.get("/licenses/lucide.txt")).text()).toContain("ISC License");
});

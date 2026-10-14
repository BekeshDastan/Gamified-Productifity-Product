import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

test("visual refresh: clock controls, silent motion and responsive sections", async ({
  page,
}, info) => {
  test.setTimeout(240000);
  const email = `visual-${crypto.randomUUID()}@example.test`;
  const password = "visual refresh browser test phrase";
  await page.goto("/auth/register");
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill(password);
  await page.getByLabel("Повторите пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Создать аккаунт", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Аккаунт создан" })).toBeVisible();
  await page.goto("/auth/login");
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page).toHaveURL("/app");
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.getByLabel("Тема оформления").selectOption("light");
  for (const title of ["Прочитать главу книги", "Подготовить план проекта", "Выйти на прогулку"]) {
    await page.getByLabel("Быстро добавить задачу").fill(title);
    await page.getByRole("button", { name: "Добавить на сегодня", exact: true }).click();
    await expect(
      page.getByRole("status").filter({ hasText: "Добавлено на сегодня: " + title }),
    ).toBeVisible();
  }
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: info.outputPath("today-desktop.png"), fullPage: true });
  await page.goto("/app/focus");
  await expect(page.getByLabel("Таймер фокуса", { exact: true })).toHaveText("25:00");
  await page.getByRole("button", { name: "5 минут", exact: true }).click();
  await expect(page.getByLabel("Таймер фокуса", { exact: true })).toHaveText("05:00");
  await page.getByRole("button", { name: "Свободный", exact: true }).click();
  await expect(page.getByLabel("Таймер фокуса", { exact: true })).toHaveText("00:00");
  await page.getByRole("button", { name: "Pomodoro", exact: true }).click();
  await page.getByLabel("Рабочий интервал, минут").fill("40");
  await expect(page.getByLabel("Таймер фокуса", { exact: true })).toHaveText("40:00");
  await page.getByLabel("Рабочий интервал, минут").fill("25");
  await page.setViewportSize({ width: 1280, height: 720 });
  await expect
    .poll(() =>
      page.getByRole("button", { name: "Начать фокус-сессию", exact: true }).evaluate((button) => {
        const box = button.getBoundingClientRect();
        return box.top >= 0 && box.bottom <= innerHeight;
      }),
    )
    .toBe(true);
  await page.screenshot({ path: info.outputPath("focus-laptop.png") });
  await page.setViewportSize({ width: 1440, height: 1000 });
  const video = page.locator("video");
  await expect
    .poll(() => video.evaluate((v: HTMLVideoElement) => v.readyState))
    .toBeGreaterThanOrEqual(2);
  expect(await video.evaluate((v: HTMLVideoElement) => v.muted)).toBe(true);
  // Explicit playback works even where browser autoplay is blocked.
  if (await page.getByRole("button", { name: "Включить анимацию", exact: true }).isVisible())
    await page.getByRole("button", { name: "Включить анимацию", exact: true }).click();
  await expect
    .poll(() => video.evaluate((v: HTMLVideoElement) => v.currentTime))
    .toBeGreaterThan(0);
  await page.getByRole("button", { name: "Остановить анимацию", exact: true }).click();
  expect(await video.evaluate((v: HTMLVideoElement) => v.paused)).toBe(true);
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.screenshot({ path: info.outputPath("focus-desktop.png"), fullPage: true });
  await page.getByRole("button", { name: "Начать фокус-сессию", exact: true }).click();
  await expect(page.getByText("Таймер идёт", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Пауза", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: /^Пауза$/ })).toBeVisible();
  const time = await page.getByLabel("Таймер фокуса", { exact: true }).innerText();
  await page.reload();
  await expect(page.getByLabel("Таймер фокуса", { exact: true })).toHaveText(time);
  expect(await page.locator("video").evaluate((v: HTMLVideoElement) => v.paused)).toBe(true);
  await page.getByRole("button", { name: "Отменить сессию", exact: true }).click();
  await page.getByRole("button", { name: "Подтвердить отмену сессии", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Начать фокус-сессию", exact: true }),
  ).toBeVisible();
  for (const theme of ["light", "dark"]) {
    await page.getByLabel("Тема оформления").selectOption(theme);
    await expect(page.locator("html")).toHaveClass(new RegExp(theme));
    for (const route of ["focus", "calendar", "", "settings"]) {
      await page.goto("/app" + (route ? "/" + route : ""));
      await page.waitForLoadState("networkidle");
      if (theme === "light" && route === "calendar")
        await page.screenshot({ path: info.outputPath("calendar-desktop.png"), fullPage: true });
      await page.setViewportSize({ width: 390, height: 844 });
      // Resize is asynchronous in Chromium; check the settled layout, not its first frame.
      await expect
        .poll(() => page.evaluate(() => document.documentElement.scrollWidth - innerWidth))
        .toBeLessThanOrEqual(0);
      const geometry = await page.evaluate(() => ({
        viewport: innerWidth,
        width: document.documentElement.scrollWidth,
        overflowing: [...document.querySelectorAll("body *")]
          .map((el) => ({
            tag: el.tagName,
            className: typeof el.className === "string" ? el.className : "svg",
            right: el.getBoundingClientRect().right,
          }))
          .filter((el) => el.right > innerWidth)
          .slice(0, 20),
      }));
      expect(geometry.width, JSON.stringify(geometry)).toBeLessThanOrEqual(geometry.viewport);
      expect(
        (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath(`${route || "today"}-mobile-${theme}.png`),
        fullPage: true,
      });
      await page.setViewportSize({ width: 1440, height: 1000 });
    }
  }
  await page.setViewportSize({ width: 360, height: 800 });
  for (const route of [
    "quests",
    "goals",
    "habits",
    "journal",
    "analytics",
    "planning",
    "progress",
    "character",
    "achievements",
    "rewards",
    "challenges",
    "bosses",
    "seasons",
    "navigator",
    "notifications",
    "settings/appearance",
    "guide",
  ]) {
    await page.goto("/app/" + route);
    await page.waitForLoadState("networkidle");
    await expect(page.locator("main h1")).toBeVisible();
    await expect
      .poll(() => page.evaluate(() => document.documentElement.scrollWidth - innerWidth), {
        message: route,
      })
      .toBeLessThanOrEqual(0);
    await expect(page.locator("body")).not.toContainText("Application error");
  }
});

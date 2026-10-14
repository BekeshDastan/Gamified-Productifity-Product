import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
async function account(page: Page) {
  await page.clock.setFixedTime("2026-10-12T12:00:00Z");
  const email = "analytics-" + crypto.randomUUID() + "@example.test",
    password = "analytics browser password phrase";
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
}
test("analytics charts tables heatmap and exact source links agree in both themes", async ({
  page,
}, info) => {
  test.setTimeout(120000);
  await account(page);
  await page.getByLabel("Быстро добавить задачу").fill("Источник графика");
  await page.getByRole("button", { name: "Добавить на сегодня", exact: true }).click();
  await page.getByRole("link", { name: "Источник графика", exact: true }).click();
  await page.getByLabel("Дедлайн с часовым поясом").fill("2026-10-12T10:00:00Z");
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Сохранено." })).toBeVisible();
  const taskURL = page.url();
  const panel = page
    .locator("section")
    .filter({ has: page.getByRole("heading", { name: "Следующий шаг", exact: true }) });
  await panel.getByRole("button", { name: "Завершить", exact: true }).click();
  await panel.getByRole("button", { name: "Подтвердить действие", exact: true }).click();
  await expect(page.getByLabel("Статус", { exact: true })).toHaveValue("completed");
  await page.goto("/app/analytics");
  await expect(page.getByRole("heading", { name: "Динамика по дням", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "7 дней", exact: true }).click();
  await expect(page.getByLabel("Начало аналитики", { exact: true })).toHaveValue("2026-10-06");
  await page.locator('.recharts-bar-rectangle path[fill="var(--warning)"]').last().click();
  await expect(page.getByLabel("Фильтр источников", { exact: true })).toHaveValue("late");
  await page.getByRole("button", { name: "Источники 2026-10-12", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Источники за 2026-10-12", exact: true }),
  ).toBeVisible();
  const sources = page.getByRole("region", { name: "Исходные записи дня", exact: true });
  await expect(
    sources.getByRole("link", { name: "Источник графика", exact: true }),
  ).toHaveAttribute("href", new URL(taskURL).pathname);
  await expect(sources.getByText("Завершение задачи · Поздно", { exact: true })).toBeVisible();
  const response = await page.request.get("/api/v1/analytics?from=2026-10-06&to=2026-10-13");
  expect(response.status()).toBe(200);
  const view = (await response.json()).data,
    day = view.days.find((d: { date: string }) => d.date === "2026-10-12");
  expect(day.completed).toBe(1);
  expect(day.late).toBe(1);
  expect(day.on_time).toBe(0);
  expect(day.scores.daily).toBeNull();
  const table = page.getByRole("region", { name: "Показатели по дням", exact: true });
  await expect(
    table
      .getByRole("row")
      .filter({ has: page.getByRole("rowheader", { name: "2026-10-12", exact: true }) }),
  ).toContainText("1");
  await page.getByLabel("Показатель графика", { exact: true }).selectOption("scores");
  await expect(
    table
      .getByRole("row")
      .filter({ has: page.getByRole("rowheader", { name: "2026-10-12", exact: true }) }),
  ).toContainText("Нет плана");
  await page.getByRole("button", { name: "Показать карту года", exact: true }).click();
  const heat = page.getByRole("region", { name: "Карта дней за 2026 год", exact: true });
  await expect(heat.getByRole("button")).toHaveCount(365);
  await heat.getByRole("button", { name: /^2026-10-12:/ }).click();
  await expect(page.getByLabel("Начало аналитики", { exact: true })).toHaveValue("2026-10-12");
  await expect(page.getByLabel("Конец аналитики включительно", { exact: true })).toHaveValue(
    "2026-10-12",
  );
  await expect(
    page.getByRole("button", { name: "Источники 2026-10-12", exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.emulateMedia({ reducedMotion: "reduce", colorScheme: "dark" });
  await expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
    .toBe(true);
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
  await page.screenshot({ path: info.outputPath("analytics-mobile-dark.png"), fullPage: true });
  await page
    .getByRole("heading", { name: "Динамика по дням", exact: true })
    .scrollIntoViewIfNeeded();
  await page.screenshot({ path: info.outputPath("analytics-chart-mobile-dark.png") });
  await page.emulateMedia({ colorScheme: "light" });
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
  await page.screenshot({ path: info.outputPath("analytics-mobile-light.png"), fullPage: true });
  await page.getByLabel("Фильтр источников", { exact: true }).selectOption("task");
  await expect(sources.getByRole("link", { name: "Источник графика", exact: true })).toHaveCount(1);
  await sources.getByRole("link", { name: "Источник графика", exact: true }).click();
  await expect(page).toHaveURL(taskURL);
});

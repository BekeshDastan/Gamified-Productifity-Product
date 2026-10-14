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
test("analytics reports preview disclose incomplete observations and save immutable snapshots", async ({
  page,
}) => {
  test.setTimeout(120000);
  await account(page);
  await page.goto("/app/analytics");
  await page.getByRole("button", { name: "Показать предпросмотр отчёта", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Предпросмотр отчёта", exact: true }),
  ).toBeVisible();
  const compare = page.getByRole("region", {
    name: "Сравнение равных периодов отчёта",
    exact: true,
  });
  await expect(compare.getByText("Нет полных наблюдений", { exact: true })).toHaveCount(6);
  await compare
    .getByRole("button", { name: "Источники: Завершённые задачи, текущий период", exact: true })
    .click();
  await expect(page.getByRole("region", { name: "Источники числа в снимке" })).toContainText(
    "В снимке нет записей для этого числа.",
  );
  const save = page.getByRole("button", { name: "Сохранить снимок отчёта", exact: true });
  await expect(save).toBeDisabled();
  await page
    .getByLabel("Подтверждаю сохранение неизменяемого снимка отчёта", { exact: true })
    .check();
  await save.click();
  await expect(page.getByRole("heading", { name: "Сохранённый отчёт", exact: true })).toBeVisible();
  const listResponse = await page.request.get("/api/v1/analytics/reports");
  expect(listResponse.status()).toBe(200);
  const list = (await listResponse.json()).data;
  expect(list.items).toHaveLength(1);
  expect(list.items[0].automatic).toBe(false);
  expect(list.items[0].snapshot).toBeUndefined();
  const id = list.items[0].id,
    original = (await (await page.request.get(`/api/v1/analytics/reports/${id}`)).json()).data
      .snapshot;
  expect(original.current_observed).toBe(false);
  expect(original.previous_observed).toBe(false);
  expect(original.comparisons.every((c: { percent: number | null }) => c.percent === null)).toBe(
    true,
  );
  await page.getByRole("link", { name: "Ссылка на этот отчёт", exact: true }).click();
  await expect(page).toHaveURL(new RegExp("report_id=" + id));
  await page.reload();
  await expect(page.getByRole("heading", { name: "Сохранённый отчёт", exact: true })).toBeVisible();
  expect(
    (await (await page.request.get(`/api/v1/analytics/reports/${id}`)).json()).data.snapshot,
  ).toEqual(original);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
    .toBe(true);
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});
test("analytics personal targets require consent preserve units and protect local edits", async ({
  page,
}, info) => {
  test.setTimeout(120000);
  await account(page);
  await page.goto("/app/settings/areas");
  for (const name of ["Дела", "Фокус", "Привычки"]) {
    await page.getByLabel("Название направления", { exact: true }).fill(name);
    await page.getByRole("button", { name: "Создать направление", exact: true }).click();
    await expect(page.getByLabel("Название направления", { exact: true })).toHaveValue("");
  }
  await page.goto("/app/analytics");
  await page.getByRole("button", { name: "7 дней", exact: true }).click();
  const table = page.getByRole("region", {
    name: "Личные ориентиры и фактические значения",
    exact: true,
  });
  await expect(table.getByText("Нет оценки", { exact: true })).toHaveCount(3);
  for (const [name, metric, target] of [
    ["Дела", "tasks", "10"],
    ["Фокус", "focus_minutes", "60"],
    ["Привычки", "habit_norms", "3"],
  ] as const) {
    await page.getByText(`${name}: изменить ориентир и открыть записи`, { exact: true }).click();
    await page.getByLabel(`Показатель: ${name}`, { exact: true }).selectOption(metric);
    await page.getByLabel(`Ориентир: ${name}`, { exact: true }).fill(target);
    const save = page.getByRole("button", { name: `Сохранить ориентир «${name}»`, exact: true });
    await expect(save).toBeDisabled();
    await page
      .getByLabel(`Подтверждаю ориентир для направления «${name}» на выбранный период`, {
        exact: true,
      })
      .check();
    await save.click();
    await expect(
      page.getByLabel(`Подтверждаю ориентир для направления «${name}» на выбранный период`, {
        exact: true,
      }),
    ).not.toBeChecked();
  }
  await expect(
    page.getByRole("group", { name: "Диаграмма выполнения личных ориентиров", exact: true }),
  ).toBeVisible();
  await expect(table.getByText("10 задач", { exact: true })).toBeVisible();
  await expect(table.getByText("60 минут", { exact: true })).toBeVisible();
  await expect(table.getByText("3 периодов", { exact: true })).toBeVisible();
  await page.getByLabel("Ориентир: Дела", { exact: true }).fill("20");
  page.once("dialog", (d) => d.dismiss());
  await page.getByRole("button", { name: "30 дней", exact: true }).click();
  await expect(page.getByLabel("Начало аналитики", { exact: true })).toHaveValue("2026-10-06");
  await expect(page.getByLabel("Ориентир: Дела", { exact: true })).toHaveValue("20");
  page.once("dialog", (d) => d.accept());
  await page
    .locator("details")
    .filter({ has: page.getByLabel("Ориентир: Дела", { exact: true }) })
    .getByRole("button", { name: "Обновить исходный ориентир", exact: true })
    .click();
  await expect(page.getByLabel("Ориентир: Дела", { exact: true })).toHaveValue("10");
  await page.getByRole("button", { name: "30 дней", exact: true }).click();
  await expect(table.getByText("Нет оценки", { exact: true })).toHaveCount(3);
  await page.getByRole("button", { name: "7 дней", exact: true }).click();
  await expect(
    page.getByRole("group", { name: "Диаграмма выполнения личных ориентиров", exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.emulateMedia({ reducedMotion: "reduce", colorScheme: "dark" });
  await expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
    .toBe(true);
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
  await page.screenshot({ path: info.outputPath("personal-targets-mobile.png"), fullPage: true });
  await page
    .getByRole("heading", { name: "Баланс по собственным ориентирам", exact: true })
    .scrollIntoViewIfNeeded();
  await page.screenshot({ path: info.outputPath("personal-target-radar-mobile.png") });
  await page
    .getByRole("group", { name: "Диаграмма выполнения личных ориентиров", exact: true })
    .screenshot({ path: info.outputPath("personal-target-radar-chart.png") });
});
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

import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
async function account(page: Page) {
  await page.clock.setFixedTime("2026-10-12T12:00:00Z");
  const email = "planning-" + crypto.randomUUID() + "@example.test",
    password = "planning browser password phrase";
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
test("planning morning flow freezes commitments and updates dashboard calendar and history", async ({
  page,
}, info) => {
  test.setTimeout(120000);
  await account(page);
  await page.getByLabel("Быстро добавить задачу").fill("Исходная задача");
  await page.getByRole("button", { name: "Добавить на сегодня", exact: true }).click();
  await expect(page.getByRole("link", { name: "Исходная задача", exact: true })).toBeVisible();
  await page.getByRole("link", { name: "Исходная задача", exact: true }).click();
  await expect(page).toHaveURL(/\/app\/quests\/[a-f0-9-]+$/);
  const taskURL = page.url();
  await page.getByLabel("Оценка, минут").fill("60");
  await page.getByLabel("Дедлайн с часовым поясом").fill("2026-10-14T18:00:00Z");
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Сохранено." })).toBeVisible();
  await page.goto("/app/planning?date=2026-10-12");
  await page.getByLabel("Доступно минут", { exact: true }).fill("120");
  await page
    .getByLabel("Задача в план", { exact: true })
    .selectOption({ label: "Исходная задача" });
  await page.getByRole("button", { name: "Добавить выбранную задачу" }).click();
  await page.getByLabel("Главная задача", { exact: true }).check();
  await page.getByRole("button", { name: "Сохранить черновик", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Блоки работы и перенос" })).toBeVisible();
  for (const [start, end] of [
    ["09:00", "09:30"],
    ["10:00", "10:30"],
  ] as const) {
    await page.getByLabel("Начало блока").fill(start);
    await page.getByLabel("Конец блока").fill(end);
    await page.getByLabel("Причина переноса или блока").fill("Выделяю время");
    await page.getByRole("button", { name: "Применить к задаче", exact: true }).click();
    await expect(page.getByLabel("Причина переноса или блока")).toHaveValue("");
  }
  await page.getByLabel("Я просмотрел задачи, ёмкость и пересечения").check();
  await page.getByRole("button", { name: "Подтвердить план", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Исходный подтверждённый план" })).toBeVisible();
  await page.getByRole("button", { name: "Изменить план", exact: true }).click();
  await page.getByRole("button", { name: "Убрать из плана", exact: true }).click();
  await page.getByLabel("Причина изменения плана").fill("Меняю текущий список");
  await page.getByRole("button", { name: "Сохранить дополнение", exact: true }).click();
  await page.reload();
  await expect(page.getByText("Выполнение исходного плана: 0 из 100 · 0 из 1 задач")).toBeVisible();
  await page.getByText("Посмотреть исходный снимок", { exact: true }).click();
  await expect(page.getByText("Блоков на момент подтверждения: 2")).toBeVisible();
  await page.goto(taskURL);
  await expect(page.getByLabel("Дедлайн с часовым поясом")).toHaveValue("2026-10-14T18:00:00Z");
  await page.goto("/app");
  await page.getByRole("button", { name: "Завершить Исходная задача", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "Выполнено: Исходная задача" }),
  ).toBeVisible();
  await page.goto("/app/planning?date=2026-10-12");
  await expect(
    page.getByText("Выполнение исходного плана: 100 из 100 · 1 из 1 задач"),
  ).toBeVisible();
  await page.setViewportSize({ width: 360, height: 800 });
  expect(
    (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
  ).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({ path: info.outputPath("planning-mobile.png"), fullPage: true });
  await page.emulateMedia({ colorScheme: "dark", reducedMotion: "reduce" });
  expect(
    (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
  ).toEqual([]);
  await page.screenshot({ path: info.outputPath("planning-mobile-dark.png"), fullPage: true });
  await page.getByLabel("Вид плана").selectOption("week");
  await expect(page.getByRole("heading", { name: "Неделя с 2026-10-12" })).toBeVisible();
  await page.getByLabel("Дата плана").fill("2026-10-11");
  await page.getByLabel("День отдыха без оценки").check();
  await page.getByRole("button", { name: "Сохранить черновик", exact: true }).click();
  await page.getByLabel("Я просмотрел задачи, ёмкость и пересечения").check();
  await page.getByRole("button", { name: "Подтвердить план", exact: true }).click();
  await expect(page.getByText("Отдых без оценки", { exact: true })).toBeVisible();
  await page.goto("/app/planning?date=2026-10-11");
  await expect(page.getByText("Отдых без оценки", { exact: true })).toBeVisible();
});
test("planning routine preview applies once and dashboard widget composition persists", async ({
  page,
}) => {
  test.setTimeout(120000);
  await account(page);
  await page.goto("/app/planning?date=2026-10-13");
  await page.getByLabel("Доступно минут", { exact: true }).fill("180");
  await page.getByRole("button", { name: "Сохранить черновик", exact: true }).click();
  await page.getByRole("button", { name: "Предпросмотр шаблона", exact: true }).click();
  await expect(page.getByLabel("Причина применения шаблона")).toBeVisible();
  await page.getByLabel("Причина применения шаблона").fill("Учебный день");
  await page.getByRole("button", { name: "Применить шаблон", exact: true }).click();
  await expect(page.getByText(/Уже применено/)).toBeVisible();
  await expect(page.getByRole("button", { name: "Применить шаблон", exact: true })).toBeDisabled();
  await page.reload();
  await page.getByRole("button", { name: "Предпросмотр шаблона", exact: true }).click();
  await expect(page.getByText(/Уже применено/)).toBeVisible();
  await expect(page.getByRole("button", { name: "Применить шаблон", exact: true })).toBeDisabled();
  await page.goto("/app/settings/appearance");
  await page.getByLabel("Показывать Фокус", { exact: true }).uncheck();
  await page.getByRole("button", { name: "Сохранить оформление", exact: true }).click();
  await expect(page.getByText("Оформление сохранено.", { exact: true })).toBeVisible();
  await page.goto("/app");
  await expect(page.getByRole("heading", { name: "Фокус", exact: true })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("heading", { name: "Фокус", exact: true })).toHaveCount(0);
});

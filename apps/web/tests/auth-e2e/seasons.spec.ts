import { revealControl } from "./disclosure";
import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
async function account(page: Page) {
  await page.clock.setFixedTime("2026-10-12T12:00:00Z");
  const email = "seasons-" + crypto.randomUUID() + "@example.test",
    password = "seasons browser password phrase";
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
async function data(page: Page, path: string) {
  const r = await page.request.get("/api/v1" + path);
  expect(r.status()).toBe(200);
  return (await r.json()).data;
}
test("seasons preserve closure history and low energy commitments with accessible controls", async ({
  page,
}, info) => {
  test.setTimeout(120000);
  await account(page);
  await page.getByLabel("Быстро добавить задачу").fill("Маленькая задача сезона");
  await page.getByRole("button", { name: "Добавить на сегодня", exact: true }).click();
  await page.getByRole("link", { name: "Маленькая задача сезона", exact: true }).click();
  await revealControl(page.getByLabel("Оценка, минут"));
  await page.getByLabel("Оценка, минут").fill("10");
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Сохранено." })).toBeVisible();
  await page.goto("/app/planning?date=2026-10-12");
  await page.getByLabel("Доступно минут", { exact: true }).fill("60");
  await page
    .getByLabel("Задача в план", { exact: true })
    .selectOption({ label: "Маленькая задача сезона" });
  let releaseTask!: () => void;
  const loadingTask = new Promise<void>((resolve) => {
    releaseTask = resolve;
  });
  await page.route(/\/api\/v1\/quests\/[0-9a-f-]+$/, async (route) => {
    if (route.request().method() === "GET") await loadingTask;
    await route.continue();
  });
  await page.getByRole("button", { name: "Добавить выбранную задачу" }).click();
  await expect(
    page.getByRole("button", { name: "Сохранить черновик", exact: true }),
  ).toBeDisabled();
  releaseTask();
  await page.getByRole("button", { name: "Сохранить черновик", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Блоки работы и перенос" })).toBeVisible();
  await page.getByLabel("Я просмотрел задачи, ёмкость и пересечения").check();
  await page.getByRole("button", { name: "Подтвердить план", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Исходный подтверждённый план" })).toBeVisible();
  await page.goto("/app/seasons");
  await page.getByRole("button", { name: "Создать сезон", exact: true }).click();
  await page.getByLabel("Название сезона", { exact: true }).fill("Мой первый сезон");
  await page.getByLabel("Тема", { exact: true }).fill("Посильный ритм");
  await page.getByLabel("Желаемый результат", { exact: true }).fill("Посильное продолжение");
  await page.getByLabel("Подтверждаю даты и результат", { exact: true }).check();
  await page.getByRole("button", { name: "Сохранить сезон", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Мой первый сезон", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Подтвердить активацию сезона", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Подтвердить активацию сезона", exact: true }),
  ).toHaveCount(0);
  await page.getByLabel("Минут на гибкие задачи", { exact: true }).fill("0");
  await page.getByRole("button", { name: "Показать облегчённый план", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Предпросмотр на 2026-10-12", exact: true }),
  ).toBeVisible();
  await page.getByLabel("Причина облегчения", { exact: true }).fill("Сегодня восстанавливаюсь");
  await page
    .getByLabel("Подтверждаю выбранные задачи и сохранение фиксированных обязательств", {
      exact: true,
    })
    .check();
  await page.getByRole("button", { name: "Применить облегчённый план", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "Изменение сохранено как дополнение" }),
  ).toBeVisible();
  const plan = await data(page, "/plans/day/2026-10-12");
  expect(plan.quest_ids).toHaveLength(0);
  expect(plan.snapshot.items).toHaveLength(1);
  expect(plan.progress.committed).toBe(1);
  await page
    .getByLabel("Причина завершения", { exact: true })
    .fill("Завершаю раньше по своему выбору");
  const close = page.getByRole("button", { name: "Зафиксировать итог и завершить", exact: true });
  await expect(close).toBeDisabled();
  await page.getByLabel("Понимаю, что итог станет неизменяемым", { exact: true }).check();
  await close.click();
  await expect(page.getByRole("heading", { name: "Сохранённый итог", exact: true })).toBeVisible();
  const season = (await data(page, "/seasons")).items[0],
    snapshot = await data(page, `/seasons/${season.id}/summary`);
  expect(snapshot.early).toBe(true);
  await page.getByRole("button", { name: "Подтвердить перенос в архив", exact: true }).click();
  await expect(page.getByRole("button", { name: "Вернуть из архива", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Архив сезонов", exact: true }).click();
  await page.getByRole("button", { name: "Мой первый сезон", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Сохранённый итог", exact: true })).toBeVisible();
  expect(await data(page, `/seasons/${season.id}/summary`)).toEqual(snapshot);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.emulateMedia({ reducedMotion: "reduce", colorScheme: "dark" });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
  await page.screenshot({ path: info.outputPath("seasons-mobile.png"), fullPage: true });
  await page.emulateMedia({ colorScheme: "light" });
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

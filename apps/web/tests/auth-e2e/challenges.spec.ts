import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
async function account(page: Page) {
  await page.clock.setFixedTime("2026-10-12T12:00:00Z");
  const email = "challenge-" + crypto.randomUUID() + "@example.test",
    password = "challenge browser password phrase";
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
async function create(page: Page, name: string, template?: string, target?: number) {
  await page.getByRole("button", { name: "Создать испытание", exact: true }).click();
  if (template) await page.getByLabel("Шаблон испытания", { exact: true }).selectOption(template);
  await page.getByLabel("Название испытания", { exact: true }).fill(name);
  if (target) await page.getByLabel("Целевой результат", { exact: true }).fill(String(target));
  await page.getByLabel("Начало испытания", { exact: true }).fill("2026-10-12");
  await page.getByRole("button", { name: "Создать с этими правилами", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Зафиксированные правила", exact: true }),
  ).toBeVisible();
  return (await data(page, "/challenges/runs")).items[0].id as string;
}
async function join(page: Page) {
  const button = page.getByRole("button", { name: "Вступить в испытание", exact: true });
  await expect(button).toBeDisabled();
  await page.getByLabel("Я прочитал и принимаю эти правила", { exact: true }).check();
  await button.click();
  await expect(page.getByRole("progressbar", { name: "Прогресс испытания" })).toBeVisible();
}
test("challenges accept suggestions once and correct automatic progress", async ({ page }) => {
  test.setTimeout(120000);
  await account(page);
  await page.goto("/app/challenges");
  await expect(page.getByRole("heading", { name: "Квесты на день и неделю" })).toBeVisible();
  expect((await data(page, "/quests")).total).toBe(0);
  await page
    .getByRole("button", { name: /^Принять «/ })
    .first()
    .click();
  await expect(page.getByRole("link", { name: "Открыть принятую задачу" })).toHaveCount(1);
  expect((await data(page, "/quests")).total).toBe(1);
  const task = (await data(page, "/quests")).items[0];
  expect(task.planned_date).toBeNull();
  expect(task.status).toBe("backlog");
  const id = await create(page, "Один результат");
  await join(page);
  await page.goto("/app/quests/" + task.id);
  const panel = page
    .locator("section")
    .filter({ has: page.getByRole("heading", { name: "Следующий шаг", exact: true }) });
  await panel.getByRole("button", { name: "Завершить", exact: true }).click();
  await panel.getByRole("button", { name: "Подтвердить действие", exact: true }).click();
  await expect(page.getByLabel("Статус", { exact: true })).toHaveValue("completed");
  await page.goto("/app/challenges?run_id=" + id);
  await expect(page.getByText(/1 \/ 3 задач/)).toBeVisible();
  await page.getByRole("link", { name: task.title, exact: true }).click();
  await panel.getByRole("button", { name: "Открыть снова", exact: true }).click();
  await panel.getByLabel("Причина действия").fill("Исправление результата");
  await panel.getByRole("button", { name: "Подтвердить действие", exact: true }).click();
  await expect(page.getByLabel("Статус", { exact: true })).toHaveValue("todo");
  await page.goto("/app/challenges?run_id=" + id);
  await expect(page.getByText(/0 \/ 3 задач/)).toBeVisible();
  expect((await data(page, "/challenges/runs/" + id + "/sources")).total).toBe(0);
});
test("challenges label manual reports preserve corrections and remain accessible", async ({
  page,
}, info) => {
  test.setTimeout(120000);
  await account(page);
  await page.goto("/app/challenges");
  const id = await create(page, "Моё чтение", "reading-pages", 10);
  await join(page);
  await page.getByLabel("Количество самоотчёта", { exact: true }).fill("10");
  await page
    .getByLabel("Заметка к самоотчёту", { exact: true })
    .fill("CHALLENGE_PRIVATE_SENTINEL_C128");
  await page.getByRole("button", { name: "Сохранить самоотчёт", exact: true }).click();
  await expect(page.getByText(/10 \/ 10 страниц/)).toBeVisible();
  await expect(page.locator('form[aria-busy="true"]')).toHaveCount(0);
  await page.getByRole("button", { name: "Сохранить самоотчёт", exact: true }).click();
  await expect
    .poll(async () => (await data(page, "/challenges/runs/" + id + "/check-ins")).items.length)
    .toBe(2);
  const rows = page.locator('details[id^="checkin-"]');
  await expect(rows).toHaveCount(2);
  const row = page.locator("#" + (await rows.first().getAttribute("id")));
  await row.locator("summary").click();
  await row.getByLabel(/Исправленное количество:/).fill("0");
  await row.getByRole("button", { name: "Сохранить исправление", exact: true }).click();
  await expect(page.getByText(/10 \/ 10 страниц/)).toBeVisible();
  await expect(page.locator('form[aria-busy="true"]')).toHaveCount(0);
  await page
    .getByLabel("Подтверждаю закрытие испытания и сохранение итога", { exact: true })
    .check();
  await page
    .getByRole("button", { name: "Закрыть испытание и сохранить итог", exact: true })
    .click();
  await expect(page.getByText(/Итог: Цель достигнута/)).toBeVisible();
  await expect(
    page.getByRole("status").filter({ hasText: "Награда этого испытания: 25 XP" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Показать источники первого итога", exact: true }).click();
  await expect(page.getByText(/Исходные записи на момент первого закрытия/)).toBeVisible();
  expect((await data(page, "/challenges/runs/" + id + "/final-sources")).total).toBe(1);
  await page.setViewportSize({ width: 360, height: 800 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  for (const theme of ["light", "dark"]) {
    await page.getByLabel("Тема оформления", { exact: true }).selectOption(theme);
    await expect(page.locator("html")).toHaveClass(new RegExp(theme));
    expect(
      (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
    ).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({
      path: info.outputPath("challenges-" + theme + ".png"),
      fullPage: true,
    });
  }
});

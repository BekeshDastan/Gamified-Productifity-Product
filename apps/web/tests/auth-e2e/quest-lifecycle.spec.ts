import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
function card(page: Page, name: string) {
  return page.locator("section").filter({ has: page.getByRole("heading", { name, exact: true }) });
}
async function account(page: Page) {
  const email = "quest-flow-" + crypto.randomUUID() + "@example.test";
  const password = "quest lifecycle password phrase";
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
async function create(page: Page, title: string, deadline?: string) {
  await page.goto("/app/quests");
  await page.getByRole("button", { name: "Создать задачу", exact: true }).click();
  await page.getByLabel("Название задачи").fill(title);
  if (deadline) await page.getByLabel("Дедлайн с часовым поясом", { exact: true }).fill(deadline);
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  const savedLink = page
    .getByRole("status")
    .filter({ hasText: "Задача сохранена" })
    .getByRole("link");
  const savedPath = await savedLink.getAttribute("href");
  await savedLink.click();
  await expect(page).toHaveURL(savedPath!);
  await expect(page.getByLabel("Название задачи")).toHaveValue(title);
  return page.url().split("/").at(-1)!;
}
async function action(page: Page, name: string, status: string, reason?: string, date?: string) {
  const panel = card(page, "Следующий шаг");
  await panel.getByRole("button", { name, exact: true }).click();
  if (reason) await panel.getByLabel("Причина действия").fill(reason);
  if (date) await panel.getByLabel("Новая плановая дата", { exact: true }).fill(date);
  await panel.getByRole("button", { name: "Подтвердить действие", exact: true }).click();
  await expect(page.getByLabel("Статус", { exact: true })).toHaveValue(status);
}
test("lifecycle history and private quarantine work through the real UI", async ({
  page,
}, info) => {
  test.setTimeout(120_000);
  await account(page);
  const id = await create(page, "Пройти материал", "2026-10-14T18:00:00+05:00");
  await action(page, "Начать", "in_progress");
  await action(page, "Перенести", "postponed", "Встреча заняла время", "2026-10-22");
  await expect(page.getByLabel("Дедлайн с часовым поясом", { exact: true })).toHaveValue(
    "2026-10-14T13:00:00Z",
  );
  await action(page, "К выполнению", "todo");
  await action(page, "Пропустить", "skipped", "Пересмотр приоритета");
  await action(page, "Открыть снова", "todo");
  await action(page, "Не получилось", "failed", "Потребовалась помощь");
  await action(page, "Открыть снова", "todo");
  await action(page, "Завершить", "completed");
  await page.reload();
  await expect(page.getByLabel("Статус", { exact: true })).toHaveValue("completed");
  const history = card(page, "История задачи");
  await expect(history).toContainText("Встреча заняла время");
  await expect(history).toContainText("Пересмотр приоритета");
  await expect(history).toContainText("Потребовалась помощь");
  const events = (await (await page.request.get("/api/v1/quests/" + id + "/history")).json()).data;
  expect(events.filter((e: { kind: string }) => e.kind === "complete")).toHaveLength(1);
  await page.getByLabel("Файл для задачи").setInputFiles({
    name: "notes.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("Private acceptance notes"),
  });
  await page.getByRole("button", { name: "Загрузить файл", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "ожидает проверки" })).toBeVisible();
  const files = (await (await page.request.get("/api/v1/quests/" + id + "/attachments")).json())
    .data;
  expect(files).toHaveLength(1);
  expect(files[0].verdict).toBe("pending");
  expect(
    (await page.request.get("/api/v1/attachments/" + files[0].id + "/download")).status(),
  ).toBe(409);
  await page.getByRole("button", { name: "Повторить проверку", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "остаётся закрытым" })).toBeVisible();
  await page.getByLabel("Файл для задачи").setInputFiles({
    name: "fake.png",
    mimeType: "image/png",
    buffer: Buffer.from("<html>not an image</html>"),
  });
  await page.getByRole("button", { name: "Загрузить файл", exact: true }).click();
  await expect(
    page.getByRole("alert").filter({ hasText: "расширение должно совпадать" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Удалить notes.txt", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Файл удалён." })).toBeVisible();
  await page.getByLabel("Файл для задачи").setInputFiles({
    name: "limit.txt",
    mimeType: "text/plain",
    buffer: Buffer.alloc(10 * 1024 * 1024, "a"),
  });
  await page.getByRole("button", { name: "Загрузить файл", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "ожидает проверки" })).toBeVisible();
  await page.getByRole("button", { name: "Удалить limit.txt", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Файл удалён." })).toBeVisible();
  await page.getByLabel("Название задачи").fill("После собственных действий");
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Сохранено." })).toBeVisible();
  await page.setViewportSize({ width: 360, height: 800 });
  expect(
    (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
  ).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({ path: info.outputPath("quest-history-mobile.png"), fullPage: true });
});
test("subtasks roll up and templates filters Kanban and bulk archive preserve outcomes", async ({
  page,
}, info) => {
  test.setTimeout(150_000);
  await account(page);
  const parent = await create(page, "План недели");
  await page.getByRole("button", { name: "Добавить подзадачу", exact: true }).click();
  const form = card(page, "Новая задача");
  await form.getByLabel("Название задачи").fill("Обязательная глава");
  await form.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  const childLink = page
    .getByRole("status")
    .filter({ hasText: "Задача сохранена" })
    .getByRole("link");
  const childPath = await childLink.getAttribute("href");
  await childLink.click();
  await expect(page).toHaveURL(childPath!);
  const child = page.url().split("/").at(-1)!;
  expect(child).not.toBe(parent);
  await expect(page.getByLabel("Название задачи")).toHaveValue("Обязательная глава");
  await action(page, "Завершить", "completed");
  await page.goto("/app/quests/" + parent);
  await expect(page.getByLabel("Статус", { exact: true })).toHaveValue("completed");
  await expect(card(page, "Подзадачи и блокировки")).toContainText("1 из 1");
  await page.goto("/app/quests/" + child);
  await page.getByLabel("Название нового шаблона").fill("Чтение");
  await page.getByRole("button", { name: "Сохранить как шаблон", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Шаблон сохранён." })).toBeVisible();
  await page.getByLabel("Выберите шаблон").selectOption({ label: "Чтение" });
  await page.getByRole("button", { name: "Предпросмотр задачи", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Будет создана одна задача" })).toBeVisible();
  await page.getByRole("button", { name: "Создать из шаблона", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Задача сохранена" })).toBeVisible();
  await page.goto("/app/quests");
  await page.getByLabel("Поиск задач").fill("Обязательная");
  await page.getByRole("button", { name: "Применить фильтры", exact: true }).click();
  await expect(page.getByLabel("Поиск задач")).toHaveValue("Обязательная");
  await expect(page.locator(".quest-list a")).toHaveCount(2);
  await page.getByLabel("Название представления").fill("Главы");
  await page.getByRole("button", { name: "Сохранить представление", exact: true }).click();
  await expect(page.getByRole("button", { name: "Главы", exact: true })).toBeVisible();
  await page.reload();
  await page.getByRole("button", { name: "Главы", exact: true }).click();
  await expect(page.locator(".quest-list a")).toHaveCount(2);
  const list = await page
    .locator(".quest-list a")
    .evaluateAll((a) => a.map((x) => x.getAttribute("href")).sort());
  await page.getByRole("button", { name: "Доска", exact: true }).click();
  const board = await page
    .locator(".quest-kanban a")
    .evaluateAll((a) => a.map((x) => x.getAttribute("href")).sort());
  expect(board).toEqual(list);
  for (const checkbox of await page
    .getByRole("checkbox", { name: "Выбрать: Обязательная глава", exact: true })
    .all())
    await checkbox.check();
  await page.getByRole("button", { name: "Предпросмотр изменений (2)", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Изменится задач: 2" })).toBeVisible();
  await page.getByRole("button", { name: "Применить ко всем выбранным", exact: true }).click();
  await expect(page.locator(".quest-kanban a")).toHaveCount(0);
  await page.getByLabel("Показать архив").check();
  await page.getByRole("button", { name: "Применить фильтры", exact: true }).click();
  await expect(page.locator(".quest-kanban a")).toHaveCount(2);
  for (const checkbox of await page
    .getByRole("checkbox", { name: "Выбрать: Обязательная глава", exact: true })
    .all())
    await checkbox.check();
  await page.getByLabel("Пакетное действие").selectOption("restore");
  await page.getByRole("button", { name: "Предпросмотр изменений (2)", exact: true }).click();
  await page.getByRole("button", { name: "Применить ко всем выбранным", exact: true }).click();
  await expect(page.locator(".quest-kanban a")).toHaveCount(0);
  const saved = (await (await page.request.get("/api/v1/quests/" + child)).json()).data;
  expect(saved.status).toBe("completed");
  expect(saved.archived_at).toBeNull();
  await page.getByRole("button", { name: "Сбросить", exact: true }).click();
  await page.getByRole("button", { name: "Список", exact: true }).click();
  await expect(page.locator(".quest-list a")).toHaveCount(3);
  await page.setViewportSize({ width: 360, height: 800 });
  await expect(page.getByLabel("Тема оформления")).toBeEnabled();
  await page.getByLabel("Тема оформления").selectOption("dark");
  await expect(page.getByLabel("Тема оформления")).toHaveValue("dark");
  await expect(page.locator("html")).toHaveClass(/dark/);
  expect(
    (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
  ).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({ path: info.outputPath("quest-list-dark-mobile.png"), fullPage: true });
});

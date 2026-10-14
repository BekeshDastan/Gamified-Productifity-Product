import { revealControl } from "./disclosure";
import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
test("quest fields persist after reload and stale editor keeps local input", async ({
  page,
  context,
}, info) => {
  const email = "quest-" + crypto.randomUUID() + "@example.test";
  const password = "quest browser password phrase";
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
  await page.goto("/app/quests");
  await page.getByRole("button", { name: "Создать задачу", exact: true }).click();
  await revealControl(page.getByLabel("Название задачи"));
  await page.getByLabel("Название задачи").fill("Прочитать главу");
  await revealControl(page.getByLabel("Описание", { exact: true }));
  await page.getByLabel("Описание", { exact: true }).fill("Заметки к первой главе");
  await revealControl(page.getByLabel("Приоритет", { exact: true }));
  await page.getByLabel("Приоритет", { exact: true }).selectOption("high");
  await revealControl(page.getByLabel("Сложность (1–5)"));
  await page.getByLabel("Сложность (1–5)").fill("3");
  await page.getByLabel("Плановая дата", { exact: true }).fill("2026-10-13");
  await revealControl(page.getByLabel("Дедлайн с часовым поясом"));
  await page.getByLabel("Дедлайн с часовым поясом").fill("2026-10-14T18:00:00+05:00");
  await revealControl(page.getByLabel("Оценка, минут"));
  await page.getByLabel("Оценка, минут").fill("30");
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Задача сохранена" })).toBeVisible();
  await page.getByRole("status").filter({ hasText: "Задача сохранена" }).getByRole("link").click();
  await expect(page).toHaveURL(/\/app\/quests\/[a-f0-9-]+$/);
  await page.reload();
  await expect(page.getByLabel("Название задачи")).toHaveValue("Прочитать главу");
  await expect(page.getByLabel("Описание", { exact: true })).toHaveValue("Заметки к первой главе");
  await expect(page.getByLabel("Приоритет", { exact: true })).toHaveValue("high");
  await expect(page.getByLabel("Плановая дата", { exact: true })).toHaveValue("2026-10-13");
  await expect(page.getByLabel("Дедлайн с часовым поясом")).toHaveValue("2026-10-14T13:00:00Z");
  const stale = await context.newPage();
  await stale.goto(page.url());
  await expect(stale.getByLabel("Название задачи")).toHaveValue("Прочитать главу");
  await revealControl(page.getByLabel("Название задачи"));
  await page.getByLabel("Название задачи").fill("Сохранено из первого окна");
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Сохранено." })).toBeVisible();
  await revealControl(stale.getByLabel("Название задачи"));
  await stale.getByLabel("Название задачи").fill("Мои несохранённые изменения");
  await stale.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await expect(stale.getByRole("alert").filter({ hasText: "Запись изменилась" })).toBeVisible();
  await expect(stale.getByLabel("Название задачи")).toHaveValue("Мои несохранённые изменения");
  await stale.close();
  await page.setViewportSize({ width: 360, height: 800 });
  expect(
    (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
  ).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({ path: info.outputPath("quest-mobile.png"), fullPage: true });
});

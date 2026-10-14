import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
function card(page: Page, name: string) {
  return page.locator("section").filter({ has: page.getByRole("heading", { name, exact: true }) });
}
async function account(page: Page) {
  const email = "goal-" + crypto.randomUUID() + "@example.test";
  const password = "goal browser password phrase";
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
async function decision(page: Page, action: string) {
  await page.getByLabel("Действие с целью").selectOption(action);
  await page.getByLabel("Причина решения").fill("Проверенное решение");
  await page.getByRole("button", { name: "Сохранить решение", exact: true }).click();
}
test("goal metric correction review archive and stale criteria work in the real UI", async ({
  page,
  context,
}, info) => {
  test.setTimeout(120_000);
  await account(page);
  await page.goto("/app/goals");
  await page.getByRole("button", { name: "Создать цель", exact: true }).click();
  await page.getByLabel("Название цели", { exact: true }).fill("Прочитать две книги");
  await page.getByLabel("Критерий успеха", { exact: true }).fill("Прочитать и записать выводы");
  await page
    .getByRole("group", { name: "Способ расчёта прогресса", exact: true })
    .getByRole("button", { name: "Числовой результат", exact: true })
    .click();
  await page.getByLabel("Целевое значение").fill("2");
  await page.getByLabel("Единица результата").fill("книги");
  await page.getByLabel("Начальное состояние цели").selectOption("active");
  await page.getByRole("button", { name: "Сохранить цель", exact: true }).click();
  await page.getByRole("status").filter({ hasText: "Цель сохранена" }).getByRole("link").click();
  await expect(page.getByRole("progressbar", { name: "Прогресс цели" })).toHaveAttribute(
    "value",
    "0",
  );
  await page.getByLabel("Добавить результат", { exact: true }).fill("2");
  await page.getByLabel("Дата результата", { exact: true }).fill("2026-10-12");
  await page.getByRole("button", { name: "Записать результат", exact: true }).click();
  await expect(page.getByRole("progressbar", { name: "Прогресс цели" })).toHaveAttribute(
    "value",
    "100",
  );
  await expect(card(page, "Прочитать две книги")).toContainText("Активна");
  await page.getByLabel("Действие с целью").selectOption("achieve");
  await page.getByLabel(/Подтверждаю: критерий/).check();
  await decision(page, "achieve");
  await expect(card(page, "Прочитать две книги")).toContainText("Достигнута");
  await page.getByText("Исправить результат", { exact: true }).click();
  await page.getByLabel("Исправленное значение").fill("1");
  await page.getByLabel("Причина исправления результата").fill("Вторая книга ещё не закончена");
  await page.getByRole("button", { name: "Сохранить исправление", exact: true }).click();
  await expect(page.getByRole("progressbar", { name: "Прогресс цели" })).toHaveAttribute(
    "value",
    "50",
  );
  await expect(card(page, "Прочитать две книги")).toContainText("Активна");
  await expect(card(page, "История цели")).toContainText("Достижение отменено");
  await page.getByRole("button", { name: "Изменить цель", exact: true }).click();
  const stale = await context.newPage();
  await stale.goto(page.url());
  await stale.getByRole("button", { name: "Изменить цель", exact: true }).click();
  await page
    .getByLabel("Критерий успеха", { exact: true })
    .fill("Прочитать и написать два конспекта");
  await page.getByLabel("Причина изменения плана").fill("Уточнил критерий");
  await page.getByRole("button", { name: "Сохранить цель", exact: true }).click();
  await expect(page.getByLabel("Причина изменения плана")).toHaveValue("");
  await stale.getByLabel("Критерий успеха", { exact: true }).fill("Мои локальные условия");
  await stale.getByLabel("Причина изменения плана").fill("Другой план");
  await stale.getByRole("button", { name: "Сохранить цель", exact: true }).click();
  await expect(stale.getByRole("alert").filter({ hasText: "Запись изменилась" })).toBeVisible();
  await expect(stale.getByLabel("Критерий успеха", { exact: true })).toHaveValue(
    "Мои локальные условия",
  );
  await stale.close();
  await page.getByRole("button", { name: "Закрыть редактор цели", exact: true }).click();
  await page.getByLabel("Действие с целью").selectOption("close-unachieved");
  await page.getByLabel("Что удалось выполнить").fill("Одна книга");
  await page.getByLabel("Что помешало").fill("Не хватило времени");
  await decision(page, "close-unachieved");
  await expect(card(page, "Прочитать две книги")).toContainText("Завершена без достижения");
  await decision(page, "archive");
  await expect(page.getByLabel("Действие с целью")).toHaveValue("restore");
  await page.reload();
  await expect(card(page, "Прочитать две книги")).toContainText("Архив");
  await decision(page, "restore");
  await page.getByLabel("Действие с целью").selectOption("review");
  await page.getByLabel("Новая целевая дата").fill("2026-11-30");
  await page.getByLabel("Пересмотренный критерий").fill("Прочитать обе книги без спешки");
  await decision(page, "review");
  await expect(card(page, "Прочитать две книги")).toContainText("Активна");
  await expect(card(page, "История цели")).toContainText("Уточнил критерий");
  await page.setViewportSize({ width: 360, height: 800 });
  expect(
    (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
  ).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath("goal-metric-light-mobile.png"), fullPage: true });
});
test("goal template preview creates a plan and quest events update its progress", async ({
  page,
}, info) => {
  test.setTimeout(120_000);
  await account(page);
  await page.goto("/app/goals");
  await page.getByRole("button", { name: "Подготовка к экзамену", exact: true }).click();
  await page.getByLabel("Начало плана").fill("2026-12-25");
  await page.getByLabel("Задача 1", { exact: true }).fill("Разобрать свою программу");
  await page.getByRole("button", { name: "Показать план и даты", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Предпросмотр плана" })).toBeVisible();
  await expect(page.getByText("2026-12-25 — 2027-01-23", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Создать цель и задачи", exact: true }),
  ).toBeDisabled();
  await page.getByLabel("Я проверил даты, задачи и нагрузку").check();
  await page.getByRole("button", { name: "Создать цель и задачи", exact: true }).click();
  await page.getByRole("status").filter({ hasText: "Цель сохранена" }).getByRole("link").click();
  await expect(page).toHaveURL(/\/app\/goals\/[a-f0-9-]+$/);
  const goalURL = page.url();
  await decision(page, "resume");
  await expect(card(page, "Подготовка к экзамену")).toContainText("Активна");
  await page.getByRole("link", { name: "Разобрать свою программу", exact: true }).click();
  const task = card(page, "Следующий шаг");
  await task.getByRole("button", { name: "Завершить", exact: true }).click();
  await task.getByRole("button", { name: "Подтвердить действие", exact: true }).click();
  await expect(page.getByLabel("Статус", { exact: true })).toHaveValue("completed");
  await page.goto(goalURL);
  await expect(page.getByRole("progressbar", { name: "Прогресс цели" })).toHaveAttribute(
    "value",
    /^33\.33/,
  );
  await page.getByRole("link", { name: "Разобрать свою программу", exact: true }).click();
  await task.getByRole("button", { name: "Открыть снова", exact: true }).click();
  await task.getByRole("button", { name: "Подтвердить действие", exact: true }).click();
  await expect(page.getByLabel("Статус", { exact: true })).toHaveValue("todo");
  await page.goto(goalURL);
  await expect(page.getByRole("progressbar", { name: "Прогресс цели" })).toHaveAttribute(
    "value",
    "0",
  );
  await page.getByLabel("Тема оформления").selectOption("dark");
  await expect(page.locator("html")).toHaveClass(/dark/);
  await page.setViewportSize({ width: 360, height: 800 });
  expect(
    (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
  ).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath("goal-plan-dark-mobile.png"), fullPage: true });
});

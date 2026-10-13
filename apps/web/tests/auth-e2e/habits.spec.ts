import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
async function account(page: Page) {
  await page.clock.setFixedTime("2026-10-12T12:00:00Z");
  const email = "habit-" + crypto.randomUUID() + "@example.test",
    password = "habit browser password phrase";
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
  return { email, password };
}
function habit(page: Page, title: string) {
  return page
    .locator("section.habit-card")
    .filter({ has: page.getByRole("heading", { name: title, exact: true }) });
}
test("habits quantitative progress corrections future norm archive and dashboard persist", async ({
  page,
}, info) => {
  test.setTimeout(120000);
  await account(page);
  await page.goto("/app/onboarding");
  for (const name of [
    "Ваше время",
    "Направления",
    "Первая цель",
    "Первая задача",
    "Первая привычка",
  ]) {
    await page.getByRole("button", { name: "Пропустить шаг", exact: true }).click();
    await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
  }
  await page.getByLabel("Название черновика").fill("Читать двадцать страниц");
  await page.getByRole("button", { name: "Сохранить и продолжить", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Ваше пространство готово", exact: true }),
  ).toBeVisible();
  await page.goto("/app/habits");
  await expect(habit(page, "Читать двадцать страниц")).toHaveCount(0);
  await page.getByRole("button", { name: "Настроить стартовую привычку", exact: true }).click();
  await expect(page.getByLabel("Название привычки")).toHaveValue("Читать двадцать страниц");
  await page.getByRole("button", { name: "Добавить привычку", exact: true }).click();
  const h = habit(page, "Читать двадцать страниц");
  await expect(h).toBeVisible();
  await h.getByLabel("Количество (страниц)", { exact: true }).fill("10");
  await h.getByRole("button", { name: "Добавить отметку", exact: true }).click();
  await expect(h.getByRole("progressbar")).toHaveAttribute("value", "50");
  await h.getByLabel("Количество (страниц)", { exact: true }).fill("10");
  await h.getByRole("button", { name: "Добавить отметку", exact: true }).click();
  await expect(h.getByRole("progressbar")).toHaveAttribute("value", "100");
  await expect(h).toContainText("Серия: 1");
  await page.reload();
  await expect(h.getByRole("progressbar")).toHaveAttribute("value", "100");
  await h.getByText("История периодов и исправлений", { exact: true }).click();
  await h.getByText("Исправить отметку", { exact: true }).first().click();
  await h.getByLabel("Исправленное количество").first().fill("0");
  await h.getByLabel("Причина исправления").first().fill("Лишняя отметка");
  await h.getByRole("button", { name: "Сохранить исправление", exact: true }).first().click();
  await expect(h.getByRole("progressbar")).toHaveAttribute("value", "50");
  await expect(h).toContainText("Лишняя отметка");
  await h.getByText("Будущее расписание и нормы", { exact: true }).click();
  await h.getByLabel("Дата новой нормы").fill("2026-10-13");
  await h.getByLabel("Новая норма", { exact: true }).fill("30");
  await h.getByLabel("Причина смены нормы").fill("Пробую новую норму завтра");
  await h.getByRole("button", { name: "Предпросмотр нормы", exact: true }).click();
  await expect(h.getByLabel("Предпросмотр нормы", { exact: true })).toContainText("20 → 30");
  await expect(h.getByRole("progressbar")).toHaveAttribute("value", "50");
  // A different form updates the record after the user has reviewed a schedule.
  await h.getByLabel("Количество (страниц)", { exact: true }).fill("1");
  await h.getByRole("button", { name: "Добавить отметку", exact: true }).click();
  await expect(h.getByRole("progressbar")).toHaveAttribute("value", "55");
  await expect(h.getByLabel("Новая норма", { exact: true })).toHaveValue("30");
  await expect(h.getByLabel("Причина смены нормы")).toHaveValue("Пробую новую норму завтра");
  await h.getByLabel("Я подтверждаю новую норму с указанной даты").check();
  await h.getByRole("button", { name: "Применить новую норму", exact: true }).click();
  await expect(
    h.getByRole("button", { name: "Загрузить свежую версию, сохранив ввод" }),
  ).toBeVisible();
  await expect(h.getByText(/С 2026-10-13: 30/)).toHaveCount(0);
  await h.getByRole("button", { name: "Загрузить свежую версию, сохранив ввод" }).click();
  await expect(
    h.getByRole("button", { name: "Загрузить свежую версию, сохранив ввод" }),
  ).toHaveCount(0);
  await h.getByRole("button", { name: "Предпросмотр нормы", exact: true }).click();
  await expect(h.getByLabel("Предпросмотр нормы", { exact: true })).toContainText("20 → 30");
  await h.getByLabel("Я подтверждаю новую норму с указанной даты").check();
  await h.getByRole("button", { name: "Применить новую норму", exact: true }).click();
  await expect(h.getByText(/С 2026-10-13: 30/)).toBeVisible();
  await h.getByText("Убрать в архив", { exact: true }).click();
  await h.getByLabel("Причина архива").fill("Вернусь позже");
  await h.getByRole("button", { name: "Архивировать привычку", exact: true }).click();
  await expect(h).toHaveCount(0);
  await page.getByRole("button", { name: "Показать архив", exact: true }).click();
  await expect(h).toContainText("Лишняя отметка");
  await h
    .locator("summary")
    .filter({ hasText: /^Вернуть привычку$/ })
    .click();
  await h.getByLabel("Причина возврата").fill("Продолжаю чтение");
  await h.getByRole("button", { name: "Вернуть привычку", exact: true }).click();
  await expect(h).toHaveCount(0);
  await page.getByRole("button", { name: "Показать активные", exact: true }).click();
  await expect(h.getByRole("progressbar")).toHaveAttribute("value", "55");
  await page.goto("/app");
  const quick = habit(page, "Читать двадцать страниц");
  await quick.getByLabel("Количество (страниц)", { exact: true }).fill("10");
  await quick.getByRole("button", { name: "Добавить отметку", exact: true }).click();
  await expect(quick.getByRole("progressbar")).toHaveAttribute("value", "100");
  await page.goto("/app/habits");
  await page.setViewportSize({ width: 360, height: 800 });
  for (const theme of ["light", "dark"]) {
    await page.getByLabel("Тема оформления").selectOption(theme);
    expect(
      (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
    ).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({
      path: info.outputPath("habits-" + theme + "-mobile.png"),
      fullPage: true,
    });
  }
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(h.getByRole("progressbar")).toHaveAttribute("value", "100");
});

test("habits core goal task calendar flow and weekly norm", async ({ page }) => {
  test.setTimeout(120000);
  await account(page);
  await page.goto("/app/goals");
  await page.getByRole("button", { name: "Подготовка к экзамену", exact: true }).click();
  await page.getByLabel("Начало плана").fill("2026-10-12");
  await page.getByLabel("Задача 1", { exact: true }).fill("Разобрать учебную программу");
  await page.getByRole("button", { name: "Показать план и даты", exact: true }).click();
  await page.getByLabel("Я проверил даты, задачи и нагрузку").check();
  await page.getByRole("button", { name: "Создать цель и задачи", exact: true }).click();
  await page.getByRole("status").filter({ hasText: "Цель сохранена" }).getByRole("link").click();
  await expect(page).toHaveURL(/\/app\/goals\/[a-f0-9-]+$/);
  const goalURL = page.url();
  await page.getByLabel("Действие с целью").selectOption("resume");
  await page.getByLabel("Причина решения").fill("Начинаю подготовку");
  await page.getByRole("button", { name: "Сохранить решение", exact: true }).click();
  await expect(page.getByLabel("Действие с целью")).not.toHaveValue("resume");
  await page.getByRole("link", { name: "Разобрать учебную программу", exact: true }).click();
  await expect(page).toHaveURL(/\/app\/quests\/[a-f0-9-]+$/);
  const taskURL = page.url();
  await page.goto("/app/calendar");
  await page.getByRole("button", { name: "Добавить событие", exact: true }).click();
  await page.getByLabel("Название события").fill("Время для учебной программы");
  await page
    .getByLabel("Поиск: Связанная задача", { exact: true })
    .fill("Разобрать учебную программу");
  await page
    .getByLabel("Связанная задача", { exact: true })
    .selectOption({ label: "Разобрать учебную программу" });
  await page.getByLabel("Без времени", { exact: true }).uncheck();
  await page.getByLabel("Начало события").fill("2026-10-12T09:00");
  await page.getByLabel("Конец события").fill("2026-10-12T09:30");
  await page.getByRole("button", { name: "Сохранить событие", exact: true }).click();
  await expect(page.getByLabel("Название события")).toHaveCount(0);
  await page.goto(taskURL);
  const action = page
    .locator("section")
    .filter({ has: page.getByRole("heading", { name: "Следующий шаг", exact: true }) });
  await action.getByRole("button", { name: "Завершить", exact: true }).click();
  await action.getByRole("button", { name: "Подтвердить действие", exact: true }).click();
  await expect(page.getByLabel("Статус", { exact: true })).toHaveValue("completed");
  await page.goto(goalURL);
  await expect(page.getByRole("progressbar", { name: "Прогресс цели" })).toHaveAttribute(
    "value",
    /^33\.33/,
  );
  await page.goto("/app/habits");
  await page.getByLabel("Название привычки").fill("Учебная практика трижды в неделю");
  await page.getByLabel("Тип привычки").selectOption("binary");
  await page.getByLabel("Период нормы", { exact: true }).selectOption("weekly");
  await page.getByLabel("Норма периода", { exact: true }).fill("3");
  await expect(page.getByLabel("Название привычки")).toHaveValue(
    "Учебная практика трижды в неделю",
  );
  await page.getByRole("button", { name: "Добавить привычку", exact: true }).click();
  const h = habit(page, "Учебная практика трижды в неделю");
  await expect(h).toBeVisible();
  for (const progress of ["33", "66", "100"]) {
    await h.getByRole("button", { name: "Добавить отметку", exact: true }).click();
    await expect(h.getByRole("progressbar")).toHaveAttribute("value", progress);
  }
  await expect(h).toContainText("Серия: 1");
  await h.getByText("Календарь отметок", { exact: true }).click();
  await expect(h.locator(".habit-heatmap > li")).toHaveCount(1);
  await page.reload();
  await expect(h.getByRole("progressbar")).toHaveAttribute("value", "100");
});

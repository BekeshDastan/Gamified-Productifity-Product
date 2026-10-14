import { revealControl } from "./disclosure";
import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
test("navigator explanations opt-out and private sources work without external network", async ({
  page,
}, info) => {
  test.setTimeout(120000);
  await page.route("**/*", (route) => {
    const url = new URL(route.request().url());
    return url.hostname === "localhost" || url.hostname === "127.0.0.1"
      ? route.continue()
      : route.abort();
  });
  await page.goto("/auth/register");
  const email = "navigator-" + crypto.randomUUID() + "@example.test",
    password = "navigator browser password phrase";
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
  await page
    .getByRole("navigation", { name: "Разделы приложения" })
    .getByRole("link", { name: "Навигатор", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Главная цель месяца", exact: true }),
  ).toBeVisible();
  await page.goto("/app/planning?date=2026-10-13");
  await page.getByLabel("Доступно минут", { exact: true }).fill("60");
  await page.getByRole("button", { name: "Добавить рабочее окно", exact: true }).click();
  await page.getByLabel("Начало окна 1", { exact: true }).fill("13:00");
  await page.getByLabel("Конец окна 1", { exact: true }).fill("14:00");
  await page.getByRole("button", { name: "Сохранить черновик", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Блоки работы и перенос", exact: true }),
  ).toBeVisible();
  await page.goto("/app/quests");
  await page.getByRole("button", { name: "Создать задачу", exact: true }).click();
  await revealControl(page.getByLabel("Название задачи", { exact: true }));
  await page.getByLabel("Название задачи", { exact: true }).fill("Шаг для расписания");
  await revealControl(page.getByLabel("Оценка, минут", { exact: true }));
  await page.getByLabel("Оценка, минут", { exact: true }).fill("45");
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Задача сохранена" })).toBeVisible();
  await page.goto("/app/navigator");
  await page.getByLabel("Первый день расписания", { exact: true }).fill("2026-10-13");
  await page
    .getByLabel("Последний день расписания включительно", { exact: true })
    .fill("2026-10-13");
  await page.getByLabel("Выбрать «Шаг для расписания»", { exact: true }).check();
  await page
    .getByRole("button", { name: "Рассчитать предложение расписания", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Предпросмотр расписания", exact: true }),
  ).toBeVisible();
  const apply = page.getByRole("button", { name: "Применить предложение расписания", exact: true });
  await expect(apply).toBeDisabled();
  await page
    .getByLabel("Подтверждаю сохранение предложенных блоков и дополнений планов", { exact: true })
    .check();
  await apply.click();
  await expect(
    page.getByRole("status").filter({ hasText: "Сохранено календарных блоков: 2" }),
  ).toBeVisible();
  const calendar = (
    await (await page.request.get("/api/v1/calendar?from=2026-10-13&to=2026-10-14")).json()
  ).data;
  expect(
    calendar.items.filter(
      (e: { title: string; source: string }) =>
        e.title === "Шаг для расписания" && e.source === "block",
    ),
  ).toHaveLength(1);
  await expect(page.getByRole("heading", { name: "Deadline Radar", exact: true })).toBeVisible();
  await page.getByText("Почему появилась эта подсказка", { exact: true }).click();
  await expect(page.getByText("Босс месяца не выбран", { exact: true })).toBeVisible();
  await page
    .getByRole("button", { name: "Отключить подсказки: Главная цель месяца", exact: true })
    .click();
  await expect(page.getByRole("heading", { name: "Главная цель месяца", exact: true })).toHaveCount(
    0,
  );
  await page.reload();
  await expect(page.getByRole("heading", { name: "Главная цель месяца", exact: true })).toHaveCount(
    0,
  );
  await page.getByText("Какие подсказки показывать", { exact: true }).click();
  await page
    .getByRole("button", { name: "Включить подсказки: Главная цель месяца", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Главная цель месяца", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Личный проект", exact: true }).click();
  await page.getByLabel("Название цели", { exact: true }).fill("Мой проверяемый проект");
  await page.getByLabel("Начало плана", { exact: true }).fill("2026-10-13");
  await page.getByRole("button", { name: "Далее по шагам цели", exact: true }).click();
  await page.getByLabel("Этап 1", { exact: true }).fill("Описать небольшой результат");
  await page.getByRole("button", { name: "Далее по шагам цели", exact: true }).click();
  await page.getByLabel("Задача 1", { exact: true }).fill("Записать первый критерий");
  await page.getByLabel("Оценка задачи 1, минут", { exact: true }).fill("15");
  await page.getByRole("button", { name: "Показать план и даты", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Предпросмотр плана", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Этап 1: Описать небольшой результат", { exact: false }),
  ).toBeVisible();
  const beforeGoals = (await (await page.request.get("/api/v1/goals")).json()).data;
  expect(beforeGoals.total).toBe(0);
  const create = page.getByRole("button", { name: "Создать цель и задачи", exact: true });
  await expect(create).toBeDisabled();
  await page.getByLabel("Я проверил даты, задачи и нагрузку", { exact: true }).check();
  await create.click();
  await expect(
    page.getByRole("status").filter({ hasText: "Цель и показанные шаги созданы" }),
  ).toBeVisible();
  const afterGoals = (await (await page.request.get("/api/v1/goals")).json()).data;
  expect(afterGoals.total).toBe(1);
  await expect(
    page.getByRole("heading", { name: "Future Self: объём практики", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText(/Недостаточно данных: нужны минимум две полные наблюдавшиеся недели/),
  ).toBeVisible();
  const response = page.waitForResponse((r) => r.url().includes("/analytics/future-self?weeks=26"));
  await page.getByRole("button", { name: "26 нед", exact: true }).click();
  expect((await (await response).json()).data.projected_hours).toBeNull();
  await page.setViewportSize({ width: 390, height: 844 });
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath("navigator-mobile.png"), fullPage: true });
});

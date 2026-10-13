import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
async function account(page: Page) {
  await page.clock.setFixedTime("2026-10-12T12:00:00Z");
  await page.addInitScript(() => {
    Object.defineProperty(window, "__focusAudioAttempts", { value: 0, writable: true });
    Object.defineProperty(window, "AudioContext", {
      value: function () {
        const w = window as unknown as Window & { __focusAudioAttempts: number };
        w.__focusAudioAttempts++;
        throw Error("Unexpected audio while sound is disabled");
      },
    });
  });
  const email = "focus-" + crypto.randomUUID() + "@example.test",
    password = "focus browser password phrase";
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
function card(page: Page, title: string) {
  return page
    .locator("section")
    .filter({ has: page.getByRole("heading", { name: title, exact: true, level: 2 }) });
}
async function task(page: Page, title: string) {
  await page.goto("/app/quests");
  await page.getByRole("button", { name: "Создать задачу", exact: true }).click();
  await page.getByLabel("Название задачи").fill(title);
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await page.getByRole("status").filter({ hasText: "Задача сохранена" }).getByRole("link").click();
  await expect(page).toHaveURL(/\/app\/quests\/[a-f0-9-]+$/);
  return page.url();
}
async function confirm(page: Page, result: "progress" | "rest" = "progress") {
  await expect(page.getByLabel("Результат фокус-сессии")).toBeVisible();
  await page.getByLabel("Результат фокус-сессии").selectOption(result);
  await page.getByLabel("Комментарий к фокус-сессии").fill("Проверенный итог");
  await page.getByRole("button", { name: "Подтвердить итог сессии", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Итог сохранён" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Новая сессия", exact: true })).toBeVisible();
}
async function accessible(page: Page, name: string, info: { outputPath: (s: string) => string }) {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.setViewportSize({ width: 360, height: 800 });
  for (const theme of ["light", "dark"]) {
    await page.getByLabel("Тема оформления", { exact: true }).selectOption(theme);
    await expect(page.locator("html")).toHaveClass(new RegExp(theme));
    expect(
      (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
    ).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({ path: info.outputPath(name + "-" + theme + ".png"), fullPage: true });
  }
}
test("focus restores across two tabs and confirmed task time has one source", async ({
  page,
  context,
}, info) => {
  test.setTimeout(180000);
  await account(page);
  const taskURL = await task(page, "Сосредоточиться на одном шаге");
  await page.goto("/app/focus");
  await page
    .getByLabel("Задача для фокуса", { exact: true })
    .selectOption({ label: "Сосредоточиться на одном шаге" });
  await page.getByRole("button", { name: "Начать фокус-сессию", exact: true }).click();
  await expect(page.getByText("Таймер идёт", { exact: true })).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Сосредоточиться на одном шаге", exact: true, level: 2 }),
  ).toBeVisible();
  await expect(page.getByRole("heading", { name: "Новая сессия", exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Пауза", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: /^Пауза$/ })).toBeVisible();
  const frozen = await page.getByLabel("Активное рабочее время", { exact: true }).innerText();
  const second = await context.newPage();
  await second.clock.setFixedTime("2026-10-12T12:00:00Z");
  await second.goto("/app/focus");
  await expect(second.getByRole("status").filter({ hasText: /^Пауза$/ })).toBeVisible();
  await expect(second.getByLabel("Активное рабочее время", { exact: true })).toHaveText(frozen);
  await second.getByRole("button", { name: "Продолжить таймер", exact: true }).click();
  await expect(second.getByText("Таймер идёт", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Сверить с сервером", exact: true }).click();
  await expect(page.getByText("Таймер идёт", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Завершить и подвести итог", exact: true }).click();
  await expect(page.getByLabel("Результат фокус-сессии")).toBeVisible();
  await second.goto(taskURL);
  await expect(card(second, "Фактическое время задачи")).toContainText("Сохранённый фокус: 0:00");
  await confirm(page);
  await expect(page.locator(".focus-history article")).toHaveCount(1);
  await expect(page.locator(".focus-history")).toContainText("Подтверждена");
  await second.reload();
  const timing = card(second, "Фактическое время задачи");
  await expect(timing).toContainText("подтверждённые фокус-сессии");
  const actual = await timing.locator("strong").innerText();
  expect(actual).not.toBe("0:00");
  await expect(second.getByLabel("Статус", { exact: true })).toHaveValue("todo");
  await second.getByLabel("Фактически, минут", { exact: true }).fill("7");
  await expect(second.getByLabel("Источник фактического времени")).toHaveValue("manual_override");
  await second.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await expect(timing.locator("strong")).toHaveText("7:00");
  await expect(timing).toContainText("ручной итог заменяет сумму фокуса");
  await second.getByLabel("Источник фактического времени").selectOption("from_focus");
  await second.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await expect(timing.locator("strong")).toHaveText(actual);
  await second.goto("/app/progress");
  await expect(second.getByText("Всего: 0 XP", { exact: true })).toBeVisible();
  await second.close();
  await accessible(page, "focus-history", info);
});
test("quick start and Pomodoro boundaries exclude breaks and respect muted sound", async ({
  page,
}, info) => {
  test.setTimeout(180000);
  await account(page);
  const taskURL = await task(page, "Пять минут без давления");
  await page.getByRole("button", { name: "Начать с пяти минут", exact: true }).click();
  await expect(page).toHaveURL("/app/focus");
  await expect(
    page.getByText("Рабочий интервал завершён. Можно закончить или продолжить по своему выбору.", {
      exact: true,
    }),
  ).toBeVisible({ timeout: 30000 });
  await expect(page.getByLabel("Активное рабочее время", { exact: true })).toHaveText("5:00");
  await expect(
    page.getByRole("button", { name: "Продолжить ещё пять минут", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Завершить и подвести итог", exact: true }).click();
  await confirm(page);
  await page.goto(taskURL);
  await expect(page.getByLabel("Статус", { exact: true })).toHaveValue("todo");
  await expect(card(page, "Фактическое время задачи").locator("strong")).toHaveText("5:00");
  await page.goto("/app/focus");
  await page.getByLabel("Режим фокуса", { exact: true }).selectOption("pomodoro");
  await page.getByLabel("Название сессии", { exact: true }).fill("Рабочий ритм");
  await page.getByLabel("Рабочий интервал, минут", { exact: true }).fill("5");
  await page.getByLabel("Перерыв, минут", { exact: true }).fill("1");
  await expect(page.getByLabel("Звук окончания интервала", { exact: true })).toBeDisabled();
  await page.getByRole("button", { name: "Начать фокус-сессию", exact: true }).click();
  await expect(
    page.getByText("Рабочий интервал завершён. Можно закончить или продолжить по своему выбору.", {
      exact: true,
    }),
  ).toBeVisible({ timeout: 30000 });
  await page.getByRole("button", { name: "Начать перерыв", exact: true }).click();
  await expect(
    page.getByText("Перерыв завершён. Следующий рабочий интервал начнётся по вашему выбору.", {
      exact: true,
    }),
  ).toBeVisible({ timeout: 30000 });
  await expect(page.getByLabel("Активное рабочее время", { exact: true })).toHaveText("5:00");
  await expect(page.getByText(/перерывы 1:00/)).toBeVisible();
  expect(
    await page.evaluate(
      () => (window as unknown as Window & { __focusAudioAttempts: number }).__focusAudioAttempts,
    ),
  ).toBe(0);
  await page.getByRole("button", { name: "Завершить и подвести итог", exact: true }).click();
  await confirm(page, "rest");
  await expect(page.getByRole("list", { name: "Итоги фокуса" })).toContainText(
    "Подтверждённая работа: 5:00",
  );
  await expect(page.getByRole("list", { name: "Итоги фокуса" })).toContainText(
    "Отмечено отдыхом: 5:00",
  );
  await accessible(page, "focus-pomodoro", info);
});

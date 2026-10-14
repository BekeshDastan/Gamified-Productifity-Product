import { revealControl } from "./disclosure";
import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
async function account(page: Page) {
  await page.clock.setFixedTime("2026-10-12T12:00:00Z");
  const email = "progression-" + crypto.randomUUID() + "@example.test",
    password = "progression browser password phrase";
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
async function task(page: Page, title: string, difficulty: number) {
  await page.goto("/app/quests");
  await page.getByRole("button", { name: "Создать задачу", exact: true }).click();
  await revealControl(page.getByLabel("Название задачи"));
  await page.getByLabel("Название задачи").fill(title);
  await revealControl(page.getByLabel("Сложность (1–5)"));
  await page.getByLabel("Сложность (1–5)").fill(String(difficulty));
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await page.getByRole("status").filter({ hasText: "Задача сохранена" }).getByRole("link").click();
  await expect(page).toHaveURL(/\/app\/quests\/[a-f0-9-]+$/);
  return page.url();
}
async function action(page: Page, name: string, status: string) {
  const card = page
    .locator("section")
    .filter({ has: page.getByRole("heading", { name: "Следующий шаг", exact: true }) });
  await card.getByRole("button", { name, exact: true }).click();
  await card.getByRole("button", { name: "Подтвердить действие", exact: true }).click();
  await expect(page.getByLabel("Статус", { exact: true })).toHaveValue(status);
}
async function closeNotices(page: Page) {
  for (let i = 0; i < 3; i++) {
    const button = page.getByRole("button", { name: "Закрыть уведомление", exact: true });
    if ((await button.count()) === 0) break;
    await button.first().click();
  }
}
test("progression UI shows actual rewards restorations level and partial quota", async ({
  page,
}, info) => {
  test.setTimeout(120000);
  await account(page);
  await page.goto("/app/progress");
  await expect(
    page
      .getByRole("group", { name: "Накопленный опыт", exact: true })
      .getByText("0", { exact: true }),
  ).toBeVisible();
  const first = await task(page, "Исходная награда", 3);
  await action(page, "Завершить", "completed");
  await expect(page.getByRole("region", { name: "Уведомления (F8)" })).toContainText("+25 XP");
  await closeNotices(page);
  await action(page, "Открыть снова", "todo");
  await expect(page.getByRole("region", { name: "Уведомления (F8)" })).toContainText("-25 XP");
  await closeNotices(page);
  await revealControl(page.getByLabel("Сложность (1–5)"));
  await page.getByLabel("Сложность (1–5)").fill("5");
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Сохранено." })).toBeVisible();
  await action(page, "Завершить", "completed");
  const notices = page.getByRole("region", { name: "Уведомления (F8)" });
  await expect(notices).toContainText("+25 XP");
  await expect(notices).not.toContainText("+100 XP");
  await page.goto("/app/progress");
  await expect(
    page
      .getByRole("group", { name: "Накопленный опыт", exact: true })
      .getByText("25", { exact: true }),
  ).toBeVisible();
  await expect(page.locator(".progression-history > li")).toHaveCount(3);
  await expect(page.locator(".progression-history")).toContainText("Корректировка");
  await page.getByText("Исходное право на награду", { exact: true }).first().click();
  await expect(page.locator(".progression-history")).toContainText(
    "Сложность при первом выполнении: 3",
  );
  await task(page, "Первый большой результат", 5);
  await action(page, "Завершить", "completed");
  await expect(page.getByRole("region", { name: "Уведомления (F8)" })).toContainText("Уровень 2");
  await page.goto("/app/progress");
  await expect(
    page
      .getByRole("group", { name: "Накопленный опыт", exact: true })
      .getByText("125", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("progressbar", { name: "Опыт до следующего уровня" }),
  ).toHaveAttribute("value", "25");
  await task(page, "Результат с частичной квотой", 5);
  await action(page, "Завершить", "completed");
  await expect(page.getByRole("region", { name: "Уведомления (F8)" })).toContainText(
    "+75 XP · +15 монет",
  );
  await page.goto("/app/progress");
  await expect(
    page
      .getByRole("group", { name: "Накопленный опыт", exact: true })
      .getByText("200", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("group", { name: "Кошелёк", exact: true }).getByText("40", { exact: true }),
  ).toBeVisible();
  await expect(page.locator(".progression-summary")).toContainText("задачи 200 / 200 XP");
  await page.reload();
  await expect(
    page
      .getByRole("group", { name: "Накопленный опыт", exact: true })
      .getByText("200", { exact: true }),
  ).toBeVisible();
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
      path: info.outputPath("progression-" + theme + "-mobile.png"),
      fullPage: true,
    });
  }
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(
    page.getByRole("progressbar", { name: "Опыт до следующего уровня" }),
  ).toHaveAttribute("value", "100");
  expect(first).toContain("/app/quests/");
});
test("progression account switch clears reward notices and private balances", async ({
  page,
  browser,
}) => {
  test.setTimeout(90000);
  await account(page);
  const other = await browser.newPage();
  const second = await account(other);
  await other.close();
  await task(page, "Личное выполнение первого аккаунта", 3);
  await action(page, "Завершить", "completed");
  await expect(page.getByRole("region", { name: "Уведомления (F8)" })).toContainText("+25 XP");
  await page.getByRole("link", { name: "Сегодня", exact: true }).first().click();

  await page.goto("/app/settings/security");
  await page.getByRole("button", { name: "Выйти", exact: true }).click();
  await expect(page.getByRole("link", { name: "Аккаунт и вход", exact: true })).toBeVisible();
  await page.goto("/app");
  await expect(page.getByText("Ваш личный аккаунт", { exact: true })).toBeVisible();
  await page.getByRole("link", { name: "Войти", exact: true }).click();
  await expect(page).toHaveURL(/\/auth\/login/);
  await expect(page.getByRole("region", { name: "Уведомления (F8)" })).not.toContainText("+25 XP");
  await page.getByLabel("Email", { exact: true }).fill(second.email);
  await page.getByLabel("Пароль", { exact: true }).fill(second.password);
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page).toHaveURL("/app");
  await page.getByText("Игровой прогресс", { exact: true }).click();
  await page.getByRole("link", { name: "Опыт и награды", exact: true }).click();
  await expect(
    page
      .getByRole("group", { name: "Накопленный опыт", exact: true })
      .getByText("0", { exact: true }),
  ).toBeVisible();
  await expect(page.locator(".progression-history > li")).toHaveCount(0);
  await expect(page.getByRole("region", { name: "Уведомления (F8)" })).not.toContainText("+25 XP");
});

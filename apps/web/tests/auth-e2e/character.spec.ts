import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
async function account(page: Page) {
  await page.clock.setFixedTime("2026-10-12T12:00:00Z");
  const email = "character-" + crypto.randomUUID() + "@example.test",
    password = "character browser password phrase";
  await page.goto("/auth/register");
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill(password);
  await page.getByLabel("Повторите пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Создать аккаунт", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Аккаунт создан" })).toBeVisible();
  await login(page, email, password);
  return { email, password };
}
async function login(page: Page, email: string, password: string) {
  await page.goto("/auth/login");
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page).toHaveURL("/app");
}
test("character achievement unlock equip title and repeated login are accessible", async ({
  page,
}, info) => {
  test.setTimeout(120000);
  const user = await account(page);
  await page.goto("/app/character");
  await expect(page.getByRole("img", { name: /^Образ:/ })).toBeVisible();
  await expect(page.locator(".character-avatar img")).toHaveCount(6);
  await page.getByText("Аксессуар · 1 открыто", { exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Надеть Аксессуар рассвета", exact: true }),
  ).toHaveCount(0);
  await page.goto("/app/settings/areas");
  await page.getByLabel("Название направления", { exact: true }).fill("Учёба");
  await page.getByRole("button", { name: "Создать направление", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Запись сохранена." })).toBeVisible();
  await page.goto("/app/quests");
  await page.getByRole("button", { name: "Создать задачу", exact: true }).click();
  await page.getByLabel("Название задачи").fill("Первый шаг к своему образу");
  await page.getByLabel("Сложность (1–5)").fill("5");
  await page.getByLabel("Направление", { exact: true }).selectOption({ label: "Учёба" });
  await page.getByLabel("Приоритет", { exact: true }).selectOption("high");
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await page.getByRole("status").filter({ hasText: "Задача сохранена" }).getByRole("link").click();
  await expect(page).toHaveURL(/\/app\/quests\/[a-f0-9-]+$/);
  const taskURL = page.url();
  const action = page
    .locator("section")
    .filter({ has: page.getByRole("heading", { name: "Следующий шаг", exact: true }) });
  await action.getByRole("button", { name: "Завершить", exact: true }).click();
  await action.getByRole("button", { name: "Подтвердить действие", exact: true }).click();
  await expect(page.getByLabel("Статус", { exact: true })).toHaveValue("completed");
  await page.goto("/app/achievements");
  const achievement = page
    .locator("section")
    .filter({ has: page.getByRole("heading", { name: "Первый шаг", exact: true }) });
  await expect(achievement.getByText("Получено", { exact: true })).toBeVisible();
  await expect(achievement.getByText("1 / 1", { exact: true })).toBeVisible();
  await page.goto("/app/character");
  await expect(page.getByRole("list", { name: "Условия навыков: Учёба" })).toContainText(
    "Движение · Открыт",
  );
  await expect(page.getByRole("list", { name: "Условия навыков: Учёба" })).toContainText(
    "Семь норм · Закрыт",
  );
  await page.getByText("Аксессуар · 2 открыто", { exact: true }).click();
  const equip = page.getByRole("button", { name: "Надеть Аксессуар рассвета", exact: true });
  await equip.focus();
  await expect(equip).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("status").filter({ hasText: "Образ сохранён" })).toBeVisible();
  await expect(page.locator('.character-avatar img[src$="accessory-1.svg"]')).toHaveCount(1);
  const title = page.getByRole("button", { name: "Выбрать титул Первый шаг", exact: true });
  await title.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByText("Сейчас: Первый шаг", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Первый шаг к своему образу", exact: true }),
  ).toHaveAttribute("href", new URL(taskURL).pathname);
  await page.goto("/app");
  await page.getByRole("button", { name: "Выйти", exact: true }).click();
  await expect(page.getByText("Ваш личный аккаунт", { exact: true })).toBeVisible();
  await login(page, user.email, user.password);
  await page.goto("/app/character");
  await expect(page.locator('.character-avatar img[src$="accessory-1.svg"]')).toHaveCount(1);
  await expect(page.getByText("Сейчас: Первый шаг", { exact: true })).toBeVisible();
  await page.emulateMedia({ reducedMotion: "reduce" });
  for (const theme of ["Светлая тема", "Тёмная тема"]) {
    await page
      .getByRole("combobox", { name: "Тема оформления", exact: true })
      .selectOption({ label: theme });
    await expect(page.getByRole("combobox", { name: "Тема оформления", exact: true })).toHaveValue(
      theme === "Тёмная тема" ? "dark" : "light",
    );
    await expect(page.locator("html")).toHaveClass(
      new RegExp(theme === "Тёмная тема" ? "dark" : "light"),
    );
    await page.setViewportSize({ width: 360, height: 800 });
    await expect
      .poll(
        () =>
          page.evaluate(() => ({
            width: innerWidth,
            scroll: document.documentElement.scrollWidth,
            elements: Array.from(document.querySelectorAll("body *"))
              .filter((e) => {
                const r = e.getBoundingClientRect();
                return r.width > 0 && r.height > 0 && r.right > innerWidth + 1;
              })
              .map((e) => ({
                tag: e.tagName,
                cls: e.getAttribute("class"),
                right: e.getBoundingClientRect().right,
                text: e.textContent?.slice(0, 80),
              }))
              .slice(-15),
          })),
        { message: "No horizontal overflow after responsive layout settles" },
      )
      .toEqual({ width: 360, scroll: 360, elements: [] });
    expect(
      (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21aa"]).analyze())
        .violations,
    ).toEqual([]);
    await page.screenshot({
      path: info.outputPath(
        theme === "Тёмная тема" ? "character-dark-mobile.png" : "character-light-mobile.png",
      ),
      fullPage: true,
    });
  }
  await page.goto("/app/achievements");
  expect(
    (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21aa"]).analyze())
      .violations,
  ).toEqual([]);
  await expect(
    page.getByRole("progressbar", { name: "Прогресс достижения Первый шаг" }),
  ).toHaveAttribute("value", "1");
});

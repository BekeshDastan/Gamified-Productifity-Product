import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

const password = "support workspace acceptance phrase";
async function signIn(page: Page, email: string) {
  await page.goto("/auth/login");
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page).toHaveURL("/app");
}
async function account(page: Page) {
  const email = `support-${crypto.randomUUID()}@example.test`;
  await page.goto("/auth/register");
  await page.getByLabel("Email", { exact: true }).fill(email);
  const field = page.getByLabel("Пароль", { exact: true });
  await field.fill(password);
  await page.getByRole("button", { name: "Показать: Пароль", exact: true }).press("Enter");
  await expect(field).toHaveAttribute("type", "text");
  await expect(field).toHaveValue(password);
  await page.getByRole("button", { name: "Скрыть: Пароль", exact: true }).press("Space");
  await expect(field).toHaveAttribute("type", "password");
  await expect(field).toHaveValue(password);
  await page.getByLabel("Повторите пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Создать аккаунт", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Аккаунт создан", exact: true })).toBeVisible();
  await signIn(page, email);
  return email;
}

test("support privacy choices persist and failed saves preserve the draft", async ({ page }) => {
  await account(page);
  await page.goto("/app/settings/privacy");
  const visibility = page.getByRole("group", {
    name: "Предпочтительная видимость профиля",
    exact: true,
  });
  await visibility.getByRole("button", { name: "Подтверждённые друзья", exact: true }).click();
  const activity = page.getByLabel("Разрешать делиться активностью с друзьями", { exact: true });
  const awards = page.getByLabel("Разрешать делиться достижениями с друзьями", { exact: true });
  const discovery = page.getByLabel("Разрешать находить мой профиль по нику", { exact: true });
  await activity.check();
  await awards.check();
  await discovery.check();
  await page.getByRole("button", { name: "Сохранить приватность", exact: true }).click();
  const saved = page.getByRole("status").filter({ hasText: "Приватность сохранена." });
  await expect(saved).toBeVisible();
  await activity.uncheck();
  await expect(saved).toHaveCount(0);
  await expect(
    page.getByRole("status").filter({ hasText: "Есть несохранённые изменения." }),
  ).toBeVisible();
  await page.route("**/api/v1/me/settings", async (route) => {
    if (route.request().method() === "PATCH") await route.abort("failed");
    else await route.continue();
  });
  await page.getByRole("button", { name: "Сохранить приватность", exact: true }).click();
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(activity).not.toBeChecked();
  await expect(awards).toBeChecked();
  await page.unroute("**/api/v1/me/settings");
  await page.getByRole("button", { name: "Сохранить приватность", exact: true }).click();
  await expect(saved).toBeVisible();
  await page.reload();
  await expect(
    visibility.getByRole("button", { name: "Подтверждённые друзья", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(activity).not.toBeChecked();
  await expect(awards).toBeChecked();
  await expect(discovery).toBeChecked();
  await expect(page.getByText("Сейчас всё доступно только вам", { exact: true })).toBeVisible();
});

test("support session cards revoke only the selected device and retain local access", async ({
  page,
  browser,
}, info) => {
  const email = await account(page);
  const second = await browser.newContext({ baseURL: "http://127.0.0.1:3100" });
  try {
    const device = await second.newPage();
    await signIn(device, email);
    await page.goto("/app/settings/security");
    await page.getByRole("button", { name: "Устройства и входы", exact: true }).click();
    await expect(page.locator(".session-list li")).toHaveCount(2);
    await expect(page.locator('.session-list li[data-current="true"]')).toContainText(
      "Это устройство",
    );
    await page.setViewportSize({ width: 360, height: 800 });
    expect(
      (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
    ).toEqual([]);
    await expect
      .poll(() => page.evaluate(() => document.documentElement.scrollWidth - innerWidth))
      .toBeLessThanOrEqual(0);
    await page.screenshot({ path: info.outputPath("security-mobile.png"), fullPage: true });
    await page.getByRole("button", { name: "Завершить вход", exact: true }).click();
    await expect(page.locator(".session-list li")).toHaveCount(1);
    await device.goto("/app/settings/security");
    await expect(
      device.getByRole("link", { name: "Войти для настройки безопасности", exact: true }),
    ).toBeVisible();
    await page.reload();
    await expect(page.getByRole("heading", { name: email, exact: true })).toBeVisible();
  } finally {
    await second.close();
  }
});

test("support guide routes motion and error recovery work on narrow screens", async ({
  page,
}, info) => {
  test.setTimeout(120000);
  await account(page);
  await page.goto("/app/guide");
  const paths = page.getByRole("group", { name: "Что хочется сделать сейчас?", exact: true });
  for (const theme of ["light", "dark"]) {
    await page.getByLabel("Тема оформления", { exact: true }).selectOption(theme);
    await expect(page.getByLabel("Тема оформления", { exact: true })).toBeEnabled();
    await expect(page.locator("html")).toHaveClass(new RegExp(theme));
    await page.setViewportSize({ width: 320, height: 740 });
    for (const name of ["Начать с малого", "Войти в фокус", "Завершить день"]) {
      await paths.getByRole("button", { name, exact: true }).click();
      await expect(page.locator(".guide-steps li")).toHaveCount(3);
      await expect
        .poll(() => page.evaluate(() => document.documentElement.scrollWidth - innerWidth))
        .toBeLessThanOrEqual(0);
    }
    expect(
      (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
    ).toEqual([]);
    await page.screenshot({ path: info.outputPath(`guide-${theme}-mobile.png`), fullPage: true });
  }
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await paths.getByRole("button", { name: "Войти в фокус", exact: true }).click();
  await expect(page.locator(".guide-steps li").first()).toHaveCSS(
    "animation-name",
    "guide-step-in",
  );
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(page.locator(".guide-steps li").first()).toHaveCSS("animation-name", "none");
  await page.getByRole("link", { name: "Перейти к таймеру", exact: true }).click();
  await expect(page).toHaveURL("/app/focus");
  await page.goto("/app/guide");
  await page.getByRole("link", { name: "Открыть мой день", exact: true }).click();
  await expect(page).toHaveURL("/app");
  await page.goto("/app/guide");
  await paths.getByRole("button", { name: "Завершить день", exact: true }).click();
  await page.getByRole("link", { name: "Открыть вечерний обзор", exact: true }).click();
  await expect(page).toHaveURL("/app/journal?section=evening");
  await page.route("**/api/v1/quests?**", (route) => route.abort("failed"));
  await page.goto("/app/quests");
  await expect(
    page.getByRole("heading", { name: "Не удалось загрузить данные", exact: true }),
  ).toBeVisible();
  await page.unroute("**/api/v1/quests?**");
  await page.getByRole("button", { name: "Повторить", exact: true }).click();
  await expect(page.getByRole("button", { name: "Создать задачу", exact: true })).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Не удалось загрузить данные", exact: true }),
  ).toHaveCount(0);
  await page.goto("/app/this-page-does-not-exist");
  await expect(
    page.getByRole("heading", { name: "Страница не найдена", exact: true }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Вернуться к своему дню", exact: true }).click();
  await expect(page).toHaveURL("/app");
});

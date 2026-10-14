import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
test("navigator explanations opt-out and private sources work without external network", async ({
  page,
}) => {
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
  await expect(page.getByRole("heading", { name: "R14 · Подсказка", exact: true })).toBeVisible();
  await page.getByText("Почему появилась эта подсказка", { exact: true }).click();
  await expect(page.getByText("Босс месяца не выбран", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Отключить R14", exact: true }).click();
  await expect(page.getByRole("heading", { name: "R14 · Подсказка", exact: true })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("heading", { name: "R14 · Подсказка", exact: true })).toHaveCount(0);
  await page.getByText("Настройки правил R01–R15", { exact: true }).click();
  await page.getByRole("button", { name: "Включить правило R14", exact: true }).click();
  await expect(page.getByRole("heading", { name: "R14 · Подсказка", exact: true })).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

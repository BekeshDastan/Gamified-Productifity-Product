import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

const password = "browser acceptance password phrase";
async function account(page: Page) {
  const email = `acceptance-${crypto.randomUUID()}@example.test`;
  await page.goto("/auth/register");
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill(password);
  await page.getByLabel("Повторите пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Создать аккаунт", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Аккаунт создан" })).toBeVisible();
  await page.getByRole("link", { name: "Перейти ко входу" }).click();
  await signIn(page, email);
  return email;
}
async function signIn(page: Page, email: string) {
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page).toHaveURL("/app");
  await expect(page.getByRole("heading", { name: email, exact: true })).toBeVisible();
}

test("two tabs share one refresh and logout clears the other tab immediately", async ({
  page,
  context,
}) => {
  const email = await account(page);
  const other = await context.newPage();
  await other.goto("/app");
  await expect(other.getByRole("heading", { name: email, exact: true })).toBeVisible();
  let refreshes = 0;
  context.on("request", (request) => {
    if (request.method() === "POST" && new URL(request.url()).pathname === "/api/v1/auth/refresh")
      refreshes++;
  });
  // Removing only access models its expiry; refresh remains a genuine cookie
  // obtained exclusively through the login form.
  await context.clearCookies({ name: "ascend_access" });
  await Promise.all([page.reload(), other.reload()]);
  await expect(page.getByRole("heading", { name: email, exact: true })).toBeVisible();
  await expect(other.getByRole("heading", { name: email, exact: true })).toBeVisible();
  expect(refreshes).toBe(1);
  await page.getByRole("button", { name: "Выйти", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Ваш личный аккаунт" })).toBeVisible();
  await expect(other.getByRole("heading", { name: "Ваш личный аккаунт" })).toBeVisible();
  await expect(other.getByRole("heading", { name: email, exact: true })).toHaveCount(0);
  await other.close();
});

test("lost refresh response requires login and never persists credentials", async ({
  page,
  context,
}) => {
  const consoleMessages: string[] = [];
  page.on("console", (message) => consoleMessages.push(message.text()));
  const email = await account(page);
  let rotated = false;
  await page.route("**/api/v1/auth/refresh", async (route) => {
    if (rotated) {
      await route.continue();
      return;
    }
    const headers = await route.request().allHeaders();
    // Perform the real server mutation without applying Set-Cookie to Chromium,
    // then drop its response. No auth response is fabricated.
    const response = await fetch("http://127.0.0.1:8080/api/v1/auth/refresh", {
      method: "POST",
      headers: {
        Origin: "http://127.0.0.1:3100",
        Cookie: headers.cookie ?? "",
        "X-CSRF-Token": headers["x-csrf-token"] ?? "",
        "Content-Type": "application/json",
      },
      body: "{}",
    });
    expect(response.status).toBe(200);
    await response.body?.cancel();
    rotated = true;
    await route.abort("failed");
  });
  await context.clearCookies({ name: "ascend_access" });
  await page.reload();
  await expect(page.getByRole("heading", { name: "Ваш личный аккаунт" })).toBeVisible();
  expect(rotated).toBe(true);
  await expect(page.getByText("Не удалось проверить вход.", { exact: false })).toBeVisible();
  await page.unroute("**/api/v1/auth/refresh");
  await page.goto("/auth/login");
  await signIn(page, email);
  const secrets = (await context.cookies())
    .filter((cookie) => /ascend_(access|refresh)/.test(cookie.name))
    .map((cookie) => cookie.value);
  for (const secret of [password, ...secrets])
    expect(consoleMessages.some((line) => line.includes(secret))).toBe(false);
  expect(
    await page.evaluate(() => Object.keys(localStorage).filter((key) => key !== "ascend-theme")),
  ).toEqual([]);
  expect(await page.evaluate(() => Object.keys(sessionStorage))).toEqual([]);
});

test("authentication forms fit a narrow screen and support both themes", async ({ page }, info) => {
  await page.setViewportSize({ width: 360, height: 800 });
  for (const theme of ["light", "dark"]) {
    await page.goto("/auth/register");
    await page.getByRole("combobox", { name: "Тема оформления" }).selectOption(theme);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    expect(
      (
        await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
          .analyze()
      ).violations,
    ).toEqual([]);
    await page.screenshot({ path: info.outputPath(`registration-${theme}.png`), fullPage: true });
  }
});

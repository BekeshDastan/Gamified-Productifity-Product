import { test, expect, type APIRequestContext, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { z } from "zod";

const messagesSchema = z.object({
  messages: z.array(
    z.object({
      ID: z.string(),
      Subject: z.string(),
      To: z.array(z.object({ Address: z.string() })),
    }),
  ),
});
const messageSchema = z.object({ Text: z.string() });
const password = "browser test password phrase";
async function mailLink(request: APIRequestContext, email: string, page: string) {
  const base = process.env.TEST_MAIL_HTTP;
  if (!base) throw new Error("TEST_MAIL_HTTP is required");
  let link = "";
  await expect
    .poll(
      async () => {
        const result = messagesSchema.parse(
          await (await request.get(base + "/api/v1/messages?limit=100")).json(),
        );
        for (const item of result.messages.filter((item) =>
          item.To.some((recipient) => recipient.Address === email),
        )) {
          const message = messageSchema.parse(
            await (await request.get(base + "/api/v1/message/" + item.ID)).json(),
          );
          const found = message.Text.match(
            new RegExp(`http://127\\.0\\.0\\.1:3100/auth/${page}#token=[A-Za-z0-9_-]{43}`),
          );
          if (found) {
            link = found[0];
            return true;
          }
        }
        return false;
      },
      { timeout: 40_000, intervals: [200, 500, 1000] },
    )
    .toBe(true);
  return link;
}
async function register(page: Page, email: string) {
  await page.goto("/auth/register");
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill(password);
  await page.getByLabel("Повторите пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Создать аккаунт", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Аккаунт создан" })).toBeVisible();
}
async function login(page: Page, email: string, value = password, next = "") {
  await page.goto("/auth/login" + next);
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill(value);
  await page.getByLabel("Запомнить меня на 30 дней").check();
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page).toHaveURL("/app");
  await expect(page.getByRole("heading", { name: email, exact: true })).toBeVisible();
}

test("real registration, SMTP verification, login, recovery and session invalidation", async ({
  page,
  context,
  request,
  browser,
}, info) => {
  test.setTimeout(120_000);
  const email = `browser-${crypto.randomUUID()}@example.test`;
  await page.setViewportSize({ width: 390, height: 900 });
  await register(page, email);
  const verification = await mailLink(request, email, "verify-email");
  await login(page, email, password, "?next=https://foreign.example/app");
  await expect(page.getByText("Почта ещё не подтверждена.", { exact: false })).toBeVisible();
  await page.goto(verification);
  await expect(page).toHaveURL(/\/auth\/verify-email$/);
  await page.getByRole("button", { name: "Подтвердить почту", exact: true }).click();
  await expect(page.getByRole("status")).toHaveText("Почта подтверждена. Можно продолжать!");
  await page.goto("/app");
  await expect(page.getByText("Почта подтверждена", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Устройства и входы" }).click();
  await expect(page.getByText("Это устройство", { exact: true })).toBeVisible();
  await page.screenshot({ path: info.outputPath("account-mobile.png"), fullPage: true });
  expect(
    (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21aa"]).analyze())
      .violations,
  ).toEqual([]);
  const device = await browser.newContext({ baseURL: "http://127.0.0.1:3100" });
  try {
    const other = await device.newPage();
    await login(other, email);
    await page.goto("/auth/forgot-password");
    await page.getByLabel("Email", { exact: true }).fill(email);
    await page.getByRole("button", { name: "Отправить ссылку" }).click();
    await expect(page.getByRole("status")).toContainText("Если аккаунт с таким адресом существует");
    const recovery = await mailLink(request, email, "reset-password");
    await page.goto(recovery);
    await expect(page).toHaveURL(/\/auth\/reset-password$/);
    await page.getByLabel("Новый пароль", { exact: true }).fill("new browser password phrase");
    await page.getByLabel("Повторите пароль", { exact: true }).fill("new browser password phrase");
    await page.getByRole("button", { name: "Сохранить пароль" }).click();
    await expect(page).toHaveURL("/auth/login?reset=1");
    await expect(page.getByRole("status")).toContainText("Пароль обновлён");
    await other.reload();
    await expect(other.getByRole("heading", { name: "Ваш личный аккаунт" })).toBeVisible();
    await page.getByLabel("Email", { exact: true }).fill(email);
    await page.getByLabel("Пароль", { exact: true }).fill(password);
    await page.getByRole("button", { name: "Войти", exact: true }).click();
    await expect(
      page.getByRole("alert").filter({ hasText: "Неверный email или пароль" }),
    ).toBeVisible();
    await page.getByLabel("Пароль", { exact: true }).fill("new browser password phrase");
    await page.getByLabel("Запомнить меня на 30 дней").check();
    await page.getByRole("button", { name: "Войти", exact: true }).click();
    await expect(page).toHaveURL("/app");
    await expect(page.getByRole("heading", { name: email, exact: true })).toBeVisible();
  } finally {
    await device.close();
  }
  const cookies = await context.cookies();
  for (const cookie of cookies.filter((cookie) =>
    ["ascend_access", "ascend_refresh"].includes(cookie.name),
  )) {
    expect(cookie.httpOnly).toBe(true);
    expect(cookie.sameSite).toBe("Lax");
    expect(cookie.expires).toBeGreaterThan(Date.now() / 1000);
  }
  expect(
    await page.evaluate(() => Object.keys(localStorage).filter((key) => key !== "ascend-theme")),
  ).toEqual([]);
  expect(await page.evaluate(() => Object.keys(sessionStorage))).toEqual([]);
});

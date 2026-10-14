import { revealControl } from "./disclosure";
import { test, expect, type APIRequestContext } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
async function mail(request: APIRequestContext, email: string, needle: string) {
  let body = "";
  await expect
    .poll(
      async () => {
        const messages = (
          await (
            await request.get(process.env.TEST_MAIL_HTTP + "/api/v1/messages?limit=100")
          ).json()
        ).messages as { ID: string; To: { Address: string }[] }[];
        for (const item of messages.filter((m) => m.To.some((t) => t.Address === email))) {
          const message = await (
            await request.get(process.env.TEST_MAIL_HTTP + "/api/v1/message/" + item.ID)
          ).json();
          if (typeof message.Text === "string" && message.Text.includes(needle)) {
            body = message.Text;
            return true;
          }
        }
        return false;
      },
      { timeout: 20000 },
    )
    .toBe(true);
  return body;
}
test("notifications consent inbox snooze unsubscribe and denied push keep personal history", async ({
  page,
  request,
}, info) => {
  test.setTimeout(180000);
  await page.route("**/*", (r) =>
    ["localhost", "127.0.0.1"].includes(new URL(r.request().url()).hostname)
      ? r.continue()
      : r.abort(),
  );
  const email = "notifications-" + crypto.randomUUID() + "@example.test",
    password = "notifications browser password phrase";
  await page.goto("/auth/register");
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill(password);
  await page.getByLabel("Повторите пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Создать аккаунт", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Аккаунт создан" })).toBeVisible();
  const verification = await mail(request, email, "/auth/verify-email#token=");
  const verifyLink = verification.match(
    /http:\/\/127\.0\.0\.1:3100\/auth\/verify-email#token=[A-Za-z0-9_-]+/,
  )?.[0];
  expect(verifyLink).toBeTruthy();
  await page.goto(verifyLink!);
  await page.getByRole("button", { name: "Подтвердить почту", exact: true }).click();
  await expect(page.getByRole("status")).toHaveText("Почта подтверждена. Можно продолжать!");
  await page.goto("/auth/login");
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page).toHaveURL("/app");
  await page.goto("/app/settings/notifications");
  const deadlines = page.getByRole("group", { name: "Сроки задач", exact: true });
  await expect(deadlines.getByLabel("Email", { exact: true })).not.toBeChecked();
  await expect(deadlines.getByLabel("Push", { exact: true })).not.toBeChecked();
  await deadlines.getByLabel("Email", { exact: true }).check();
  await page.getByLabel("Тихие часы", { exact: true }).uncheck();
  await page.getByRole("button", { name: "Сохранить каналы", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Настройки сохранены." })).toBeVisible();
  await page.evaluate(() => {
    Object.defineProperty(Notification, "requestPermission", {
      configurable: true,
      value: async () => "denied",
    });
  });
  await page.getByRole("button", { name: "Подключить push в этом браузере", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Разрешение не выдано" })).toBeVisible();
  expect((await (await page.request.get("/api/v1/notifications/push")).json()).data.items).toEqual(
    [],
  );
  const now = (await (await page.request.get("/api/v1/notifications")).json()).data.computed_at;
  await page.goto("/app/quests");
  await page.getByRole("button", { name: "Создать задачу", exact: true }).click();
  await revealControl(page.getByLabel("Название задачи", { exact: true }));
  await page
    .getByLabel("Название задачи", { exact: true })
    .fill("NOTIFICATION_PRIVATE_SENTINEL_C160");
  await revealControl(page.getByLabel("Дедлайн с часовым поясом", { exact: true }));
  await page
    .getByLabel("Дедлайн с часовым поясом", { exact: true })
    .fill(new Date(Date.parse(now) + 30 * 60000).toISOString());
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Задача сохранена" })).toBeVisible();
  await page.goto("/app/notifications");
  const item = page.getByRole("article", { name: "Напоминание о сроке задачи", exact: true });
  await expect(item).toBeVisible({ timeout: 20000 });
  await item.getByRole("button", { name: "Проверить источник", exact: true }).click();
  await expect(item.getByRole("link", { name: "Перейти к источнику", exact: true })).toBeVisible();
  await item.getByRole("button", { name: "Прочитано", exact: true }).click();
  await expect(item).toContainText("Прочитано");
  await item.getByText("История внешних отправок", { exact: true }).click();
  await expect(
    item.getByRole("heading", { name: "Письмо: Принято сервисом отправки", exact: true }),
  ).toBeVisible({ timeout: 20000 });
  const message = await mail(request, email, "/notifications/unsubscribe#token=");
  expect(message).not.toContain("NOTIFICATION_PRIVATE_SENTINEL_C160");
  const unsubscribe = message.match(
    /http:\/\/127\.0\.0\.1:3100\/notifications\/unsubscribe#token=[A-Za-z0-9_.-]+/,
  )?.[0];
  expect(unsubscribe).toBeTruthy();
  await expect(
    item.getByRole("button", { name: "Отложить напоминание", exact: true }),
  ).toBeDisabled();
  await item.getByLabel("Подтверждаю перенос напоминания", { exact: true }).check();
  await item.getByRole("button", { name: "Отложить напоминание", exact: true }).click();
  await expect(item).toContainText("Историческая запись");
  await item.getByRole("button", { name: "Проверить источник", exact: true }).click();
  await expect(item.getByRole("status")).toContainText("Источник больше не доступен");
  await page.goto(unsubscribe!);
  await expect(page).toHaveURL("/notifications/unsubscribe");
  await expect(
    page.getByRole("button", { name: "Отключить письма этого типа", exact: true }),
  ).toBeDisabled();
  await page.getByRole("checkbox").check();
  await page.getByRole("button", { name: "Отключить письма этого типа", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Письма этого типа отключены");
  await page.goto("/app/settings/notifications");
  await expect(
    page
      .getByRole("group", { name: "Сроки задач", exact: true })
      .getByLabel("Email", { exact: true }),
  ).not.toBeChecked();
  await page.setViewportSize({ width: 360, height: 800 });
  for (const theme of ["light", "dark"]) {
    await page.getByLabel("Тема оформления", { exact: true }).selectOption(theme);
    await expect(page.locator("html")).toHaveClass(new RegExp(`(?:^| )${theme}(?: |$)`));
    expect(
      (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
    ).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({ path: info.outputPath(`notifications-${theme}.png`), fullPage: true });
  }
  await page.goto("/app/notifications");
  await expect(
    page.getByRole("article", { name: "Напоминание о сроке задачи", exact: true }),
  ).toContainText("Историческая запись");
});

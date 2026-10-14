import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

test("UX first useful action, completion, mobile quick add and persistent welcome", async ({
  page,
}, info) => {
  test.setTimeout(120000);
  await page.clock.setFixedTime("2026-10-12T12:00:00Z");
  const email = `ux-${crypto.randomUUID()}@example.test`;
  await page.goto("/auth/register");
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill("a real ux acceptance password");
  await page.getByLabel("Повторите пароль", { exact: true }).fill("a real ux acceptance password");
  await page.getByRole("button", { name: "Создать аккаунт", exact: true }).click();
  await page.getByRole("link", { name: "Добавить первую задачу", exact: true }).click();
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill("a real ux acceptance password");
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page).toHaveURL("/app/onboarding");
  await expect(page.getByLabel("Имя", { exact: true })).toBeHidden();
  await page.getByLabel("Ваш первый шаг").fill("Прочитать десять страниц");
  await page.getByRole("button", { name: "Добавить на сегодня", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Добавлено на сегодня" })).toBeVisible();
  await page.getByRole("button", { name: "Перейти к моему дню", exact: true }).click();
  await expect(page).toHaveURL("/app");
  await expect(page.getByRole("heading", { name: "Сегодня", exact: true })).toBeVisible();
  const task = page.locator(".day-tasks li").filter({ hasText: "Прочитать десять страниц" });
  await task
    .getByRole("button", { name: "Завершить Прочитать десять страниц", exact: true })
    .click();
  await expect(task).toContainText("Выполнено");
  await page.reload();
  await expect(page.locator(".next-step")).toHaveCount(0);
  await expect(task).toContainText("Выполнено");
  await expect(page.getByRole("button", { name: "Устройства и входы" })).toHaveCount(0);
  const onboarding = await page.request.get("/api/v1/me/onboarding");
  expect((await onboarding.json()).data.completed_at).not.toBeNull();
  await page.screenshot({ path: info.outputPath("today-desktop.png"), fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await page
    .getByRole("navigation", { name: "Быстрая навигация" })
    .getByRole("button", { name: "Добавить задачу", exact: true })
    .click();
  const dialog = page.getByRole("dialog", { name: "Добавить задачу", exact: true });
  await dialog.getByLabel("Быстро добавить задачу").fill("Вечерняя прогулка");
  await dialog.getByRole("button", { name: "Добавить на сегодня", exact: true }).click();
  await expect(dialog.getByRole("status")).toContainText("Добавлено на сегодня");
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(
    page
      .getByRole("navigation", { name: "Быстрая навигация" })
      .getByRole("button", { name: "Добавить задачу", exact: true }),
  ).toBeFocused();
  await expect(
    page.locator(".day-tasks li").filter({ hasText: "Вечерняя прогулка" }),
  ).toBeVisible();
  for (const theme of ["light", "dark"]) {
    await page.getByLabel("Тема оформления", { exact: true }).selectOption(theme);
    await expect(page.getByLabel("Тема оформления", { exact: true })).toBeEnabled();
    await expect(page.locator("html")).toHaveClass(new RegExp(theme));
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
    await page.screenshot({ path: info.outputPath(`today-mobile-${theme}.png`), fullPage: true });
  }
});

test("UX quick input validates, preserves offline draft, prevents repeat saves and reveals invalid options", async ({
  page,
  context,
}, info) => {
  test.setTimeout(120000);
  await page.clock.setFixedTime("2026-10-12T12:00:00Z");
  const email = `ux-errors-${crypto.randomUUID()}@example.test`,
    password = "ux offline acceptance password";
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
  const add = page.getByRole("button", { name: "Добавить на сегодня", exact: true });
  await add.click();
  await expect(page.getByText("! Напишите, что хотите сделать", { exact: true })).toBeVisible();
  const input = page.getByLabel("Быстро добавить задачу");
  await input.fill("Длинная задача — " + "важный шаг ".repeat(14));
  await context.setOffline(true);
  await add.click();
  await expect(
    page.getByRole("alert").filter({ hasText: /сохранить|соединение|сеть|сервер/i }),
  ).toBeVisible();
  await expect(input).not.toHaveValue("");
  await context.setOffline(false);
  let writes = 0,
    release: (() => void) | undefined;
  await page.route("**/api/v1/quests", async (route) => {
    if (route.request().method() !== "POST") return route.continue();
    writes++;
    await new Promise<void>((resolve) => {
      release = resolve;
    });
    await route.continue();
  });
  await add.click();
  await expect(add).toBeDisabled();
  await expect.poll(() => writes).toBe(1);
  release?.();
  await expect(input).toHaveValue("");
  await page.unroute("**/api/v1/quests");
  await input.fill("Черновик на главной");
  const openAdd = page
    .locator(".app-sidebar")
    .getByRole("button", { name: "Добавить задачу", exact: true });
  const quickDialog = page.getByRole("dialog", { name: "Добавить задачу", exact: true });
  await openAdd.click();
  await page.keyboard.press("Escape");
  await expect(quickDialog).toHaveCount(0);
  await expect(input).toHaveValue("Черновик на главной");
  await openAdd.click();
  await quickDialog.getByLabel("Быстро добавить задачу").fill("Несохранённая задача");
  page.once("dialog", (dialog) => dialog.dismiss());
  await page.keyboard.press("Escape");
  await expect(quickDialog.getByLabel("Быстро добавить задачу")).toHaveValue(
    "Несохранённая задача",
  );
  page.once("dialog", (dialog) => dialog.accept());
  await quickDialog.getByRole("button", { name: "Закрыть", exact: true }).click();
  await expect(quickDialog).toHaveCount(0);
  await expect(input).toHaveValue("Черновик на главной");
  await input.fill("");
  await page.goto("/app/quests");
  await expect(page.getByLabel("Поиск задач")).toBeHidden();
  await expect(page.locator(".quest-list a")).toHaveCount(1);
  await page.getByRole("button", { name: "Создать задачу", exact: true }).click();
  await page.getByLabel("Название задачи", { exact: true }).fill("Срок без технической строки");
  await page.getByText("Дополнительные параметры", { exact: true }).click();
  await page.getByLabel("Крайний срок", { exact: true }).fill("2026-10-14T18:00");
  await page.getByLabel("Сложность (1–5)").fill("9");
  await page.getByText("Дополнительные параметры", { exact: true }).click();
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await expect(page.getByLabel("Сложность (1–5)")).toBeVisible();
  await expect(page.getByLabel("Сложность (1–5)")).toBeFocused();
  await page.getByLabel("Сложность (1–5)").fill("2");
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await page.getByRole("status").filter({ hasText: "Задача сохранена" }).getByRole("link").click();
  await expect(page.getByRole("button", { name: "Завершить", exact: true })).toBeVisible();
  await expect(page.getByLabel("Название задачи", { exact: true })).toBeHidden();
  await page.getByText("Изменить задачу", { exact: true }).click();
  await page.getByText("Дополнительные параметры", { exact: true }).click();
  await expect(page.getByLabel("Крайний срок", { exact: true })).toHaveValue("2026-10-14T18:00");
  await page.setViewportSize({ width: 360, height: 800 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath("task-mobile.png"), fullPage: true });
});

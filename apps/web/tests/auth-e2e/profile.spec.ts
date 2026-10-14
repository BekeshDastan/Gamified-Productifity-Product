import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

const password = "profile browser password phrase";
async function signIn(page: Page, email: string, value = password) {
  await page.goto("/auth/login");
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill(value);
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page).toHaveURL("/app");
  await expect(page.getByRole("link", { name: `Аккаунт: ${email}`, exact: true })).toBeVisible();
}
async function newAccount(page: Page) {
  const email = `profile-${crypto.randomUUID()}@example.test`;
  await page.goto("/auth/register");
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill(password);
  await page.getByLabel("Повторите пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Создать аккаунт", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Аккаунт создан" })).toBeVisible();
  await signIn(page, email);
  return email;
}
async function settings(page: Page) {
  const r = await page.request.get("/api/v1/me/settings");
  expect(r.ok()).toBe(true);
  return (await r.json()).data;
}

test("new account setup resumes, preferences survive another device and password change revokes it", async ({
  page,
  browser,
}, info) => {
  test.setTimeout(150_000);
  const email = await newAccount(page);
  await page.goto("/app/onboarding");
  await page.getByText("Подробная настройка — по желанию", { exact: true }).click();
  await expect(page.getByRole("heading", { name: "Как к вам обращаться" })).toBeVisible();
  await page.getByLabel("Имя", { exact: true }).fill("Дастан");
  await page.getByLabel("Ник", { exact: true }).fill(`user_${crypto.randomUUID().slice(0, 8)}`);
  await page.getByLabel("Росток", { exact: true }).check();
  await page.getByRole("button", { name: "Сохранить профиль", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Профиль сохранён." })).toBeVisible();
  await page.getByRole("button", { name: "Продолжить", exact: true }).click();
  await page.getByLabel("Часовой пояс", { exact: true }).fill("Asia/Almaty");
  await page.getByLabel("Язык дат и времени").selectOption("en");
  await page.getByLabel("Начало недели", { exact: true }).selectOption("7");
  await page.getByLabel("Формат времени").selectOption("12h");
  await page.getByRole("button", { name: "Добавить интервал", exact: true }).click();
  await page.getByRole("button", { name: "Сохранить расписание", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Расписание сохранено." })).toBeVisible();
  await page.getByRole("button", { name: "Продолжить", exact: true }).click();
  await page.getByRole("button", { name: "Программирование", exact: true }).click();
  await page.getByRole("button", { name: "Создать направление", exact: true }).click();
  await expect(page.locator(".label-list li").filter({ hasText: "Программирование" })).toHaveCount(
    1,
  );
  await page.getByRole("button", { name: "Продолжить", exact: true }).click();
  await page.getByLabel("Название черновика").fill("Пройти курс Go");
  await page.getByRole("button", { name: "Сохранить и продолжить", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Первая задача", exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByRole("heading", { name: "Первая задача", exact: true })).toBeVisible();
  await page.getByLabel("Название черновика").fill("Прочитать первую главу");
  await page.getByRole("button", { name: "Сохранить и продолжить", exact: true }).click();
  await page.getByRole("button", { name: "Пропустить шаг", exact: true }).click();
  await page.getByRole("button", { name: "Завершить настройку", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Настройка завершена", exact: true }),
  ).toBeVisible();
  // Reopening and skipping does not duplicate selected areas or saved drafts.
  await page.getByRole("button", { name: "Открыть настройку снова", exact: true }).click();
  for (let i = 0; i < 6; i++) {
    await page.getByRole("button", { name: "Пропустить шаг", exact: true }).click();
    await expect(
      page.getByRole("heading", {
        name: [
          "Ваше время",
          "Направления",
          "Первая цель",
          "Первая задача",
          "Первая привычка",
          "Всё готово",
        ][i],
        exact: true,
      }),
    ).toBeVisible();
  }
  const areas = await page.request.get("/api/v1/areas");
  expect((await areas.json()).data).toHaveLength(1);
  const setup = await page.request.get("/api/v1/me/onboarding");
  expect((await setup.json()).data.starters).toEqual({
    goal: "Пройти курс Go",
    task: "Прочитать первую главу",
    habit: "",
  });
  await page.goto("/app/settings/privacy");
  await page.getByLabel("Предпочтительная видимость профиля").selectOption("friends");
  await page.getByLabel("Разрешать делиться активностью с друзьями").check();
  await page.getByRole("button", { name: "Сохранить приватность", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "Приватность сохранена." }),
  ).toBeVisible();
  await page.goto("/app/settings/appearance");
  await page.getByLabel("Тема аккаунта").selectOption("dark");
  await page.getByLabel("Уменьшение движения", { exact: true }).selectOption("reduce");
  await page.getByLabel("Игровой режим", { exact: true }).selectOption("standard");
  await page.getByLabel("Разрешать звуки приложения").check();
  await page.getByRole("button", { name: "Поднять Фокус", exact: true }).click();
  await page.getByRole("button", { name: "Сохранить оформление", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Оформление сохранено." })).toBeVisible();
  await expect(page.locator("html")).toHaveClass(/dark/);
  await expect(page.locator("html")).toHaveAttribute("data-motion", "reduce");
  const second = await browser.newContext({ baseURL: "http://127.0.0.1:3100" });
  const device = await second.newPage();
  try {
    await signIn(device, email);
    await expect(device.locator("html")).toHaveClass(/dark/);
    const data = await settings(device);
    expect(data.timezone).toBe("Asia/Almaty");
    expect(data.week_start).toBe(7);
    expect(data.personalization).toMatchObject({
      theme: "dark",
      motion: "reduce",
      sound: true,
      game_mode: "standard",
      widget_order: ["today", "calendar", "focus", "habits", "progress"],
    });
    expect(data.privacy.profile_visibility).toBe("friends");
    await device.goto("/app/settings/profile");
    await expect(device.getByLabel("Имя", { exact: true })).toHaveValue("Дастан");
    await device.goto("/app/settings/schedule");
    await expect(device.getByLabel("Часовой пояс", { exact: true })).toHaveValue("Asia/Almaty");
    await expect(device.getByRole("time")).toHaveCount(1);
    await page.goto("/app/settings/security");
    await page.getByLabel("Текущий пароль", { exact: true }).fill("wrong password");
    await page.getByLabel("Новый пароль", { exact: true }).fill("replacement profile password");
    await page
      .getByLabel("Повторите новый пароль", { exact: true })
      .fill("replacement profile password");
    await page.getByRole("button", { name: "Изменить пароль и выйти", exact: true }).click();
    await expect(
      page.getByRole("alert").filter({ hasText: "Неверный текущий пароль" }),
    ).toBeVisible();
    await page.getByLabel("Текущий пароль", { exact: true }).fill(password);
    await page.getByRole("button", { name: "Изменить пароль и выйти", exact: true }).click();
    await expect(page).toHaveURL("/auth/login?reset=1");
    await device.reload();
    await expect(
      device.getByRole("link", { name: "Войти для настройки расписания" }),
    ).toBeVisible();
    await signIn(page, email, "replacement profile password");
    await page.goto("/app/settings/profile");
    await expect(page.getByLabel("Имя", { exact: true })).toHaveValue("Дастан");
    await page.screenshot({ path: info.outputPath("profile-desktop.png"), fullPage: true });
  } finally {
    await second.close();
  }
});

test("account settings remain accessible on a narrow screen in both themes", async ({
  page,
}, info) => {
  test.setTimeout(150_000);
  await newAccount(page);
  await page.setViewportSize({ width: 360, height: 900 });
  for (const theme of ["light", "dark"]) {
    await page.getByRole("combobox", { name: "Тема оформления", exact: true }).selectOption(theme);
    await expect(
      page.getByRole("combobox", { name: "Тема оформления", exact: true }),
    ).toBeEnabled();
    await expect(page.getByRole("combobox", { name: "Тема оформления", exact: true })).toHaveValue(
      theme,
    );
    expect((await settings(page)).personalization.theme).toBe(theme);
    await expect(page.locator("html")).toHaveClass(new RegExp(theme));
    for (const [path, name] of [
      ["profile", "Ваш профиль"],
      ["schedule", "Часовой пояс и расписание"],
      ["areas", "Направления и теги"],
      ["privacy", "Приватность"],
      ["appearance", "Ваше оформление"],
      ["security", "Устройства и безопасность"],
    ]) {
      await page.goto(`/app/settings/${path}`);
      await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
      await expect(page.locator("html")).toHaveClass(new RegExp(theme));
      await expect(page.getByRole("status").filter({ hasText: /Загружаем|Проверяем/ })).toHaveCount(
        0,
      );
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
      if (path === "schedule" || path === "profile")
        await page.screenshot({
          path: info.outputPath(`${path}-${theme}-mobile.png`),
          fullPage: true,
        });
    }
  }
});

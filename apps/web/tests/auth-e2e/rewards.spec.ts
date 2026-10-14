import { expand, revealControl } from "./disclosure";
import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
async function account(page: Page) {
  await page.clock.setFixedTime("2026-10-12T12:00:00Z");
  const email = "rewards-" + crypto.randomUUID() + "@example.test",
    password = "rewards browser password phrase";
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
async function goal(page: Page, title: string) {
  await page.goto("/app/goals");
  await page.getByRole("button", { name: "Создать цель", exact: true }).click();
  await page.getByLabel("Название цели", { exact: true }).fill(title);
  await page.getByLabel("Критерий успеха", { exact: true }).fill("Проверить результат");
  await page.getByLabel("Способ расчёта прогресса").selectOption("metric");
  await page.getByLabel("Целевое значение").fill("1");
  await page.getByLabel("Единица результата").fill("результат");
  await page.getByLabel("Начальное состояние цели").selectOption("active");
  await page.getByRole("button", { name: "Сохранить цель", exact: true }).click();
  await page.getByRole("status").filter({ hasText: "Цель сохранена" }).getByRole("link").click();
  await expect(page).toHaveURL(/\/app\/goals\/[a-f0-9-]+$/);
  return page.url();
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
test("shop insufficient balance cancel refund and claim have real durable states", async ({
  page,
}, info) => {
  test.setTimeout(120000);
  await account(page);
  await page.goto("/app/rewards");
  await page.getByRole("button", { name: "Создать личную награду", exact: true }).click();
  await page.getByLabel("Название награды").fill("Вечер отдыха");
  await page.getByLabel("Стоимость в монетах").fill("5");
  await page.getByRole("button", { name: "Сохранить личную награду", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("button", { name: "Купить Вечер отдыха", exact: true }).click();
  await expect(
    page.getByText("Недостаточно монет для этой покупки.", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Подтвердить покупку", exact: true }),
  ).toBeDisabled();
  await page.getByRole("dialog").getByRole("button", { name: "Закрыть", exact: true }).click();
  await page.goto("/app/quests");
  await page.getByRole("button", { name: "Создать задачу", exact: true }).click();
  await revealControl(page.getByLabel("Название задачи"));
  await page.getByLabel("Название задачи").fill("Заработать награду");
  await revealControl(page.getByLabel("Сложность (1–5)"));
  await page.getByLabel("Сложность (1–5)").fill("3");
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await page.getByRole("status").filter({ hasText: "Задача сохранена" }).getByRole("link").click();
  const action = page
    .locator("section")
    .filter({ has: page.getByRole("heading", { name: "Следующий шаг", exact: true }) });
  await action.getByRole("button", { name: "Завершить", exact: true }).click();
  await action.getByRole("button", { name: "Подтвердить действие", exact: true }).click();
  await expect(page.getByLabel("Статус", { exact: true })).toHaveValue("completed");
  await page.goto("/app/rewards");
  await page.getByRole("button", { name: "Купить Вечер отдыха", exact: true }).click();
  await page.getByRole("button", { name: "Подтвердить покупку", exact: true }).click();
  await expect(page.locator(".reward-history")).toContainText("Ожидает получения");
  await page.getByRole("button", { name: "Отменить покупку", exact: true }).click();
  await page.getByRole("button", { name: "Подтвердить решение", exact: true }).click();
  await expect(page.locator(".reward-history")).toContainText("Отменено, монеты возвращены");
  await page.getByRole("button", { name: "Купить Вечер отдыха", exact: true }).click();
  await page.getByRole("button", { name: "Подтвердить покупку", exact: true }).click();
  await page.getByRole("button", { name: "Подтвердить получение", exact: true }).click();
  await page.getByRole("button", { name: "Подтвердить решение", exact: true }).click();
  await expect(page.locator(".reward-history")).toContainText("Получено");
  await expect(page.getByRole("button", { name: "Отменить покупку", exact: true })).toHaveCount(0);
  await page.reload();
  await expect(page.locator(".reward-history > li")).toHaveCount(2);
  await accessible(page, "shop", info);
});
test("boss victory correction monthly dashboard and free surprise persist", async ({
  page,
}, info) => {
  test.setTimeout(120000);
  await account(page);
  const source = await goal(page, "Главный результат");
  await goal(page, "Второе намерение");
  await goal(page, "Третье намерение");
  await page.goto("/app/rewards");
  const gift = page.getByRole("button", { name: "Открыть бесплатный подарок", exact: true });
  await gift.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("status").filter({ hasText: "Получен предмет:" })).toBeVisible();
  const result = await page.getByRole("status").filter({ hasText: "Получен предмет:" }).innerText();
  await expect(gift).toHaveCount(0);
  await page.reload();
  await expect(
    page.getByText(result.replace("Получен предмет:", "Получен:").replace(/\.$/, ""), {
      exact: false,
    }),
  ).toBeVisible();
  await page.goto("/app/bosses");
  await page.getByLabel("Цель босса", { exact: true }).selectOption({ label: "Главный результат" });
  await page.getByRole("button", { name: "Создать босса цели", exact: true }).click();
  const hp = page.getByRole("progressbar", { name: "HP босса Главный результат", exact: true });
  await expect(hp).toHaveAttribute("value", "100");
  await page
    .getByLabel("Главный босс месяца", { exact: true })
    .selectOption({ label: "Главный результат" });
  await page.getByLabel("Причина выбора босса месяца").fill("Фокус месяца");
  await page.getByRole("button", { name: "Сохранить босса месяца", exact: true }).click();
  await expect(page.getByText(/Сейчас: Главный результат/)).toBeVisible();
  await page.goto(source);
  await page.getByLabel("Добавить результат", { exact: true }).fill("1");
  await page.getByLabel("Дата результата", { exact: true }).fill("2026-10-12");
  await page.getByRole("button", { name: "Записать результат", exact: true }).click();
  await expect(
    page.getByRole("progressbar", { name: "Прогресс цели", exact: true }),
  ).toHaveAttribute("value", "100");
  await page.goto("/app/bosses");
  await expect(hp).toHaveAttribute("value", "0");
  await expect(page.getByText("Ожидает подтверждения цели", { exact: true })).toBeVisible();
  await page.goto(source);
  await page.getByLabel("Действие с целью").selectOption("achieve");
  await page.getByLabel(/Подтверждаю: критерий/).check();
  await page.getByLabel("Причина решения").fill("Проверил результат");
  await page.getByRole("button", { name: "Сохранить решение", exact: true }).click();
  await expect(
    page
      .locator("section")
      .filter({ has: page.getByRole("heading", { name: "Главный результат", exact: true }) }),
  ).toContainText("Достигнута");
  await page.goto("/app/bosses");
  await expect(page.getByText("Победа подтверждена", { exact: true })).toBeVisible();
  await accessible(page, "boss", info);
  await page.goto("/app");
  await expand(page, "Босс месяца и подробности дня");
  await expect(
    page.getByRole("progressbar", { name: "HP босса Главный результат", exact: true }),
  ).toHaveAttribute("value", "0");
  await page.goto(source);
  await page.getByText("Исправить результат", { exact: true }).click();
  await page.getByLabel("Исправленное значение").fill("0");
  await page.getByLabel("Причина исправления результата").fill("Результат пока не подтверждён");
  await page.getByRole("button", { name: "Сохранить исправление", exact: true }).click();
  await expect(
    page.getByRole("progressbar", { name: "Прогресс цели", exact: true }),
  ).toHaveAttribute("value", "0");
  await page.goto("/app/bosses");
  await expect(hp).toHaveAttribute("value", "100");
  await expect(page.getByText("Победа подтверждена", { exact: true })).toHaveCount(0);
});

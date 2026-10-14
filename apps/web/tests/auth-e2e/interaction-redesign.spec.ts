import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { expand, revealControl } from "./disclosure";

async function account(page: Page) {
  const email = `interaction-${crypto.randomUUID()}@example.test`;
  const password = "interactive redesign browser test phrase";
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

test("interactive workflows persist plan choices and formatted journal without losing existing text", async ({
  page,
}, info) => {
  test.setTimeout(150000);
  await account(page);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.getByLabel("Тема оформления").selectOption("light");
  await expect(page.locator("html")).toHaveClass(/light/);
  const title = "Подготовить идею проекта";
  await page.getByLabel("Быстро добавить задачу").fill(title);
  await page.getByRole("button", { name: "Добавить на сегодня", exact: true }).click();
  await expect(page.getByRole("link", { name: title, exact: true })).toBeVisible();
  await page.goto("/app/planning?date=2026-10-15");
  await page
    .getByRole("group", { name: "Быстрый выбор доступного времени" })
    .getByRole("button", { name: "2 ч", exact: true })
    .click();
  await expect(page.getByLabel("Доступно минут", { exact: true })).toHaveValue("120");
  await page.getByRole("slider", { name: "Запас времени на день" }).press("ArrowRight");
  await expect(page.getByLabel("Доступно минут", { exact: true })).toHaveValue("135");
  const picker = page.getByRole("group", { name: "Быстрый выбор: Задача в план", exact: true });
  await picker.getByRole("button", { name: new RegExp(title) }).click();
  await expect(picker.getByRole("button", { name: new RegExp(title) })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await page.getByRole("button", { name: "Добавить выбранную задачу", exact: true }).click();
  await page.getByLabel("Главная задача", { exact: true }).check();
  await page.getByRole("button", { name: "Добавить рабочее окно", exact: true }).click();
  await page.getByLabel("Начало окна 1", { exact: true }).fill("10:00");
  await page.getByLabel("Конец окна 1", { exact: true }).fill("12:15");
  await page.getByRole("heading", { name: "Черновик плана", exact: true }).click();
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: info.outputPath("planning-desktop.png"), fullPage: true });
  await page.getByRole("button", { name: "Сохранить черновик", exact: true }).click();
  await expect(page.getByRole("button", { name: "Изменить план", exact: true })).toBeVisible();
  await page.reload();
  await page.getByRole("button", { name: "Изменить план", exact: true }).click();
  await expect(page.getByLabel("Доступно минут", { exact: true })).toHaveValue("135");
  await expect(page.getByLabel("Главная задача", { exact: true })).toBeChecked();
  await expect(page.getByLabel("Начало окна 1", { exact: true })).toHaveValue("10:00");
  await expect(page.getByLabel("Конец окна 1", { exact: true })).toHaveValue("12:15");

  await page.goto("/app/journal");
  await page.getByRole("button", { name: "Маленькая победа", exact: true }).click();
  await expect(page.getByLabel("Заголовок записи", { exact: true })).toHaveValue(
    "Маленькая победа",
  );
  const body = page.getByLabel("Текст записи", { exact: true });
  await body.fill("Сегодня я закончил важное дело.");
  await page.getByRole("button", { name: "Благодарность", exact: true }).click();
  await expect(body).toHaveValue(
    "Сегодня я закончил важное дело.\n\n## За что я сегодня благодарен?\n",
  );
  await expect(page.getByLabel("Заголовок записи", { exact: true })).toHaveValue(
    "Маленькая победа",
  );
  await body.press("ControlOrMeta+End");
  await page.getByRole("button", { name: "Жирный текст", exact: true }).click();
  await body.pressSequentially("За поддержку");
  await expect(body).toHaveValue(/\*\*За поддержку\*\*$/);
  await page.getByRole("button", { name: "Сохранить запись", exact: true }).click();
  await expect(
    page.getByText("Запись сохранена. Её видите только вы.", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("article", { name: "Предпросмотр сохранённой записи" }).locator("strong"),
  ).toHaveText("За поддержку");
  await page.reload();
  await page
    .getByRole("button", { name: "Открыть запись «Маленькая победа»", exact: true })
    .click();
  await expect(body).toHaveValue(/Сегодня я закончил важное дело\.[\s\S]*\*\*За поддержку\*\*$/);
  await page.getByRole("heading", { name: "Дневник и ритуалы", exact: true }).click();
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: info.outputPath("journal-desktop.png"), fullPage: true });

  await page.goto("/app/analytics");
  const metrics = page.getByRole("group", { name: "Быстрый выбор показателя", exact: true });
  await metrics.getByRole("button", { name: "Опыт", exact: true }).click();
  await expect(page.getByRole("group", { name: "График: Опыт", exact: true })).toBeVisible();
  await expect(
    page.getByRole("region", { name: "Показатели по дням", exact: true }),
  ).not.toBeVisible();
  await expand(page, "Таблица-альтернатива графика");
  await expect(page.getByRole("region", { name: "Показатели по дням", exact: true })).toBeVisible();
  await page.goto("/app/focus");
  await expand(page, "История фокуса");
  await page
    .getByRole("group", { name: "Период истории фокуса" })
    .getByRole("button", { name: "7 дней", exact: true })
    .click();
  await revealControl(page.getByLabel("Фокус с даты", { exact: true }));
  await page.getByLabel("Фокус с даты", { exact: true }).fill("2026-09-20");
  await expect(
    page
      .getByRole("group", { name: "Период истории фокуса" })
      .getByRole("button", { pressed: true }),
  ).toHaveCount(0);
});

test("interactive sections remain accessible and fit desktop and mobile in both themes", async ({
  page,
}, info) => {
  test.setTimeout(360000);
  await account(page);
  await page.emulateMedia({ reducedMotion: "reduce" });
  for (const theme of ["light", "dark"]) {
    await page.getByLabel("Тема оформления").selectOption(theme);
    await expect(page.locator("html")).toHaveClass(new RegExp(theme));
    await expect(page.getByLabel("Тема оформления")).toBeEnabled();
    for (const route of [
      "planning",
      "journal",
      "focus",
      "analytics",
      "navigator",
      "progress",
      "achievements",
      "challenges",
      "character",
      "rewards",
      "bosses",
      "seasons",
    ]) {
      await page.goto("/app/" + route);
      await page.waitForLoadState("networkidle");
      await expect(page.locator("html")).toHaveClass(new RegExp(theme));
      if (route === "focus") await expand(page, "История фокуса");
      if (theme === "light") {
        await page.setViewportSize({ width: 1440, height: 1000 });
        await expect
          .poll(async () => (await page.locator("main").boundingBox())?.x ?? 0, {
            message: `${route}: desktop content clears the sidebar after resizing`,
          })
          .toBeGreaterThanOrEqual(244);
        await expect
          .poll(() => page.evaluate(() => document.documentElement.scrollWidth - innerWidth))
          .toBeLessThanOrEqual(0);
        await page.screenshot({ path: info.outputPath(`${route}-desktop.png`), fullPage: true });
      }
      // Check expanded interactive controls as well as the initial state.
      if (route === "journal") await expand(page, "Теги, цель и параметры записи");
      if (route === "focus") await revealControl(page.getByLabel("Фокус с даты", { exact: true }));
      if (route === "analytics")
        await revealControl(page.getByLabel("Начало аналитики", { exact: true }));
      await page.setViewportSize({ width: 360, height: 800 });
      await expect
        .poll(() => page.evaluate(() => document.documentElement.scrollWidth - innerWidth), {
          message: `${route} ${theme}`,
        })
        .toBeLessThanOrEqual(0);
      await expect(page.locator("main h1")).toBeVisible();
      const violations = (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze())
        .violations;
      expect(violations, `${route} ${theme}`).toEqual([]);
      await page.screenshot({
        path: info.outputPath(`${route}-mobile-${theme}.png`),
        fullPage: true,
      });
    }
  }
});

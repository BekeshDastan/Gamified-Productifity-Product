import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { expand, revealControl } from "./disclosure";

async function account(page: Page) {
  await page.clock.setFixedTime("2026-10-12T12:00:00Z");
  const email = `workspaces-${crypto.randomUUID()}@example.test`;
  const password = "workspace browser verification phrase";
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
const choice = (page: Page, group: string, name: string) =>
  page.getByRole("group", { name: group, exact: true }).getByRole("button", { name, exact: true });

test("workspace shortcuts save real tasks habits goals and calendar dates", async ({
  page,
}, info) => {
  test.setTimeout(150000);
  await account(page);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/app/quests");
  await page.getByRole("button", { name: "Создать задачу", exact: true }).click();
  await page.getByLabel("Название задачи", { exact: true }).fill("Подготовить первую главу");
  await choice(page, "Когда заняться задачей", "Завтра").click();
  await choice(page, "Сколько времени займёт задача", "25 мин").click();
  await choice(page, "Быстрый выбор приоритета", "Важное").click();
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await page.getByRole("status").filter({ hasText: "Задача сохранена" }).getByRole("link").click();
  await expect(page).toHaveURL(/\/app\/quests\/[a-f0-9-]+$/);
  await page.reload();
  await expand(page, "Изменить задачу");
  await expect(page.getByLabel("Плановая дата", { exact: true })).toHaveValue("2026-10-13");
  await expect(page.getByLabel("Оценка, минут", { exact: true })).toHaveValue("25");
  await expect(choice(page, "Быстрый выбор приоритета", "Важное")).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await page.goto("/app/quests");
  await choice(page, "Быстрый фильтр задач", "Выполнены").click();
  await expect(
    page.getByRole("heading", { name: "По этим условиям задач нет", exact: true }),
  ).toBeVisible();
  await choice(page, "Быстрый фильтр задач", "К выполнению").click();
  await expect(
    page
      .locator(".quest-list")
      .getByRole("link", { name: "Подготовить первую главу", exact: true }),
  ).toBeVisible();
  await expand(page, "Поиск и фильтры");
  await expect(page.getByLabel("Фильтр по статусу", { exact: true })).toHaveValue("todo");
  await page.getByRole("button", { name: "Доска", exact: true }).click();
  await expect(
    page
      .getByLabel("Доска задач")
      .getByRole("link", { name: "Подготовить первую главу", exact: true }),
  ).toBeVisible();

  await page.goto("/app/habits");
  await page.getByRole("button", { name: "Создать привычку", exact: true }).click();
  await page.getByLabel("Название привычки", { exact: true }).fill("Практика внимания");
  await choice(page, "Тип привычки", "Длительность").press("Enter");
  await expect(page.getByLabel("Единица", { exact: true })).toHaveValue("мин");
  await page.getByLabel("Минимальный шаг", { exact: true }).fill("2");
  await page.getByLabel("Норма периода", { exact: true }).fill("10");
  await page.getByRole("button", { name: "Добавить привычку", exact: true }).click();
  const habit = page
    .locator(".habit-card")
    .filter({ has: page.getByRole("heading", { name: "Практика внимания", exact: true }) });
  await habit.getByRole("button", { name: "10 мин", exact: true }).click();
  // Choosing a preset must not silently log activity.
  await expect(habit.getByRole("progressbar")).toHaveAttribute("value", "0");
  await habit.getByRole("button", { name: "Добавить отметку", exact: true }).click();
  await expect(habit.getByRole("progressbar")).toHaveAttribute("value", "100");
  await page.reload();
  await expect(habit.getByRole("progressbar")).toHaveAttribute("value", "100");
  await page.screenshot({ path: info.outputPath("habit-completed.png"), fullPage: true });

  await page.goto("/app/goals");
  await page.getByRole("button", { name: "Создать цель", exact: true }).click();
  await page.getByLabel("Название цели", { exact: true }).fill("Прочитать книгу с заметками");
  await page.getByLabel("Критерий успеха", { exact: true }).fill("Десять разобранных глав");
  await choice(page, "Способ расчёта прогресса", "Числовой результат").click();
  await page.getByLabel("Целевое значение", { exact: true }).fill("10");
  await page.getByLabel("Единица результата", { exact: true }).fill("глав");
  // Clicking the selected mode must not erase the user's metric values.
  await choice(page, "Способ расчёта прогресса", "Числовой результат").click();
  await expect(page.getByLabel("Единица результата", { exact: true })).toHaveValue("глав");
  await page.getByLabel("Начальное состояние цели").selectOption("active");
  await page.getByRole("button", { name: "Сохранить цель", exact: true }).click();
  await page.getByRole("status").filter({ hasText: "Цель сохранена" }).getByRole("link").click();
  await expect(page).toHaveURL(/\/app\/goals\/[a-f0-9-]+$/);
  await page.getByLabel("Добавить результат", { exact: true }).fill("2");
  await page.getByLabel("Дата результата", { exact: true }).fill("2026-10-12");
  await page.getByRole("button", { name: "Записать результат", exact: true }).click();
  await expect(page.getByRole("progressbar", { name: "Прогресс цели" })).toHaveAttribute(
    "value",
    "20",
  );
  await page.reload();
  await expect(page.getByRole("img", { name: "Достижение цели: 20%", exact: true })).toBeVisible();
  await page.screenshot({ path: info.outputPath("goal-progress.png"), fullPage: true });

  await page.goto("/app/calendar");
  await page.getByRole("button", { name: "Добавить событие", exact: true }).click();
  await page.getByLabel("Название события", { exact: true }).fill("Два дня для проекта");
  await expect(choice(page, "Длительность события", "Два дня")).toBeDisabled();
  await page.getByLabel("Начало события", { exact: true }).fill("2026-10-12");
  await choice(page, "Длительность события", "Два дня").click();
  await expect(page.getByLabel("Конец события", { exact: true })).toHaveValue("2026-10-14");
  await page.getByRole("button", { name: "Сохранить событие", exact: true }).click();
  await expect(page.getByLabel("Название события", { exact: true })).toHaveCount(0);
  await page.reload();
  await page.getByText(/События диапазона — доступный список/).click();
  await page.getByRole("button", { name: "2026-10-12 · Два дня для проекта", exact: true }).click();
  await expect(page.getByLabel("Конец события", { exact: true })).toHaveValue("2026-10-14");
  await page.goto("/app/calendar");
  await page.getByRole("button", { name: "Добавить событие", exact: true }).click();
  await page.getByLabel("Название события", { exact: true }).fill("Час на глубокую работу");
  await page.getByLabel("Без времени", { exact: true }).uncheck();
  await page.getByLabel("Начало события", { exact: true }).fill("2026-10-12T14:30");
  await choice(page, "Длительность события", "60 мин").click();
  await expect(page.getByLabel("Конец события", { exact: true })).toHaveValue("2026-10-12T15:30");
  await page.getByRole("button", { name: "Сохранить событие", exact: true }).click();
  await expect(page.getByLabel("Название события", { exact: true })).toHaveCount(0);
  await page.reload();
  await page.getByText(/События диапазона — доступный список/).click();
  await page
    .getByRole("button", { name: "2026-10-12 · Час на глубокую работу", exact: true })
    .click();
  await expect(page.getByLabel("Конец события", { exact: true })).toHaveValue("2026-10-12T15:30");
});

test("workspace settings previews persist and reduced motion controls actual animations", async ({
  page,
}, info) => {
  test.setTimeout(150000);
  await account(page);
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await page.goto("/app/settings/profile");
  await page.getByLabel("Имя", { exact: true }).fill("Путешественник");
  await page.getByLabel("Компас", { exact: true }).check();
  await expect(page.getByLabel("Предпросмотр профиля")).toContainText("Путешественник");
  await page.getByRole("button", { name: "Сохранить профиль", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Профиль сохранён." })).toBeVisible();
  await page.reload();
  await expect(page.getByLabel("Компас", { exact: true })).toBeChecked();
  await expect(page.getByLabel("Имя", { exact: true })).toHaveValue("Путешественник");

  await page.goto("/app/settings/schedule");
  await page.getByRole("button", { name: "Рабочий день 9–18", exact: true }).click();
  await expect(page.locator(".schedule-row")).toHaveCount(5);
  await expect(page.getByLabel("Предпросмотр рабочей недели")).toContainText("09:00–18:00");
  await page.getByRole("button", { name: "Удалить интервал 5", exact: true }).click();
  await page.getByRole("button", { name: "Сохранить расписание", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Расписание сохранено." })).toBeVisible();
  await page.reload();
  await expect(page.locator(".schedule-row")).toHaveCount(4);
  await expect(page.getByLabel("Начало 1", { exact: true })).toHaveValue("09:00");

  await page.goto("/app/settings/areas");
  const areas = page
    .locator("section")
    .filter({ has: page.getByRole("heading", { name: "Направления жизни", exact: true }) });
  await areas.getByLabel("Название направления", { exact: true }).fill("Мой проект");
  await areas.getByRole("button", { name: "Цвет #d76b4c", exact: true }).click();
  await areas
    .getByRole("group", { name: "Значок", exact: true })
    .getByRole("button", { name: "Проект", exact: true })
    .click();
  await areas.getByRole("button", { name: "Создать направление", exact: true }).click();
  await expect(areas.locator(".label-list")).toContainText("Мой проект");
  await page.reload();
  await areas.getByRole("button", { name: "Изменить Мой проект", exact: true }).click();
  await expect(areas.getByLabel("Цвет", { exact: true })).toHaveValue("#d76b4c");
  await expect(
    areas
      .getByRole("group", { name: "Значок", exact: true })
      .getByRole("button", { name: "Проект", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");

  await page.goto("/app/settings/appearance");
  await choice(page, "Тема аккаунта", "Тёмная").click();
  await page.getByRole("button", { name: "Проверить анимацию", exact: true }).click();
  await expect(page.locator(".preview-mini > span").first()).toHaveCSS(
    "animation-name",
    "preview-rise",
  );
  await choice(page, "Уменьшение движения", "Всегда уменьшать").click();
  await page.getByRole("button", { name: "Проверить анимацию", exact: true }).click();
  await expect(page.locator(".preview-mini > span").first()).toHaveCSS("animation-name", "none");
  await page.getByRole("button", { name: "Сохранить оформление", exact: true }).click();
  await expect(page.locator("html")).toHaveClass(/dark/);
  await expect(page.locator("html")).toHaveAttribute("data-motion", "reduce");
  await page.reload();
  await expect(choice(page, "Тема аккаунта", "Тёмная")).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator("html")).toHaveAttribute("data-motion", "reduce");
  await page.screenshot({ path: info.outputPath("appearance-saved-dark.png"), fullPage: true });
  await page.goto("/app/quests");
  await page.getByRole("button", { name: "Создать задачу", exact: true }).click();
  await expect(page.locator(".task-composer")).toHaveCSS("animation-name", "none");
});

test("workspace audit covers remaining routes and expanded editors in both themes", async ({
  page,
}, info) => {
  test.setTimeout(240000);
  await account(page);
  await page.emulateMedia({ reducedMotion: "reduce" });
  const routes = [
    "quests",
    "habits",
    "goals",
    "calendar",
    "notifications",
    "settings",
    "settings/profile",
    "settings/schedule",
    "settings/areas",
    "settings/appearance",
    "settings/notifications",
    "settings/privacy",
    "settings/security",
    "onboarding",
    "guide",
  ];
  for (const theme of ["light", "dark"]) {
    await page.getByLabel("Тема оформления", { exact: true }).selectOption(theme);
    await expect(page.locator("html")).toHaveClass(new RegExp(theme));
    await expect(page.getByLabel("Тема оформления", { exact: true })).toBeEnabled();
    for (const route of routes) {
      await page.goto("/app/" + route);
      await page.waitForLoadState("networkidle");
      await expect(page.locator("html")).toHaveClass(new RegExp(theme));
      if (route === "quests") {
        await page.getByRole("button", { name: "Создать задачу", exact: true }).click();
        await revealControl(page.getByLabel("Приоритет", { exact: true }));
      }
      if (route === "habits")
        await page.getByRole("button", { name: "Создать привычку", exact: true }).click();
      if (route === "goals")
        await page.getByRole("button", { name: "Создать цель", exact: true }).click();
      if (route === "calendar")
        await page.getByRole("button", { name: "Добавить событие", exact: true }).click();
      await page.setViewportSize({ width: 1440, height: 1000 });
      await expect
        .poll(async () => (await page.locator("main").boundingBox())?.x ?? 0)
        .toBeGreaterThanOrEqual(244);
      await page.screenshot({
        path: info.outputPath(`${route.replaceAll("/", "-")}-${theme}-desktop.png`),
        fullPage: true,
      });
      await page.setViewportSize({ width: 360, height: 800 });
      await expect
        .poll(() => page.evaluate(() => document.documentElement.scrollWidth - innerWidth), {
          message: `${route} ${theme}`,
        })
        .toBeLessThanOrEqual(0);
      await expect(page.locator("main h1")).toBeVisible();
      expect(
        (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
        `${route} ${theme}`,
      ).toEqual([]);
      await page.screenshot({
        path: info.outputPath(`${route.replaceAll("/", "-")}-${theme}-mobile.png`),
        fullPage: true,
      });
    }
  }
});

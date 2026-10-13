import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
async function account(page: Page) {
  await page.clock.setFixedTime("2026-10-12T12:00:00Z");
  const email = "journal-" + crypto.randomUUID() + "@example.test",
    password = "journal browser password phrase";
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
  return { email, password };
}
async function data(page: Page, path: string) {
  const response = await page.request.get("/api/v1" + path);
  expect(response.status()).toBe(200);
  return (await response.json()).data;
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
test("journal restores private drafts escapes Markdown and clears drafts on logout", async ({
  page,
}, info) => {
  test.setTimeout(120000);
  await account(page);
  await page.goto("/app/journal");
  await page.getByLabel("Заголовок записи", { exact: true }).fill("Мой частный вывод");
  const body =
    "# Вывод\nJOURNAL_PRIVATE_SENTINEL_C120\n<script>window.__journalXss=true</script>\n[опасно](javascript:alert)\n**Полезный шаг**";
  await page.getByLabel("Текст записи", { exact: true }).fill(body);
  page.once("dialog", (dialog) => dialog.dismiss());
  await page.getByRole("button", { name: "Утро", exact: true }).click();
  await expect(page.getByLabel("Заголовок записи", { exact: true })).toHaveValue(
    "Мой частный вывод",
  );
  page.once("dialog", (dialog) => dialog.accept());
  await page.reload();
  await page.getByRole("button", { name: "Восстановить черновик", exact: true }).click();
  await expect(page.getByLabel("Текст записи", { exact: true })).toHaveValue(body);
  await page.getByRole("button", { name: "Сохранить запись", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Запись сохранена" })).toBeVisible();
  const preview = page.getByRole("article", { name: "Предпросмотр сохранённой записи" });
  await expect(preview.locator("script,img")).toHaveCount(0);
  await expect(preview.locator("strong")).toHaveText("Полезный шаг");
  expect(await page.evaluate(() => "__journalXss" in window)).toBe(false);
  await page.reload();
  await expect(
    page.getByRole("button", { name: "Открыть запись «Мой частный вывод»", exact: true }),
  ).toBeVisible();
  await accessible(page, "journal-private", info);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.getByLabel("Заголовок записи", { exact: true }).fill("Несохранённое перед выходом");
  await page.getByLabel("Текст записи", { exact: true }).fill("Только текущий аккаунт");
  page.once("dialog", (dialog) => dialog.accept());
  await page.goto("/app");
  await page.getByRole("button", { name: "Выйти", exact: true }).click();
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          Object.keys(sessionStorage).filter((key) => key.startsWith("ascend-private-draft:"))
            .length,
      ),
    )
    .toBe(0);
  await account(page);
  await page.goto("/app/journal");
  await expect(
    page.getByRole("button", { name: "Открыть запись «Мой частный вывод»", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Восстановить черновик", exact: true }),
  ).toHaveCount(0);
});
test("journal rituals keep snapshots and Weekly Reset applies chosen changes once", async ({
  page,
}, info) => {
  test.setTimeout(180000);
  let expectedReload = false;
  const unexpectedDialogs: string[] = [];
  page.on("dialog", async (dialog) => {
    if (!expectedReload) {
      unexpectedDialogs.push(dialog.message());
      console.info("journal acceptance: unexpected navigation guard");
    }
    await dialog.accept();
  });
  await account(page);
  await page.goto("/app/quests");
  await page.getByRole("button", { name: "Создать задачу", exact: true }).click();
  await page.getByLabel("Название задачи", { exact: true }).fill("Задача ритуала");
  await page.getByLabel("Плановая дата", { exact: true }).fill("2026-10-12");
  await page.getByLabel("Дедлайн с часовым поясом", { exact: true }).fill("2026-10-14T18:00:00Z");
  await page.getByRole("button", { name: "Сохранить задачу", exact: true }).click();
  await page.getByRole("status").filter({ hasText: "Задача сохранена" }).getByRole("link").click();
  await expect(page).toHaveURL(/\/app\/quests\/[a-f0-9-]+$/);
  const taskId = page.url().split("/").at(-1)!;
  await page.goto("/app/journal");
  await page.getByRole("button", { name: "Утро", exact: true }).click();
  await page.getByLabel("Доступно минут", { exact: true }).fill("60");
  await page.getByLabel("Задача в план", { exact: true }).selectOption({ label: "Задача ритуала" });
  await page.getByRole("button", { name: "Добавить выбранную задачу", exact: true }).click();
  await page.getByLabel("Главная задача", { exact: true }).check();
  await page.getByLabel("Оценить доступное время", { exact: true }).check();
  await page.getByRole("button", { name: "Подтвердить утренний план", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "Утренний план подтверждён" }),
  ).toBeVisible();
  console.info("journal acceptance: first morning saved");
  const first = await data(page, "/routine-runs/morning/2026-10-12");
  await page.getByLabel("Доступно минут", { exact: true }).fill("90");
  await page.getByLabel("Причина изменения плана", { exact: true }).fill("Уточнил время");
  await page.getByRole("button", { name: "Подтвердить утренний план", exact: true }).click();
  await expect
    .poll(async () => (await data(page, "/routine-runs/morning/2026-10-12")).run.version)
    .toBe(2);
  const next = await data(page, "/routine-runs/morning/2026-10-12");
  expect(next.run.id).toBe(first.run.id);
  expect(next.plan.id).toBe(first.plan.id);
  expect(next.plan.snapshot.input.available_minutes).toBe(60);
  await expect(page.getByLabel("Причина изменения плана", { exact: true })).toHaveValue("");
  console.info("journal acceptance: repeated morning saved");
  await page.getByRole("button", { name: "Вечер", exact: true }).click();
  await page.getByLabel("Один вывод дня", { exact: true }).fill("Сохранённый вывод без настроения");
  expectedReload = true;
  await page.reload();
  expectedReload = false;
  await page.getByRole("button", { name: "Вечер", exact: true }).click();
  await page.getByRole("button", { name: "Восстановить черновик ритуала", exact: true }).click();
  await expect(page.getByLabel("Один вывод дня", { exact: true })).toHaveValue(
    "Сохранённый вывод без настроения",
  );
  await page.getByRole("button", { name: "Сохранить вечерний разбор", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "Вечерний разбор сохранён" }),
  ).toBeVisible();
  console.info("journal acceptance: evening saved");
  const review = await data(page, "/reviews/day/2026-10-12");
  expect(review.stats.planned).toBe(1);
  expect(review.stats.completed).toBe(0);
  expect(review.run.payload.answers.mood).toBeNull();
  await page.getByLabel("Один вывод дня", { exact: true }).fill("Уточнённый личный вывод");
  await page.getByRole("button", { name: "Сохранить вечерний разбор", exact: true }).click();
  await expect.poll(async () => (await data(page, "/reviews/day/2026-10-12")).run.version).toBe(2);
  await expect(page.locator('form[aria-busy="true"]')).toHaveCount(0);
  await page.getByRole("button", { name: "Чек-листы и история", exact: true }).click();
  await page.getByLabel("Шаг 1 утра", { exact: true }).fill("Новый первый шаг утра");
  await page.getByRole("button", { name: "Сохранить шаблон утра", exact: true }).click();
  await expect
    .poll(async () => (await data(page, "/routine-runs/templates/morning")).version)
    .toBe(1);
  await expect(
    page.getByRole("button", { name: "Сохранить шаблон утра", exact: true }),
  ).toBeEnabled();
  console.info("journal acceptance: template saved");
  const history = await data(page, "/routine-runs");
  expect(
    history.items.some(
      (event: {
        snapshot: { kind: string; payload: { template: { steps: { label: string }[] } } };
      }) =>
        event.snapshot.kind === "morning" &&
        event.snapshot.payload.template.steps[0]?.label === "Оценить доступное время",
    ),
  ).toBe(true);
  await page.getByRole("button", { name: "Weekly Reset", exact: true }).click();
  await page.getByRole("button", { name: "Подготовить Weekly Reset", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Выбрать изменения Weekly Reset", exact: true }),
  ).toBeVisible();
  console.info("journal acceptance: reset preview ready");
  await page.getByLabel("Действие для «Задача ритуала»", { exact: true }).selectOption("move");
  await page
    .getByLabel("Причина для «Задача ритуала»", { exact: true })
    .fill("Выбираю следующую неделю");
  await page.getByLabel("Главная задача на выбранный день", { exact: true }).check();
  await page.getByLabel("Подтвердить план на 2026-10-19", { exact: true }).check();
  await page
    .getByLabel("Вывод и причина недельного решения", { exact: true })
    .fill("Сохраню реалистичный план");
  await page.getByLabel("Подтверждаю выбранные переносы, архив и планы", { exact: true }).check();
  await page.getByRole("button", { name: "Применить Weekly Reset", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Weekly Reset применён" })).toBeVisible();
  const moved = await data(page, "/quests/" + taskId);
  expect(moved.planned_date).toBe("2026-10-19");
  expect(moved.deadline).toBe("2026-10-14T18:00:00Z");
  const plan = await data(page, "/plans/day/2026-10-19");
  expect(plan.quest_ids).toEqual([taskId]);
  expect(plan.top_ids).toEqual([taskId]);
  console.info("journal acceptance: weekly reset applied");
  expect(unexpectedDialogs).toEqual([]);
  await accessible(page, "journal-weekly-reset", info);
});

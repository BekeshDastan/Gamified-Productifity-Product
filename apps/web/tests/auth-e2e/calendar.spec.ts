import { revealControl } from "./disclosure";
import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
async function account(page: Page) {
  await page.clock.setFixedTime("2026-10-12T12:00:00Z");
  const email = "calendar-" + crypto.randomUUID() + "@example.test",
    password = "calendar browser password phrase";
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
  await page.goto("/app/calendar");
  await expect(page.getByLabel("Вид календаря")).toBeVisible();
}
async function openList(page: Page) {
  const summary = page.getByText(/События диапазона — доступный список/);
  if (await summary.evaluate((el) => !el.parentElement?.hasAttribute("open")))
    await summary.click();
  return page.locator("details").filter({ has: summary });
}
async function selectEvent(page: Page, name: string) {
  const list = await openList(page);
  await list.getByRole("button", { name, exact: true }).click();
  await expect(page.getByLabel("Название события")).toBeVisible();
}
test("calendar recurring edits persist across five views and the quest list", async ({
  page,
}, info) => {
  test.setTimeout(120000);
  await account(page);
  await page.getByRole("button", { name: "Добавить событие", exact: true }).click();
  await page.getByLabel("Название события").fill("Ежедневное чтение");
  await page.getByLabel("Начало события").fill("2026-10-12");
  await page.getByLabel("Конец события").fill("2026-10-13");
  await page.getByLabel("Повторение", { exact: true }).selectOption("daily");
  await page.getByLabel("Что повторять").selectOption("task");
  await page.getByLabel("Число повторений").fill("3");
  await page.getByRole("button", { name: "Сохранить событие", exact: true }).click();
  await expect(page.getByLabel("Название события")).toHaveCount(0);
  await revealControl(page.getByLabel("Источник календаря"));
  await page.getByLabel("Источник календаря").selectOption("block");
  for (const view of [
    "timeGridDay",
    "timeGridWeek",
    "dayGridMonth",
    "multiMonthYear",
    "listMonth",
  ]) {
    await page.getByLabel("Вид календаря").selectOption(view);
    await expect(page.getByLabel("Источник календаря")).toHaveValue("block");
    const list = await openList(page);
    await expect(list.getByRole("button", { name: /Ежедневное чтение/ })).toHaveCount(
      view === "timeGridDay" ? 1 : 3,
    );
    await expect(page.locator(".fc-view")).toBeVisible();
  }
  await page.getByLabel("Вид календаря").selectOption("dayGridMonth");
  await selectEvent(page, "2026-10-12 · Ежедневное чтение");
  await page.getByLabel("Название события").fill("Чтение перенесено");
  await page.getByLabel("Начало события").fill("2026-10-13");
  await page.getByLabel("Конец события").fill("2026-10-14");
  await page.getByLabel("Причина изменения").fill("План изменился");
  await page.getByRole("button", { name: "Сохранить событие", exact: true }).click();
  await expect(page.getByLabel("Название события")).toHaveCount(0);
  await revealControl(page.getByLabel("Источник календаря"));
  await page.getByLabel("Источник календаря").selectOption("history");
  let list = await openList(page);
  await expect(
    list.getByRole("button", { name: "2026-10-12 · Прежний план: Ежедневное чтение", exact: true }),
  ).toHaveCount(1);
  await list.getByRole("button").click();
  await page.getByRole("link", { name: "Открыть задачу", exact: true }).click();
  await expect(page.getByLabel("Плановая дата", { exact: true })).toHaveValue("2026-10-13");
  await expect(page.getByLabel("Статус", { exact: true })).toHaveValue("postponed");
  await page.goto("/app/calendar");
  await revealControl(page.getByLabel("Источник календаря"));
  await page.getByLabel("Источник календаря").selectOption("block");
  await selectEvent(page, "2026-10-14 · Ежедневное чтение");
  await page.getByLabel("Отменить событие").check();
  await page.getByLabel("Причина изменения").fill("День отдыха");
  await page.getByRole("button", { name: "Сохранить событие", exact: true }).click();
  await expect(page.getByLabel("Название события")).toHaveCount(0);
  await selectEvent(page, "2026-10-13 · Ежедневное чтение");
  await page.getByLabel("Изменить повторения").selectOption("future");
  await page.getByLabel("Название события").fill("Будущее чтение");
  await page.getByLabel("Число повторений").fill("2");
  await page.getByLabel("Причина изменения").fill("Новый будущий план");
  await page.getByRole("button", { name: "Сохранить событие", exact: true }).click();
  await expect(page.getByLabel("Название события")).toHaveCount(0);
  await page.reload();
  await revealControl(page.getByLabel("Источник календаря"));
  await page.getByLabel("Источник календаря").selectOption("block");
  await page.getByLabel("Статус календаря").selectOption("cancelled");
  list = await openList(page);
  await expect(
    list.getByRole("button", { name: "2026-10-14 · Ежедневное чтение", exact: true }),
  ).toHaveCount(1);
  await page.setViewportSize({ width: 360, height: 800 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  expect(
    (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
  ).toEqual([]);
  await page.getByRole("heading", { name: "Календарь", exact: true }).click();
  await page.screenshot({ path: info.outputPath("calendar-light-mobile.png"), fullPage: true });
  await page.getByLabel("Тема оформления").selectOption("dark");
  await expect(page.locator("html")).toHaveClass(/dark/);
  expect(
    (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
  ).toEqual([]);
  await page.getByRole("heading", { name: "Календарь", exact: true }).click();
  await page.screenshot({ path: info.outputPath("calendar-dark-mobile.png"), fullPage: true });
});
test("calendar time block drag resize and failed move revert", async ({ page }, info) => {
  test.setTimeout(120000);
  await account(page);
  await page.getByRole("button", { name: "Добавить событие", exact: true }).click();
  await page.getByLabel("Название события").fill("Работа над проектом");
  await page.getByLabel("Без времени", { exact: true }).uncheck();
  await page.getByLabel("Начало события").fill("2026-10-12T09:00");
  await page.getByLabel("Конец события").fill("2026-10-12T10:00");
  await page.getByRole("button", { name: "Сохранить событие", exact: true }).click();
  await expect(page.getByLabel("Название события")).toHaveCount(0);
  await page.getByLabel("Вид календаря").selectOption("timeGridDay");
  const event = page.locator(".fc-timegrid-event").filter({ hasText: "Работа над проектом" });
  await expect(event).toHaveCount(1);
  await event.scrollIntoViewIfNeeded();
  const initialTop = await event
    .locator("..")
    .evaluate((el) => parseFloat((el as HTMLElement).style.top));
  const before = await event.boundingBox();
  if (!before) throw Error("event missing");
  const moved = page.waitForResponse(
    (r) => r.url().includes("/api/v1/calendar/events/") && r.request().method() === "PATCH",
  );
  await page.mouse.move(before.x + before.width / 2, before.y + 12);
  await page.mouse.down();
  await page.mouse.move(before.x + before.width / 2, before.y + 60, { steps: 12 });
  await page.mouse.up();
  expect((await moved).status()).toBe(200);
  await expect(page.getByRole("status").filter({ hasText: "Событие сохранено" })).toBeVisible();
  await page.reload();
  await page.getByLabel("Вид календаря").selectOption("timeGridDay");
  await event.scrollIntoViewIfNeeded();
  let current = await event.boundingBox();
  if (!current) throw Error("moved event missing");
  expect(
    await event.locator("..").evaluate((el) => parseFloat((el as HTMLElement).style.top)),
  ).toBeGreaterThan(initialTop + 10);
  const resizer = event.locator(".fc-event-resizer-end");
  await event.hover();
  const edge = await resizer.boundingBox();
  if (!edge) throw Error("resize missing");
  const resized = page.waitForResponse(
    (r) => r.url().includes("/api/v1/calendar/events/") && r.request().method() === "PATCH",
  );
  await page.mouse.move(edge.x + edge.width / 2, edge.y + edge.height / 2);
  await page.mouse.down();
  await page.mouse.move(edge.x + edge.width / 2, edge.y + edge.height / 2 + 48, { steps: 12 });
  await page.mouse.up();
  expect((await resized).status()).toBe(200);
  await expect(page.getByRole("status").filter({ hasText: "Событие сохранено" })).toBeVisible();
  await page.reload();
  await page.getByLabel("Вид календаря").selectOption("timeGridDay");
  await event.scrollIntoViewIfNeeded();
  current = await event.boundingBox();
  if (!current) throw Error("resized event missing");
  expect(current.height).toBeGreaterThan(before.height + 10);
  await page.route("**/api/v1/calendar/events/*", (route) =>
    route.request().method() === "PATCH"
      ? route.fulfill({
          status: 409,
          contentType: "application/json",
          body: JSON.stringify({ error: { code: "VERSION_CONFLICT", message: "Запись изменена" } }),
        })
      : route.continue(),
  );
  const original = current;
  const originalTime = await event.locator(".fc-event-time").innerText();
  const originalTop = await event
    .locator("..")
    .evaluate((el) => parseFloat((el as HTMLElement).style.top));
  await page.mouse.move(original.x + original.width / 2, original.y + 12);
  await page.mouse.down();
  await page.mouse.move(original.x + original.width / 2, original.y + 60, { steps: 12 });
  await page.mouse.up();
  await expect(
    page.getByRole("alert").filter({ hasText: "Событие возвращено на прежнее место" }),
  ).toBeVisible();
  await expect
    .poll(async () =>
      Math.abs(
        (await event.locator("..").evaluate((el) => parseFloat((el as HTMLElement).style.top))) -
          originalTop,
      ),
    )
    .toBeLessThanOrEqual(1);
  await expect(event.locator(".fc-event-time")).toHaveText(originalTime);
  await page.unroute("**/api/v1/calendar/events/*");
  await page.screenshot({ path: info.outputPath("calendar-timegrid.png"), fullPage: true });
});

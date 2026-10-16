import { test, expect, type Page, type APIRequestContext } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
const password = "social browser acceptance phrase";
async function account(page: Page, request: APIRequestContext, name: string) {
  const email = `social-${crypto.randomUUID()}@example.test`;
  await page.goto("/auth/register");
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill(password);
  await page.getByLabel("Повторите пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Создать аккаунт", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Аккаунт создан" })).toBeVisible();
  let link = "";
  const mail = process.env.TEST_MAIL_HTTP!;
  await expect
    .poll(
      async () => {
        const list = await (await request.get(mail + "/api/v1/messages?limit=100")).json();
        for (const item of list.messages.filter((m: { To: { Address: string }[] }) =>
          m.To.some((to) => to.Address === email),
        )) {
          const message = await (await request.get(mail + "/api/v1/message/" + item.ID)).json();
          const match = message.Text.match(
            /http:\/\/127\.0\.0\.1:3100\/auth\/verify-email#token=[A-Za-z0-9_-]{43}/,
          );
          if (match) {
            link = match[0];
            return true;
          }
        }
        return false;
      },
      { timeout: 40000 },
    )
    .toBe(true);
  await page.goto("/auth/login");
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page).toHaveURL("/app");
  await page.goto(link);
  await page.getByRole("button", { name: "Подтвердить почту", exact: true }).click();
  await expect(page.getByRole("status")).toHaveText("Почта подтверждена. Можно продолжать!");
  // Give the two fixtures distinguishable public pseudonyms using the existing profile contract.
  const profile = (await (await page.request.get("/api/v1/me")).json()).data;
  const csrf = (await (await page.request.get("/api/v1/auth/csrf")).json()).data.token;
  const response = await page.request.patch("/api/v1/me", {
    headers: {
      Origin: "http://127.0.0.1:3100",
      "X-CSRF-Token": csrf,
      "If-Match": `"${profile.version}"`,
    },
    data: { name, nickname: name.toLowerCase(), avatar: profile.avatar },
  });
  expect(response.status()).toBe(200);
  await page.goto("/app/friends");
  await expect(page.getByRole("heading", { name: "Вместе — дальше" })).toBeVisible();
  await expect(page.getByLabel("Мой код приглашения")).toBeVisible();
}
test("social two people consent live revocation coop review block and mobile", async ({
  page,
  browser,
  request,
}, info) => {
  test.setTimeout(180000);
  const second = await browser.newContext({ baseURL: "http://127.0.0.1:3100" });
  const friend = await second.newPage();
  try {
    await account(page, request, "Aurora");
    await account(friend, request, "Orion");
    const code = (await friend.getByLabel("Мой код приглашения").textContent())!;
    await page.getByLabel("У вас уже есть код друга?").fill(code);
    await page.getByRole("button", { name: "Отправить приглашение", exact: true }).click();
    await expect(friend.getByRole("button", { name: "Принять дружбу", exact: true })).toBeVisible();
    await friend.getByRole("button", { name: "Принять дружбу", exact: true }).click();
    await expect(page.getByRole("heading", { name: "orion", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Настроить мой доступ", exact: true }).click();
    await page.getByLabel("Регулярность за 7 дней", { exact: false }).check();
    await expect(friend.getByText("активных дней · UTC", { exact: true })).toBeVisible();
    await page.getByLabel("Регулярность за 7 дней", { exact: false }).uncheck();
    await expect(friend.getByText("активных дней · UTC", { exact: true })).toHaveCount(0);
    await page.getByRole("button", { name: "Закрыть", exact: true }).click();
    await page.getByRole("button", { name: "Общие испытания", exact: true }).click();
    await page.getByRole("button", { name: "Новое общее испытание", exact: true }).click();
    await page.getByLabel("Название общего испытания").fill("Книжная команда");
    await page.getByLabel("Общая цель", { exact: true }).fill("20");
    const consent = page.getByLabel("Принимаю правила и разрешаю показывать мой вклад", {
      exact: false,
    });
    await expect(page.getByRole("button", { name: "Создать с общими правилами" })).toBeDisabled();
    await consent.check();
    await page.route("**/api/v1/social/rooms", async (route) => {
      if (route.request().method() === "POST") await route.abort("failed");
      else await route.continue();
    });
    await page.getByRole("button", { name: "Создать с общими правилами" }).click();
    await expect(page.getByRole("alert")).toBeVisible();
    await expect(page.getByLabel("Название общего испытания")).toHaveValue("Книжная команда");
    await page.unroute("**/api/v1/social/rooms");
    await page.getByRole("button", { name: "Создать с общими правилами" }).click();
    const room = page.getByRole("region", { name: "Книжная команда", exact: true });
    await expect(room).toBeVisible();
    await room.getByLabel("Пригласить друга в испытание").selectOption({ label: "orion" });
    await room.getByRole("button", { name: "Пригласить в испытание", exact: true }).click();
    await friend.getByRole("button", { name: "Общие испытания", exact: true }).click();
    const partnerRoom = friend.getByRole("region", { name: "Книжная команда", exact: true });
    await expect(partnerRoom).toBeVisible();
    await expect(
      partnerRoom.getByRole("button", { name: "Присоединиться", exact: true }),
    ).toBeDisabled();
    await partnerRoom
      .getByLabel("Принимаю правила и разрешаю показывать мой вклад", { exact: false })
      .check();
    await partnerRoom.getByRole("button", { name: "Присоединиться", exact: true }).click();
    await partnerRoom
      .getByRole("button", { name: "Увеличить: Книжная команда", exact: true })
      .click();
    await partnerRoom
      .getByRole("button", { name: "Увеличить: Книжная команда", exact: true })
      .click();
    await partnerRoom.getByRole("button", { name: "Сохранить отметку", exact: true }).click();
    await expect(room.locator(".social-score strong")).toHaveText("2");
    await friend.reload();
    await friend.getByRole("button", { name: "Общие испытания", exact: true }).click();
    await expect(partnerRoom.getByLabel("Сегодня, стр.", { exact: true })).toHaveValue("2");
    await partnerRoom.getByRole("button", { name: "Попросить подтверждение", exact: true }).click();
    await friend.getByLabel("Кто подтвердит?").selectOption({ label: "aurora" });
    await friend
      .getByLabel("Разрешаю выбранному партнёру видеть отметку", { exact: false })
      .check();
    await friend.getByRole("button", { name: "Отправить на подтверждение", exact: true }).click();
    await expect(page.getByRole("button", { name: "Подтвердить", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Подтвердить", exact: true }).click();
    await expect(partnerRoom.getByText("Подтверждено партнёром", { exact: true })).toBeVisible();
    await page.setViewportSize({ width: 390, height: 844 });
    expect(
      (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
    ).toEqual([]);
    await expect
      .poll(() => page.evaluate(() => document.documentElement.scrollWidth - innerWidth))
      .toBeLessThanOrEqual(0);
    await page.screenshot({ path: info.outputPath("social-mobile.png"), fullPage: true });
    await page.emulateMedia({ reducedMotion: "reduce" });
    await expect
      .poll(() =>
        page
          .locator(".social-room")
          .first()
          .evaluate((el) => getComputedStyle(el).animationName),
      )
      .toBe("none");
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.screenshot({ path: info.outputPath("social-desktop.png"), fullPage: true });
    await page.getByLabel("Тема оформления", { exact: true }).selectOption("dark");
    expect(
      (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa"]).analyze()).violations,
    ).toEqual([]);
    await page.screenshot({ path: info.outputPath("social-dark.png"), fullPage: true });
    await page.getByLabel("Тема оформления", { exact: true }).selectOption("light");
    await friend.getByRole("button", { name: "Поддержка и победы", exact: true }).click();
    await friend.getByLabel("Мой партнёр", { exact: true }).selectOption({ label: "aurora" });
    await friend.getByLabel("Принимать поддержку", { exact: false }).check();
    await page.getByRole("button", { name: "Поддержка и победы", exact: true }).click();
    await page.getByLabel("Кого поддержать", { exact: true }).selectOption({ label: "orion" });
    await page.getByRole("button", { name: "Поддержать", exact: true }).click();
    await expect(friend.locator(".social-nudge")).toContainText("Один небольшой шаг");
    await friend.reload();
    await friend.getByRole("button", { name: "Поддержка и победы", exact: true }).click();
    await expect(friend.getByLabel("Принимать поддержку", { exact: false })).toBeChecked();
    await friend.getByLabel("Принимать поддержку", { exact: false }).uncheck();
    await expect(friend.locator(".social-nudge")).toHaveCount(0);
    // Earn a real achievement through an existing task, then publish it explicitly.
    await page.goto("/app/challenges");
    await page
      .getByRole("button", { name: /^Принять «/ })
      .first()
      .click();
    await expect(
      page.getByRole("link", { name: "Открыть принятую задачу", exact: true }),
    ).toHaveCount(1);
    const task = (await (await page.request.get("/api/v1/quests")).json()).data.items[0];
    await page.goto("/app/quests/" + task.id);
    const actions = page
      .locator("section")
      .filter({ has: page.getByRole("heading", { name: "Следующий шаг", exact: true }) });
    await actions.getByRole("button", { name: "Завершить", exact: true }).click();
    await actions.getByRole("button", { name: "Подтвердить действие", exact: true }).click();
    await expect(page.getByLabel("Статус", { exact: true })).toHaveValue("completed");
    expect((await page.request.get("/api/v1/achievements")).status()).toBe(200);
    await page.goto("/app/friends");
    await page.getByRole("button", { name: "Поддержка и победы", exact: true }).click();
    await page
      .getByLabel("Поделиться заработанным достижением", { exact: true })
      .selectOption({ index: 1 });
    await page.getByRole("button", { name: "Опубликовать для друзей", exact: true }).click();
    await expect(friend.locator(".social-feed article")).toHaveCount(1);
    await page.getByRole("button", { name: "Скрыть публикацию", exact: true }).click();
    await expect(friend.locator(".social-feed article")).toHaveCount(0);
    await page.getByRole("button", { name: "Общие испытания", exact: true }).click();
    await page.getByRole("button", { name: "Новое общее испытание", exact: true }).click();
    await page.getByRole("button", { name: /Дружеский PvP/ }).click();
    await page.getByLabel("Название общего испытания").fill("Равный старт");
    await page
      .getByLabel("Принимаю правила и разрешаю показывать мой вклад", { exact: false })
      .check();
    await page.getByLabel("Отдельно согласен участвовать в рейтинге", { exact: false }).check();
    await page.getByRole("button", { name: "Создать с общими правилами", exact: true }).click();
    const race = page.getByRole("region", { name: "Равный старт", exact: true });
    await race.getByLabel("Пригласить друга в испытание").selectOption({ label: "orion" });
    await race.getByRole("button", { name: "Пригласить в испытание", exact: true }).click();
    await friend.getByRole("button", { name: "Общие испытания", exact: true }).click();
    const raceInvite = friend.getByRole("region", { name: "Равный старт", exact: true });
    await raceInvite
      .getByLabel("Принимаю правила и разрешаю показывать мой вклад", { exact: false })
      .check();
    await raceInvite
      .getByLabel("Отдельно согласен участвовать в рейтинге", { exact: false })
      .check();
    await raceInvite.getByRole("button", { name: "Присоединиться", exact: true }).click();
    await expect(
      race.getByRole("list", { name: "Участники: Равный старт" }).getByRole("listitem"),
    ).toHaveCount(2);
    await expect(
      raceInvite.getByRole("button", { name: "Сохранить отметку", exact: true }),
    ).toHaveCount(0);
    await friend.getByRole("button", { name: "Мои люди", exact: true }).click();
    await friend.getByText("Действия с другом", { exact: true }).click();
    await friend.getByRole("button", { name: "Заблокировать", exact: true }).click();
    await friend.getByRole("button", { name: "Подтвердить действие", exact: true }).click();
    await expect(room.locator(".social-score strong")).toHaveText("0");
    await page.getByRole("button", { name: "Мои люди", exact: true }).click();
    await expect(page.getByRole("heading", { name: "orion", exact: true })).toHaveCount(0);
    await expect(friend.getByRole("button", { name: "Разблокировать", exact: true })).toBeVisible();
  } finally {
    await second.close();
  }
});

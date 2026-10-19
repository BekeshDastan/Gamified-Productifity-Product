import assert from "node:assert/strict";
import { readFileSync, mkdirSync } from "node:fs";
import https from "node:https";
import { spawnSync } from "node:child_process";
import { chromium } from "@playwright/test";

const project = process.argv[2] ?? "ascend-release-local";
if (project !== "ascend-release-local")
  throw new Error("This smoke creates data only in the isolated local release project");
mkdirSync(".data/release-local", { recursive: true });
const certificate = ".data/release-local/root.crt";
const copy = spawnSync(
  "docker",
  ["cp", `${project}-edge-1:/data/caddy/pki/authorities/local/root.crt`, certificate],
  { stdio: "inherit" },
);
assert.equal(copy.status, 0);
await new Promise((resolve, reject) => {
  const request = https.get(
    "https://localhost:3443/health/ready",
    { ca: readFileSync(certificate) },
    (response) => {
      assert.equal(response.statusCode, 200);
      assert.equal(response.headers["strict-transport-security"], "max-age=31536000");
      response.resume();
      response.on("end", resolve);
    },
  );
  request.on("error", reject);
});
const browser = await chromium.launch();
const context = await browser.newContext({
  baseURL: "https://localhost:3443",
  ignoreHTTPSErrors: true,
});
try {
  const api = context.request;
  async function mutate(path, data, headers = {}) {
    const csrf = await api.get("/api/v1/auth/csrf");
    assert.equal(csrf.status(), 200);
    const token = (await csrf.json()).data.token;
    const response = await api.post(`/api/v1${path}`, {
      data,
      headers: {
        Origin: "https://localhost:3443",
        "X-CSRF-Token": token,
        "Idempotency-Key": crypto.randomUUID(),
        ...headers,
      },
    });
    assert.ok(response.ok(), `${path}: HTTP ${response.status()}`);
    return (await response.json()).data;
  }
  const email = `release-${crypto.randomUUID()}@example.test`;
  const password = `local smoke ${crypto.randomUUID()}`;
  await mutate("/auth/register", { email, password });
  await mutate("/auth/login", { email, password, remember_me: false });
  const cookies = (await context.cookies()).filter((cookie) => cookie.name.startsWith("__Host-"));
  assert.ok(cookies.length >= 2);
  assert.ok(
    cookies.every((cookie) => cookie.secure && cookie.httpOnly && cookie.sameSite === "Lax"),
  );
  const quest = await mutate("/quests", {
    title: "Локальная приёмка запуска",
    priority: "normal",
    difficulty: 2,
    status: "todo",
    required: true,
    tags: [],
  });
  const saved = await api.get(`/api/v1/quests/${quest.id}`);
  assert.equal((await saved.json()).data.title, "Локальная приёмка запуска");
  const csrf = (await (await api.get("/api/v1/auth/csrf")).json()).data.token;
  const upload = await api.post("/api/v1/attachments", {
    headers: {
      Origin: "https://localhost:3443",
      "X-CSRF-Token": csrf,
      "Idempotency-Key": crypto.randomUUID(),
    },
    multipart: {
      quest_id: quest.id,
      version: `"${quest.version}"`,
      file: {
        name: "release-check.txt",
        mimeType: "text/plain",
        buffer: Buffer.from("ASCEND release backup and scanner fixture\n"),
      },
    },
  });
  assert.ok(upload.ok(), `upload: HTTP ${upload.status()}`);
  const attachment = (await upload.json()).data;
  assert.equal(attachment.verdict, "clean", "Live ClamAV must classify the clean file");
  const page = await context.newPage();
  const failures = [];
  page.on("pageerror", (error) => failures.push(error.message));
  await page.goto("/app/quests");
  await page.getByRole("heading", { name: "Задачи", exact: true }).waitFor();
  await page.getByText("Локальная приёмка запуска", { exact: true }).first().waitFor();
  await page.reload();
  await page.getByText("Локальная приёмка запуска", { exact: true }).first().waitFor();
  assert.deepEqual(failures, []);
  await page.screenshot({ path: ".data/release-local/https-quests.png", fullPage: true });
  assert.equal((await api.get("/metrics")).status(), 404);
  console.log(
    "HTTPS certificate, HSTS, secure cookies, registration/login, persisted quest, live clean-file scan, reload and private metrics: passed.",
  );
} finally {
  await context.close();
  await browser.close();
}

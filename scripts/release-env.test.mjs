import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, rmSync, rmdirSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { test } from "node:test";

test("release validation distinguishes local readiness and rejects misconfiguration without secrets", () => {
  const directory = mkdtempSync(path.join(tmpdir(), "ascend-release-env-"));
  const file = path.join(directory, "release.env");
  const password = "fixture-password-never-print-123456789";
  const config = {
    APP_ENV: "production",
    APP_DOMAIN: "localhost",
    APP_ORIGIN: "https://localhost:3443",
    RELEASE_TAG: "test",
    RELEASE_ENV_FILE: file.replaceAll("\\", "/"),
    POSTGRES_PASSWORD: password,
    DATABASE_URL: `postgres://ascend:${password}@db:5432/ascend?sslmode=disable`,
    AUTH_SIGNING_KEY: Buffer.alloc(32, 1).toString("base64"),
    AUTH_INSECURE_LOCAL_COOKIES: "false",
  };
  function check(values, local = true) {
    writeFileSync(
      file,
      Object.entries(values)
        .map(([key, value]) => `${key}=${value}`)
        .join("\n"),
    );
    const result = spawnSync(
      process.execPath,
      ["scripts/validate-release-env.mjs", file, ...(local ? ["--local"] : [])],
      { encoding: "utf8" },
    );
    assert.ok(
      !`${result.stdout}${result.stderr}`.includes(password),
      "credentials must never appear in diagnostics",
    );
    return result;
  }
  try {
    assert.equal(check(config).status, 0);
    const publicAttempt = check(config, false);
    assert.notEqual(publicAttempt.status, 0);
    assert.match(publicAttempt.stderr, /actual domain/);
    assert.match(publicAttempt.stderr, /SMTP_HOST is required/);
    for (const changes of [
      { APP_DOMAIN: "different.example.test" },
      { APP_ORIGIN: "http://localhost:3443" },
      { AUTH_INSECURE_LOCAL_COOKIES: "true" },
      { AUTH_SIGNING_KEY: "!" + config.AUTH_SIGNING_KEY },
      { DATABASE_URL: `postgres://ascend:${password}@public.example.test/ascend` },
      { OPS_BEARER_TOKEN: "short" },
    ])
      assert.notEqual(check({ ...config, ...changes }).status, 0);
  } finally {
    rmSync(file, { force: true });
    rmdirSync(directory);
  }
});

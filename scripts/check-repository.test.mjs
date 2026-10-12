import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { inspectFile } from "./check-repository.mjs";

describe("repository guard", () => {
  it("rejects tracked local environments, private storage and binaries", () => {
    for (const filename of [
      ".env",
      "apps/web/.env.local",
      "private.key",
      ".data/export.json",
      "bin/api.exe",
    ]) {
      assert.ok(inspectFile(filename, "").length, filename);
    }
  });
  it("accepts examples and ordinary source", () => {
    assert.deepEqual(inspectFile(".env.example", "APP_ENV=development"), []);
    assert.deepEqual(
      inspectFile("apps/web/src/app/page.tsx", "export default function Page() {}"),
      [],
    );
  });
  it("rejects a disguised private key and access token without returning the value", () => {
    const key = "-----BEGIN " + "PRIVATE KEY-----";
    const token = "ghp_" + "a".repeat(36);
    assert.deepEqual(inspectFile("source.txt", key), ["private key content"]);
    assert.deepEqual(inspectFile("source.txt", token), ["GitHub credential"]);
  });
});

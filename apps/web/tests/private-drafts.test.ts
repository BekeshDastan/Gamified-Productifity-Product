import { afterEach, expect, it } from "vitest";
import { clearPrivateDrafts, draftKey } from "../src/lib/private-drafts";
afterEach(() => sessionStorage.clear());
it("scopes tab drafts by account and erases only private drafts on sign-out", () => {
  const first = draftKey("owner-a", "journal"),
    second = draftKey("owner-b", "journal");
  expect(first).not.toBe(second);
  sessionStorage.setItem(first, JSON.stringify({ title: "Личное" }));
  sessionStorage.setItem(second, "other");
  sessionStorage.setItem("public-theme", "dark");
  clearPrivateDrafts();
  expect(sessionStorage.getItem(first)).toBeNull();
  expect(sessionStorage.getItem(second)).toBeNull();
  expect(sessionStorage.getItem("public-theme")).toBe("dark");
});

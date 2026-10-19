import { test, expect } from "@playwright/test";

test("production CSP uses fresh nonces, blocks injected scripts and permits hydration", async ({
  page,
}) => {
  await page.route("**/api/v1/auth/**", (route) =>
    route.request().url().endsWith("/csrf")
      ? route.fulfill({
          json: {
            data: { token: "a".repeat(43) },
            meta: { request_id: "csp-test", server_time: "2026-10-19T00:00:00Z" },
          },
        })
      : route.fulfill({ status: 401, json: { error: { code: "UNAUTHORIZED" } } }),
  );
  const response = await page.goto("/auth/register");
  const policy = response?.headers()["content-security-policy"] ?? "";
  expect(policy).toContain("frame-ancestors 'none'");
  expect(policy).not.toContain("unsafe-eval");
  const first = policy.match(/'nonce-([^']+)'/)?.[1];
  expect(first).toBeTruthy();
  await expect(page.getByLabel("Email", { exact: true })).toBeEnabled();
  const scripts = await page
    .locator("script:not([src])")
    .evaluateAll((nodes) =>
      nodes
        .filter((node) => !node.getAttribute("type")?.includes("json"))
        .map((node) => (node as HTMLScriptElement).nonce),
    );
  expect(scripts.length).toBeGreaterThan(0);
  expect(scripts.every((nonce) => nonce === first)).toBe(true);
  // Inject through the HTML parser. DevTools evaluate executes in a privileged
  // context and is not a faithful model of an injected response fragment.
  await page.route("**/auth/register", async (route) => {
    const response = await route.fetch();
    const body = (await response.text()).replace(
      "</body>",
      "<script>document.documentElement.dataset.injected = 'yes'</script></body>",
    );
    await route.fulfill({ response, body });
  });
  const next = await page.reload();
  await expect(page.locator("html")).not.toHaveAttribute("data-injected", "yes");
  expect(next?.headers()["content-security-policy"]).not.toContain(`nonce-${first}`);
  await expect(page.getByLabel("Email", { exact: true })).toBeEnabled();
});

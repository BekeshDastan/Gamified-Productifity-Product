import { expect, type Locator, type Page } from "@playwright/test";

// Existing feature tests exercise the same fields through the redesigned disclosures.
// Click real summaries; never force clicks or change the DOM's open state.
export async function revealControl(control: Locator) {
  await expect(control).toBeAttached();
  for (const parent of await control.locator("xpath=ancestor::details").all()) {
    if ((await parent.getAttribute("open")) === null)
      await parent.locator(":scope > summary").click();
  }
}
export async function expand(page: Page, label: string) {
  const summary = page.locator("summary").filter({ hasText: label }).first();
  await expect(summary).toBeVisible();
  const parent = summary.locator("..");
  if ((await parent.getAttribute("open")) === null) await summary.click();
}

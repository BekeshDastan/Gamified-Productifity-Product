import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { AppShell } from "../src/components/app-shell";
vi.mock("next/navigation", () => ({ usePathname: () => "/app" }));
describe("application navigation", () => {
  it("does not link unfinished modules to fake screens", () => {
    render(<AppShell>Содержание</AppShell>);
    expect(screen.queryByRole("link", { name: "Задачи" })).not.toBeInTheDocument();
    expect(screen.getByText("Задачи").closest("[aria-disabled]")).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.getByRole("link", { name: "Оформление" })).toHaveAttribute(
      "href",
      "/app/settings/appearance",
    );
  });
  it("opens and closes the mobile menu with keyboard", async () => {
    const user = userEvent.setup();
    render(<AppShell>Содержание</AppShell>);
    screen.getByRole("button", { name: "Открыть меню" }).focus();
    await user.keyboard("{Enter}");
    const dialog = screen.getByRole("dialog", { name: "Разделы" });
    expect(within(dialog).getByRole("link", { name: "Сегодня" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Открыть меню" })).toHaveFocus();
  });
});

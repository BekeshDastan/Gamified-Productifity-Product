import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { describe, expect, it, vi } from "vitest";
import {
  SafeLink,
  UnsavedChangesProvider,
  useUnsavedChanges,
} from "../src/lib/forms/unsaved-changes";
vi.mock("next/link", () => ({
  default: ({
    onNavigate,
    ...props
  }: ComponentProps<"a"> & { onNavigate?: (event: { preventDefault: () => void }) => void }) => (
    <a
      {...props}
      onClick={(event) => {
        onNavigate?.({ preventDefault: () => event.preventDefault() });
        event.preventDefault();
      }}
    />
  ),
}));
function Fixture() {
  useUnsavedChanges(true);
  return <SafeLink href="/app/guide">Покинуть</SafeLink>;
}
describe("unsaved changes", () => {
  it("requires explicit confirmation for internal navigation and cleans up on unmount", async () => {
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    const view = render(
      <UnsavedChangesProvider>
        <Fixture />
      </UnsavedChangesProvider>,
    );
    await userEvent.click(screen.getByRole("link"));
    expect(confirm).toHaveBeenCalledOnce();
    const event = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
    view.unmount();
    const clean = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(clean);
    expect(clean.defaultPrevented).toBe(false);
    confirm.mockRestore();
  });
});

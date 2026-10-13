import { afterEach, expect, it } from "vitest";
import { fireEvent, render, screen, waitFor, cleanup } from "@testing-library/react";
import { useState } from "react";
import { z } from "zod";
import { useTabDraft, TabDraftRecovery } from "../src/lib/forms/tab-draft";
const schema = z.object({ text: z.string() });
function Harness() {
  const [value, setValue] = useState({ text: "" });
  const draft = useTabDraft({
    storageKey: "ascend-private-draft:owner:evening",
    schema,
    value,
    dirty: value.text !== "",
    restore: setValue,
  });
  return (
    <>
      <input
        aria-label="Вывод"
        value={value.text}
        onChange={(e) => setValue({ text: e.target.value })}
      />
      <TabDraftRecovery draft={draft} />
      <button onClick={draft.clear}>Сохранено</button>
    </>
  );
}
afterEach(() => {
  cleanup();
  sessionStorage.clear();
});
it("restores a validated tab draft and deletes it after saving", async () => {
  sessionStorage.setItem(
    "ascend-private-draft:owner:evening",
    JSON.stringify({ text: "Личный вывод" }),
  );
  render(<Harness />);
  await screen.findByRole("button", { name: "Восстановить черновик ритуала" });
  fireEvent.click(screen.getByRole("button", { name: "Восстановить черновик ритуала" }));
  expect(screen.getByLabelText("Вывод")).toHaveValue("Личный вывод");
  fireEvent.click(screen.getByRole("button", { name: "Сохранено" }));
  await waitFor(() =>
    expect(sessionStorage.getItem("ascend-private-draft:owner:evening")).toBeNull(),
  );
});

import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { z } from "zod";
import { useValidatedForm } from "../src/lib/forms/use-validated-form";
import { UnsavedChangesProvider } from "../src/lib/forms/unsaved-changes";
import { Form } from "../src/components/ui/form";
import { Input } from "../src/components/ui/field";
import { Button } from "../src/components/ui/button";
import { ApiError } from "../src/lib/api/client";
const schema = z.object({ name: z.string().trim().min(2, "Не менее 2 символов") });
function Fixture({ save }: { save: (values: { name: string }) => Promise<void> }) {
  const { form, onSubmit, saving, error } = useValidatedForm({
    schema,
    defaultValues: { name: "" },
    fields: ["name"],
    save,
  });
  return (
    <Form aria-label="Тестовая форма" onSubmit={onSubmit} saving={saving} error={error}>
      <Input label="Имя" {...form.register("name")} error={form.formState.errors.name?.message} />
      <Button type="submit">Сохранить</Button>
    </Form>
  );
}
const mount = (save: (values: { name: string }) => Promise<void>) =>
  render(
    <UnsavedChangesProvider>
      <Fixture save={save} />
    </UnsavedChangesProvider>,
  );
describe("validated forms", () => {
  it("rejects invalid input without a request", async () => {
    const save = vi.fn();
    mount(save);
    await userEvent.click(screen.getByRole("button", { name: "Сохранить" }));
    expect(await screen.findByText(/Не менее/)).toBeInTheDocument();
    expect(save).not.toHaveBeenCalled();
  });
  it("locks duplicate submissions before validation, then resets dirtiness after success", async () => {
    let finish!: () => void;
    const save = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        }),
    );
    mount(save);
    await userEvent.type(screen.getByLabelText("Имя"), "Дастан");
    const before = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(before);
    expect(before.defaultPrevented).toBe(true);
    const form = screen.getByRole("form");
    fireEvent.submit(form);
    fireEvent.submit(form);
    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    expect(screen.getByRole("button")).toBeDisabled();
    expect(screen.getByLabelText("Имя")).toBeDisabled();
    await act(async () => finish());
    await waitFor(() => expect(screen.getByRole("button")).not.toBeDisabled());
    const after = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(after);
    expect(after.defaultPrevented).toBe(false);
  });
  it("shows field errors and retains input after a server failure", async () => {
    mount(async () => {
      throw new ApiError(422, "VALIDATION_ERROR", "Проверьте поля.", "r1", {
        name: "Имя занято",
        role: "ignored",
      });
    });
    await userEvent.type(screen.getByLabelText("Имя"), "Дастан");
    await userEvent.click(screen.getByRole("button"));
    expect(await screen.findByText(/Имя занято/)).toBeInTheDocument();
    expect(screen.getByLabelText("Имя")).toHaveValue("Дастан");
    expect(screen.queryByText("ignored")).not.toBeInTheDocument();
  });
});

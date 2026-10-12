import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { Button } from "../src/components/ui/button";
import { Dialog } from "../src/components/ui/dialog";
import { Input, DatePicker } from "../src/components/ui/field";
import { Progress } from "../src/components/ui/card";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "../src/components/ui/tabs";

describe("shared accessible controls", () => {
  it("labels errors and date-only input", () => {
    render(
      <>
        <Input label="Название" error="Введите название" />
        <DatePicker label="Дата" defaultValue="2026-10-12" />
      </>,
    );
    expect(screen.getByLabelText("Название")).toHaveAccessibleDescription("! Введите название");
    expect(screen.getByLabelText("Название")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByLabelText("Дата")).toHaveValue("2026-10-12");
  });
  it("traps focus in dialog and returns it to trigger on Escape", async () => {
    const user = userEvent.setup();
    render(
      <Dialog title="Изменить" description="Настройки" trigger={<Button>Открыть</Button>}>
        <Input label="Имя" />
        <Button>Сохранить</Button>
      </Dialog>,
    );
    await user.click(screen.getByRole("button", { name: "Открыть" }));
    await waitFor(() =>
      expect(screen.getByRole("dialog")).toContainElement(document.activeElement as HTMLElement),
    );
    for (let i = 0; i < 6; i++) {
      await user.tab();
      expect(screen.getByRole("dialog")).toContainElement(document.activeElement as HTMLElement);
    }
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Открыть" })).toHaveFocus();
  });
  it("tabs work with arrow keys", async () => {
    const user = userEvent.setup();
    render(
      <Tabs defaultValue="one">
        <TabsList aria-label="Разделы">
          <TabsTrigger value="one">Первый</TabsTrigger>
          <TabsTrigger value="two">Второй</TabsTrigger>
        </TabsList>
        <TabsContent value="one">Один</TabsContent>
        <TabsContent value="two">Два</TabsContent>
      </Tabs>,
    );
    await user.tab();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "Второй" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel")).toHaveTextContent("Два");
  });
  it("does not turn unknown progress into zero", () => {
    render(<Progress label="Прогресс" value={null} />);
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    expect(screen.getByText(/пока нет данных/)).toBeInTheDocument();
  });
});

import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ErrorState, ConflictDialog } from "../src/components/ui/states";
import { ServiceStatus } from "../src/components/service-status";
import ErrorPage from "../src/app/error";
import { ApiError } from "../src/lib/api/client";
afterEach(() => vi.unstubAllGlobals());
describe("page states", () => {
  it("does not display raw route exceptions and supports recovery", async () => {
    const retry = vi.fn();
    render(<ErrorPage error={new Error("password in private exception")} retry={retry} />);
    expect(screen.queryByText(/password/)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Повторить" }));
    expect(retry).toHaveBeenCalledOnce();
  });
  it("provides an explicit login state for expired sessions", () => {
    render(<ErrorState error={new ApiError(401, "UNAUTHORIZED", "error")} retry={vi.fn()} />);
    expect(screen.getByRole("link", { name: "Перейти ко входу" })).toHaveAttribute(
      "href",
      "/auth/login?next=%2Fapp",
    );
  });
  it("keeps conflict open when reload fails", async () => {
    const close = vi.fn();
    render(
      <ConflictDialog
        open
        onOpenChange={close}
        currentVersion={4}
        reload={async () => {
          throw new Error("no connection");
        }}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Загрузить актуальную запись" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Ввод сохранён");
    expect(close).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Продолжить редактирование" }));
    expect(close).toHaveBeenCalledWith(false);
  });
  it("retries a real API failure without displaying false success", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(new Response("unavailable", { status: 503 }))
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            data: { status: "ok" },
            meta: { request_id: "test-2", server_time: "2026-10-12T00:00:00Z" },
          }),
        ),
      );
    vi.stubGlobal("fetch", fetcher);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
    render(
      <QueryClientProvider client={client}>
        <ServiceStatus />
      </QueryClientProvider>,
    );
    expect(await screen.findByRole("alert")).toHaveTextContent("Не удалось загрузить данные");
    expect(screen.queryByText(/установлено/)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Повторить" }));
    expect(await screen.findByText("Соединение с сервером установлено")).toBeInTheDocument();
    expect(fetcher).toHaveBeenCalledTimes(2);
  });
});

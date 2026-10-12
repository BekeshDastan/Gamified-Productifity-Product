import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import Home from "../src/app/page";

describe("product landing", () => {
  it("explains the current availability instead of offering an unfinished account", () => {
    render(<Home />);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Большие перемены");
    expect(screen.getByRole("status")).toHaveTextContent(
      "Аккаунты и персональные функции пока в разработке",
    );
    expect(screen.getByRole("link", { name: "Войти" })).toHaveAttribute("href", "/auth/login");
    expect(screen.getByRole("link", { name: "Регистрация" })).toHaveAttribute(
      "href",
      "/auth/register",
    );
    expect(screen.getByRole("link", { name: /Лицензии ресурсов/ })).toHaveAttribute(
      "href",
      "/licenses/README.txt",
    );
  });

  it("provides a real, named navigation link", () => {
    render(<Home />);
    expect(screen.getByRole("link", { name: "ASCEND, главная" })).toHaveAttribute("href", "/");
  });
});

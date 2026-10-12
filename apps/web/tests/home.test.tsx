import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import Home from "../src/app/page";

describe("technical start page", () => {
  it("explains the current availability instead of offering an unfinished account", () => {
    render(<Home />);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Большие перемены");
    expect(screen.getByRole("status")).toHaveTextContent("пока в разработке");
    expect(screen.queryByRole("link", { name: /войти|регистрация/i })).not.toBeInTheDocument();
  });

  it("provides a real, named navigation link", () => {
    render(<Home />);
    expect(screen.getByRole("link", { name: "ASCEND, главная" })).toHaveAttribute("href", "/");
  });
});
